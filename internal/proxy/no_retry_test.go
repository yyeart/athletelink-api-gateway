package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/proxy"
)

func TestReverseProxyDoesNotRetryFailedGET(t *testing.T) {
	var coreRequests atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if coreRequests.Add(1) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer cannot hijack connection")
			return
		}
		connection, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack Core connection: %v", err)
			return
		}
		if buffered == nil {
			t.Error("hijack returned no buffered connection")
		}
		if err := connection.Close(); err != nil {
			t.Errorf("close Core connection: %v", err)
		}
	}))
	defer core.Close()

	reverseProxy, err := proxy.NewReverseProxy(
		parseURL(t, core.URL),
		nil,
		func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusBadGateway)
		},
	)
	if err != nil {
		t.Fatalf("NewReverseProxy() error = %v", err)
	}

	transport, ok := reverseProxy.Transport.(*http.Transport)
	if !ok || !transport.DisableKeepAlives || transport.Protocols == nil ||
		!transport.Protocols.HTTP1() || transport.Protocols.HTTP2() {
		t.Fatalf("proxy transport = %#v, want HTTP/1 without connection reuse", reverseProxy.Transport)
	}

	gateway := httptest.NewServer(reverseProxy)
	defer gateway.Close()
	client := gateway.Client()
	for _, wantStatus := range []int{http.StatusNoContent, http.StatusBadGateway} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, gateway.URL+"/api/v1/requests", nil)
		if err != nil {
			t.Fatalf("create Gateway request: %v", err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("call Gateway: %v", err)
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
	if got := coreRequests.Load(); got != 2 {
		t.Errorf("Core received %d requests, want exactly 2", got)
	}
}
