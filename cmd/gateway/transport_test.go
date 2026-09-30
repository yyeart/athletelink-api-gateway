package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestCoreProxyTransportAndUpstreamStatuses(t *testing.T) {
	const (
		clientRequestID = "client-supplied-id"
		coreRequestID   = "core-supplied-id"
	)

	tests := []struct {
		name        string
		coreStatus  int
		coreBody    string
		unavailable bool
		timeout     bool
	}{
		{name: "unavailable Core returns Gateway 502", unavailable: true},
		{name: "Core response timeout returns Gateway 504", timeout: true},
		{name: "Core 502 passes through", coreStatus: http.StatusBadGateway, coreBody: "Core bad gateway\n"},
		{name: "Core 504 passes through", coreStatus: http.StatusGatewayTimeout, coreBody: "Core gateway timeout\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			coreIDs := make(chan string, 1)
			coreCanceled := make(chan struct{}, 1)
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				coreIDs <- r.Header.Get("X-Request-Id")
				if tc.timeout {
					<-r.Context().Done()
					coreCanceled <- struct{}{}
					return
				}

				w.Header().Set("X-Core-Header", "from-core")
				w.Header().Set("X-Request-Id", coreRequestID)
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(tc.coreStatus)
				writeTestString(t, w, tc.coreBody)
			}))
			coreURL := core.URL
			if tc.unavailable {
				core.Close()
			} else {
				t.Cleanup(core.Close)
			}

			gatewayURL, token, client := startGatewayWithCore(t, coreURL)
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, gatewayURL+"/api/v1/requests", nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("X-Request-Id", clientRequestID)

			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("call Gateway: %v", err)
			}
			body, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read response: %v; close response: %v", readErr, closeErr)
			}

			wantStatus := tc.coreStatus
			if tc.unavailable {
				wantStatus = http.StatusBadGateway
			} else if tc.timeout {
				wantStatus = http.StatusGatewayTimeout
			}
			if response.StatusCode != wantStatus {
				t.Errorf("status = %d, want %d", response.StatusCode, wantStatus)
			}

			ids := response.Header.Values("X-Request-Id")
			if len(ids) != 1 || ids[0] == "" || ids[0] == clientRequestID || ids[0] == coreRequestID {
				t.Errorf("X-Request-Id = %q, want one Gateway-generated ID", ids)
			}
			if !tc.unavailable {
				select {
				case coreID := <-coreIDs:
					if len(ids) == 1 && coreID != ids[0] {
						t.Errorf("Core request ID = %q, client response ID = %q", coreID, ids[0])
					}
				case <-time.After(2 * time.Second):
					t.Fatal("Core did not receive the request")
				}
			}

			if tc.unavailable || tc.timeout {
				if got := response.Header.Get("X-Core-Header"); got != "" {
					t.Errorf("Gateway error included Core header %q", got)
				}
				if tc.timeout {
					select {
					case <-coreCanceled:
					case <-time.After(2 * time.Second):
						t.Error("Core request was not canceled after timeout")
					}
				}
				return
			}

			if string(body) != tc.coreBody {
				t.Errorf("body = %q, want %q", body, tc.coreBody)
			}
			if got := response.Header.Get("X-Core-Header"); got != "from-core" {
				t.Errorf("X-Core-Header = %q, want from-core", got)
			}
			if got := response.Header.Get("Content-Type"); got != "text/plain" {
				t.Errorf("Content-Type = %q, want text/plain", got)
			}
		})
	}
}

func startGatewayWithCore(t *testing.T, coreURL string) (string, string, *http.Client) {
	t.Helper()

	addresses := unusedTCPAddresses(t, 2)
	httpAddr, redisAddr := addresses[0], addresses[1]
	const secret = "gateway-transport-test-signing-key"
	t.Setenv("GATEWAY_HTTP_ADDR", httpAddr)
	setRequiredServiceURLsForTest(t, coreURL)
	t.Setenv("GATEWAY_CORE_TIMEOUT", "200ms")
	t.Setenv("GATEWAY_WRITE_TIMEOUT", "3s")
	t.Setenv("GATEWAY_CORS_ORIGINS", "https://app.example")
	t.Setenv("GATEWAY_SHUTDOWN_TIMEOUT", "1s")
	t.Setenv("REDIS_ADDR", redisAddr)
	t.Setenv("JWT_SECRET", secret)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWithContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("shutdown Gateway: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Gateway did not shut down")
		}
	})

	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		response, err := getWithContext(t, client, "http://"+httpAddr+"/healthz")
		if err == nil {
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Errorf("drain response body: %v", err)
			}
			if err := response.Body.Close(); err != nil {
				t.Errorf("close response body: %v", err)
			}
			if response.StatusCode == http.StatusOK {
				started = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !started {
		t.Fatal("Gateway did not start")
	}

	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub":  "123e4567-e89b-42d3-a456-426614174001",
		"jti":  "123e4567-e89b-42d3-a456-426614174002",
		"type": "ACCESS",
		"iat":  now.Add(-time.Minute).Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign access token: %v", err)
	}

	return "http://" + httpAddr, token, client
}
