package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestGatewayStartsWithoutRedisButIsNotReady(t *testing.T) {
	addresses := unusedTCPAddresses(t, 3)
	httpAddr, grpcAddr, redisAddr := addresses[0], addresses[1], addresses[2]
	t.Setenv("GATEWAY_HTTP_ADDR", httpAddr)
	t.Setenv("GATEWAY_GRPC_ADDR", grpcAddr)
	t.Setenv("GATEWAY_CORE_URL", "http://127.0.0.1:1")
	t.Setenv("REDIS_ADDR", redisAddr)
	t.Setenv("JWT_SECRET", "test-only-secret")
	t.Setenv("GATEWAY_SHUTDOWN_TIMEOUT", "1s")

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

	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + httpAddr + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	assertHTTPStatus(t, client, "http://"+httpAddr+"/healthz", http.StatusOK)
	assertHTTPStatus(t, client, "http://"+httpAddr+"/readyz", http.StatusServiceUnavailable)
}

func unusedTCPAddresses(t *testing.T, count int) []string {
	t.Helper()
	listeners := make([]net.Listener, 0, count)
	addresses := make([]string, 0, count)
	for range count {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
		addresses = append(addresses, listener.Addr().String())
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return addresses
}

func assertHTTPStatus(t *testing.T, client *http.Client, url string, want int) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		t.Errorf("GET %s: status = %d, want %d", url, resp.StatusCode, want)
	}
}
