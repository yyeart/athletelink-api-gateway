package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	handler := httpapi.NewHandler(
		&httpapi.Readiness{},
		func(context.Context) error { return errors.New("redis unavailable") },
	)
	recorder := serveRequest(handler, http.MethodGet, "/healthz")

	assertResponse(t, recorder, http.StatusOK, "ok\n")
	assertContentType(t, recorder)
}

func TestReadinessEndpointTracksState(t *testing.T) {
	t.Parallel()

	readiness := &httpapi.Readiness{}
	var dependencyErr error
	handler := httpapi.NewHandler(readiness, func(context.Context) error {
		return dependencyErr
	})

	recorder := serveRequest(handler, http.MethodGet, "/readyz")
	assertResponse(t, recorder, http.StatusServiceUnavailable, "not ready\n")
	assertContentType(t, recorder)

	readiness.Set(true)
	recorder = serveRequest(handler, http.MethodGet, "/readyz")
	assertResponse(t, recorder, http.StatusOK, "ok\n")

	dependencyErr = errors.New("redis unavailable")
	recorder = serveRequest(handler, http.MethodGet, "/readyz")
	assertResponse(t, recorder, http.StatusServiceUnavailable, "not ready\n")

	dependencyErr = nil
	recorder = serveRequest(handler, http.MethodGet, "/readyz")
	assertResponse(t, recorder, http.StatusOK, "ok\n")

	readiness.Set(false)
	recorder = serveRequest(handler, http.MethodGet, "/readyz")
	assertResponse(t, recorder, http.StatusServiceUnavailable, "not ready\n")
}

func TestHealthEndpointRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	handler := httpapi.NewHandler(&httpapi.Readiness{}, healthyDependency)
	recorder := serveRequest(handler, http.MethodPost, "/healthz")

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerReturnsNotFoundForUnregisteredPaths(t *testing.T) {
	t.Parallel()

	handler := httpapi.NewHandler(&httpapi.Readiness{}, healthyDependency)
	paths := []string{"/", "/unknown", "/healthz/", "/readyz/"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			recorder := serveRequest(handler, http.MethodGet, path)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func serveRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		context.Background(),
		method,
		path,
		nil,
	)
	handler.ServeHTTP(recorder, request)

	return recorder
}

func healthyDependency(context.Context) error { return nil }

func assertResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantBody string,
) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Errorf("status = %d, want %d", recorder.Code, wantStatus)
	}

	if recorder.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", recorder.Body.String(), wantBody)
	}
}

func assertContentType(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	const want = "text/plain; charset=utf-8"
	if got := recorder.Header().Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}
