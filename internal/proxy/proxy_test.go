package proxy_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/proxy"
)

type observedRequest struct {
	method   string
	rawQuery string
	body     string
	err      error
}

func TestReverseProxyPreservesRequestAndUpstreamResponse(t *testing.T) {
	t.Parallel()

	observed := make(chan observedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, request *http.Request) {
			body, err := io.ReadAll(request.Body)
			observed <- observedRequest{
				method:   request.Method,
				rawQuery: request.URL.RawQuery,
				body:     string(body),
				err:      err,
			}

			w.Header().Set("X-Upstream", "core-stub")
			w.WriteHeader(http.StatusCreated)
			if _, writeErr := io.WriteString(w, "created\n"); writeErr != nil {
				t.Errorf("upstream WriteString() error = %v", writeErr)
			}
		},
	))
	defer upstream.Close()

	reverseProxy := mustReverseProxy(t, upstream.URL, func(*httputil.ProxyRequest) {})
	gatewayServer := httptest.NewServer(reverseProxy)
	defer gatewayServer.Close()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		gatewayServer.URL+"/requests?sport=football&radius=5000",
		bytes.NewBufferString(`{"title":"training"}`),
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	response, err := gatewayServer.Client().Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response.Body) error = %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Errorf("response.Body.Close() error = %v", err)
	}

	if response.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", response.StatusCode, http.StatusCreated)
	}

	if got := response.Header.Get("X-Upstream"); got != "core-stub" {
		t.Errorf("X-Upstream = %q, want %q", got, "core-stub")
	}

	if got := string(responseBody); got != "created\n" {
		t.Errorf("body = %q, want %q", got, "created\n")
	}

	got := <-observed
	if got.err != nil {
		t.Fatalf("upstream ReadAll(request.Body) error = %v", got.err)
	}

	if got.method != http.MethodPost {
		t.Errorf("upstream method = %q, want %q", got.method, http.MethodPost)
	}

	if got.rawQuery != "sport=football&radius=5000" {
		t.Errorf("upstream query = %q", got.rawQuery)
	}

	if got.body != `{"title":"training"}` {
		t.Errorf("upstream body = %q", got.body)
	}
}

func TestReverseProxyPathIsControlledByRewrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rewrite proxy.RewriteFunc
		want    string
	}{
		{
			name:    "preserve external prefix",
			rewrite: func(*httputil.ProxyRequest) {},
			want:    "/external/requests",
		},
		{
			name: "remove external prefix",
			rewrite: func(request *httputil.ProxyRequest) {
				request.Out.URL.Path = strings.TrimPrefix(
					request.In.URL.Path,
					"/external",
				)
				request.Out.URL.RawPath = ""
			},
			want: "/requests",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			pathSeen := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, request *http.Request) {
					pathSeen <- request.URL.Path
					w.WriteHeader(http.StatusNoContent)
				},
			))
			defer upstream.Close()

			reverseProxy := mustReverseProxy(t, upstream.URL, test.rewrite)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"http://gateway.test/external/requests",
				nil,
			)
			reverseProxy.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
			}

			if got := <-pathSeen; got != test.want {
				t.Fatalf("upstream path = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReverseProxyCallsErrorHandlerForUnavailableUpstream(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {},
	))
	target := parseURL(t, upstream.URL)
	upstream.Close()

	errorSeen := make(chan error, 1)
	reverseProxy, err := proxy.NewReverseProxy(
		target,
		nil,
		func(w http.ResponseWriter, _ *http.Request, transportErr error) {
			errorSeen <- transportErr
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
	)
	if err != nil {
		t.Fatalf("NewReverseProxy() error = %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://gateway.test/requests",
		nil,
	)
	reverseProxy.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}

	select {
	case transportErr := <-errorSeen:
		if transportErr == nil {
			t.Fatal("ErrorHandler error = nil, want transport error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ErrorHandler was not called")
	}
}

func TestReverseProxyPropagatesClientCancellation(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	requestCanceled := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, request *http.Request) {
			close(requestStarted)
			<-request.Context().Done()
			requestCanceled <- request.Context().Err()
		},
	))
	defer upstream.Close()

	reverseProxy := mustReverseProxy(t, upstream.URL, nil)
	gatewayServer := httptest.NewServer(reverseProxy)
	defer gatewayServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		gatewayServer.URL+"/requests",
		nil,
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	clientResult := make(chan error, 1)
	go func() {
		response, requestErr := gatewayServer.Client().Do(request)
		if response != nil {
			closeErr := response.Body.Close()
			if requestErr == nil {
				requestErr = closeErr
			}
		}

		clientResult <- requestErr
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach upstream")
	}

	cancel()

	select {
	case requestErr := <-clientResult:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("client error = %v, want context canceled", requestErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client request did not stop after cancellation")
	}

	select {
	case upstreamErr := <-requestCanceled:
		if !errors.Is(upstreamErr, context.Canceled) {
			t.Fatalf("upstream context error = %v, want context canceled", upstreamErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream context was not canceled")
	}
}

func mustReverseProxy(
	t *testing.T,
	target string,
	rewrite proxy.RewriteFunc,
) http.Handler {
	t.Helper()

	reverseProxy, err := proxy.NewReverseProxy(
		parseURL(t, target),
		rewrite,
		func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	)
	if err != nil {
		t.Fatalf("NewReverseProxy() error = %v", err)
	}

	return reverseProxy
}

func parseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	target, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", rawURL, err)
	}

	return target
}
