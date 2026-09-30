package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/proxy"
)

func TestAuthAndGameUpstreamErrorsPassThrough(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"Auth 400", http.MethodPost, "/api/v1/auth/login", http.StatusBadRequest},
		{"Auth 404", http.MethodPost, "/api/v1/auth/login", http.StatusNotFound},
		{"Game 400", http.MethodGet, "/api/v1/rank-tiers", http.StatusBadRequest},
		{"Game 404", http.MethodGet, "/api/v1/rank-tiers", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const upstreamBody = "upstream-defined error\n"
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/problem+json")
				w.Header().Set("X-Upstream-Error", "original")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, upstreamBody)
			}))
			defer upstream.Close()

			handler := newServiceTransportHandler(t, upstream.URL, upstream.URL, time.Second)
			request := httptest.NewRequest(tc.method, tc.path, nil)
			if strings.HasPrefix(tc.name, "Game") {
				request.Header.Set("Authorization", "Bearer test-token")
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != tc.status || recorder.Body.String() != upstreamBody {
				t.Errorf("response = (%d, %q), want (%d, %q)",
					recorder.Code, recorder.Body.String(), tc.status, upstreamBody)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want upstream content type", got)
			}
			if got := recorder.Header().Get("X-Upstream-Error"); got != "original" {
				t.Errorf("X-Upstream-Error = %q, want original", got)
			}
			if got := recorder.Header().Get("X-Request-Id"); got != "service-test-id" {
				t.Errorf("X-Request-Id = %q, want service-test-id", got)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("upstream calls = %d, want 1", got)
			}
		})
	}
}

func TestAuthAndGameTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		timeout            bool
		wantStatus         int
	}{
		{"Auth connection failure", http.MethodPost, "/api/v1/auth/login", false, http.StatusBadGateway},
		{"Auth timeout", http.MethodPost, "/api/v1/auth/login", true, http.StatusGatewayTimeout},
		{"Game connection failure", http.MethodGet, "/api/v1/rank-tiers", false, http.StatusBadGateway},
		{"Game timeout", http.MethodGet, "/api/v1/rank-tiers", true, http.StatusGatewayTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				<-r.Context().Done()
			}))
			upstreamURL := upstream.URL
			if tc.timeout {
				defer upstream.Close()
			} else {
				upstream.Close()
			}

			handler := newServiceTransportHandler(t, upstreamURL, upstreamURL, 100*time.Millisecond)
			request := httptest.NewRequest(tc.method, tc.path, nil)
			if strings.HasPrefix(tc.name, "Game") {
				request.Header.Set("Authorization", "Bearer test-token")
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if got := recorder.Header().Get("X-Request-Id"); got != "service-test-id" {
				t.Errorf("X-Request-Id = %q, want service-test-id", got)
			}
			wantCalls := int32(0)
			if tc.timeout {
				wantCalls = 1
			}
			if got := calls.Load(); got != wantCalls {
				t.Errorf("upstream calls = %d, want %d", got, wantCalls)
			}
		})
	}
}

func TestAuthMutatingRequestIsNotRetriedAfterConnectionFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream connection: %v", err)
			return
		}
		if err := connection.Close(); err != nil {
			t.Errorf("close upstream connection: %v", err)
		}
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(newServiceTransportHandler(t, upstream.URL, upstream.URL, time.Second))
	defer gateway.Close()

	for _, wantStatus := range []int{http.StatusNoContent, http.StatusBadGateway} {
		request, err := http.NewRequest(http.MethodPost, gateway.URL+"/api/v1/auth/login", strings.NewReader("same-body"))
		if err != nil {
			t.Fatal(err)
		}
		response, err := gateway.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read response: %v; close response: %v", readErr, closeErr)
		}
		if response.StatusCode != wantStatus {
			t.Errorf("status = %d, want %d", response.StatusCode, wantStatus)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls = %d, want exactly 2", got)
	}
}

func newServiceTransportHandler(t *testing.T, authURL, gameURL string, timeout time.Duration) http.Handler {
	t.Helper()
	newProxy := func(rawURL string, rewrite proxy.RewriteFunc) http.Handler {
		t.Helper()
		target, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		reverseProxy, err := proxy.NewReverseProxy(target, rewrite, proxy.HandleUpstreamError)
		if err != nil {
			t.Fatal(err)
		}
		return reverseProxy
	}
	return httpapi.NewHandler(httpapi.HandlerOptions{
		Readiness:       &httpapi.Readiness{},
		CheckDependency: func(context.Context) error { return nil },
		CoreProxy:       http.NotFoundHandler(),
		AuthHandler:     newProxy(authURL, proxy.RewriteAuth),
		GameHandler:     newProxy(gameURL, proxy.RewriteGame),
		CoreTimeout:     time.Second,
		AuthTimeout:     timeout,
		GameTimeout:     timeout,
		WriteTimeout:    2 * time.Second,
		Verifier:        immediateAccessVerifier{},
		NewRequestID:    func() string { return "service-test-id" },
	})
}
