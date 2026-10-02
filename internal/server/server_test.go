package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/server"
)

const testWaitTimeout = 2 * time.Second

type requestResult struct {
	status int
	body   string
	err    error
}

func TestRunMarksReadinessAndShutsDownGracefully(t *testing.T) {
	t.Parallel()

	address := availableAddress(t)
	stateChanges := make(chan bool, 4)
	var ready atomic.Bool

	srv := server.New(
		testConfig(address, time.Second),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		func(value bool) {
			ready.Store(value)
			stateChanges <- value
		},
		discardLogger(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	waitForState(t, stateChanges, true)
	if !ready.Load() {
		t.Fatal("readiness = false after listener startup, want true")
	}

	cancel()
	waitForState(t, stateChanges, false)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(testWaitTimeout):
		t.Fatal("Run() did not finish after context cancellation")
	}

	if ready.Load() {
		t.Fatal("readiness = true after shutdown, want false")
	}
}

func TestRunReturnsServiceUnavailableFromReadinessDuringShutdown(t *testing.T) {
	t.Parallel()

	address := availableAddress(t)
	readiness := &httpapi.Readiness{}
	readinessRequestStarted := make(chan struct{})
	allowReadinessResponse := make(chan struct{})
	readinessResponseReleased := false
	defer func() {
		if !readinessResponseReleased {
			close(allowReadinessResponse)
		}
	}()

	stateChanges := make(chan bool, 4)

	handler := httpapi.NewHandler(httpapi.HandlerOptions{
		Readiness:       readiness,
		CheckDependency: func(context.Context) error { return nil },
		CoreProxy:       http.NotFoundHandler(),
		CoreTimeout:     time.Second,
		WriteTimeout:    2 * time.Second,
		NewRequestID:    func() string { return "test-request-id" },
	})
	gatedHandler := http.HandlerFunc(
		func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/readyz" {
				close(readinessRequestStarted)
				<-allowReadinessResponse
			}

			handler.ServeHTTP(w, request)
		},
	)

	srv := server.New(
		testConfig(address, time.Second),
		gatedHandler,
		func(value bool) {
			readiness.Set(value)
			stateChanges <- value
		},
		discardLogger(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	waitForState(t, stateChanges, true)

	requestCtx, cancelRequest := context.WithTimeout(
		context.Background(),
		testWaitTimeout,
	)
	defer cancelRequest()

	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodGet,
		"http://"+address+"/readyz",
		nil,
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	requestResultCh := startRequest(request)
	waitForSignal(
		t,
		readinessRequestStarted,
		"request did not reach readiness handler",
	)

	cancel()
	waitForState(t, stateChanges, false)
	close(allowReadinessResponse)
	readinessResponseReleased = true

	assertRequestResult(
		t,
		requestResultCh,
		http.StatusServiceUnavailable,
		"not ready\n",
	)
	assertServerStopped(t, errCh)
}

func TestRunShutdownTimeoutDoesNotHang(t *testing.T) {
	t.Parallel()

	address := availableAddress(t)
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	stateChanges := make(chan bool, 4)
	var ready atomic.Bool

	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)

		select {
		case <-releaseRequest:
		case <-request.Context().Done():
		}
	})

	const shutdownTimeout = 25 * time.Millisecond
	srv := server.New(
		testConfig(address, shutdownTimeout),
		handler,
		func(value bool) {
			ready.Store(value)
			stateChanges <- value
		},
		discardLogger(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	waitForState(t, stateChanges, true)

	requestCtx, cancelRequest := context.WithTimeout(
		context.Background(),
		testWaitTimeout,
	)
	defer cancelRequest()

	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodGet,
		"http://"+address,
		nil,
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	clientResult := make(chan error, 1)
	go func() {
		response, requestErr := http.DefaultClient.Do(request)
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
	case <-time.After(testWaitTimeout):
		t.Fatal("request did not reach HTTP handler")
	}

	cancel()
	waitForState(t, stateChanges, false)

	select {
	case runErr := <-errCh:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("Run() error = %v, want context deadline exceeded", runErr)
		}
	case <-time.After(testWaitTimeout):
		t.Fatal("Run() hung after shutdown timeout")
	}

	close(releaseRequest)

	select {
	case <-clientResult:
	case <-time.After(testWaitTimeout):
		t.Fatal("client request did not finish after forced server close")
	}

	if ready.Load() {
		t.Fatal("readiness = true after timed-out shutdown, want false")
	}
}

func testConfig(address string, shutdownTimeout time.Duration) config.Config {
	return config.Config{
		HTTPAddr:          address,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   shutdownTimeout,
		LogLevel:          slog.LevelInfo,
	}
}

func availableAddress(t *testing.T) string {
	t.Helper()

	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(
		context.Background(),
		"tcp",
		"127.0.0.1:0",
	)
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}

	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}

	return address
}

func waitForState(t *testing.T, changes <-chan bool, want bool) {
	t.Helper()

	for {
		select {
		case got := <-changes:
			if got == want {
				return
			}
		case <-time.After(testWaitTimeout):
			t.Fatalf("readiness did not become %t", want)
		}
	}
}

func startRequest(request *http.Request) <-chan requestResult {
	resultCh := make(chan requestResult, 1)

	go func() {
		resultCh <- executeRequest(request)
	}()

	return resultCh
}

func executeRequest(request *http.Request) requestResult {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return requestResult{err: err}
	}

	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		err = readErr
	} else if closeErr != nil {
		err = closeErr
	}

	return requestResult{
		status: response.StatusCode,
		body:   string(body),
		err:    err,
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, timeoutMessage string) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(testWaitTimeout):
		t.Fatal(timeoutMessage)
	}
}

func assertRequestResult(
	t *testing.T,
	resultCh <-chan requestResult,
	wantStatus int,
	wantBody string,
) {
	t.Helper()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("readiness request error = %v", result.err)
		}
		if result.status != wantStatus {
			t.Errorf("readiness status = %d, want %d", result.status, wantStatus)
		}
		if result.body != wantBody {
			t.Errorf("readiness body = %q, want %q", result.body, wantBody)
		}
	case <-time.After(testWaitTimeout):
		t.Fatal("readiness request did not finish during shutdown")
	}
}

func assertServerStopped(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case runErr := <-errCh:
		if runErr != nil {
			t.Fatalf("Run() error = %v", runErr)
		}
	case <-time.After(testWaitTimeout):
		t.Fatal("Run() did not finish after readiness response")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
