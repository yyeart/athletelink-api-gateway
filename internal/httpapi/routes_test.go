package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

type routeVerifier struct{}

func (routeVerifier) VerifyAccessToken(context.Context, string) (requestcontext.Identity, error) {
	return requestcontext.Identity{UserID: "123e4567-e89b-42d3-a456-426614174001"}, nil
}

func TestAuthAndGameOperationRoutes(t *testing.T) {
	t.Parallel()

	const userID = "123e4567-e89b-42d3-a456-426614174001"
	tests := []struct {
		method   string
		path     string
		upstream string
	}{
		{http.MethodPut, "/api/v1/user/update-user/" + userID, "auth"},
		{http.MethodPut, "/api/v1/user/change-password/" + userID, "auth"},
		{http.MethodPut, "/api/v1/user/change-email/" + userID, "auth"},
		{http.MethodPost, "/api/v1/verification/verify-password-reset", "auth"},
		{http.MethodPost, "/api/v1/verification/verify-email/" + userID, "auth"},
		{http.MethodPost, "/api/v1/verification/send-password-reset-code", "auth"},
		{http.MethodPost, "/api/v1/verification/send-email-verification-code/" + userID, "auth"},
		{http.MethodPost, "/api/v1/user/register", "auth"},
		{http.MethodPost, "/api/v1/auth/refresh", "auth"},
		{http.MethodPost, "/api/v1/auth/login", "auth"},
		{http.MethodGet, "/api/v1/session/get-all/" + userID, "auth"},
		{http.MethodDelete, "/api/v1/user/delete/" + userID, "auth"},
		{http.MethodDelete, "/api/v1/session/terminate/" + userID, "auth"},
		{http.MethodDelete, "/api/v1/session/terminate-all/" + userID, "auth"},
		{http.MethodDelete, "/api/v1/auth/logout", "auth"},
		{http.MethodGet, "/api/v1/results/by-request/" + userID, "game"},
		{http.MethodGet, "/api/v1/rank-tiers", "game"},
		{http.MethodGet, "/api/v1/players/" + userID, "game"},
		{http.MethodGet, "/api/v1/players/" + userID + "/sports/7", "game"},
		{http.MethodGet, "/api/v1/players/" + userID + "/matches", "game"},
		{http.MethodGet, "/api/v1/leaderboards/7", "game"},
	}
	if len(tests) != 21 {
		t.Fatalf("operation cases = %d, want 21", len(tests))
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			calls := 0
			gotUpstream, gotMethod, gotURI := "", "", ""
			makeHandler := func(upstream string) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					gotUpstream, gotMethod, gotURI = upstream, r.Method, r.URL.RequestURI()
					w.WriteHeader(http.StatusAccepted)
				})
			}
			handler := newOperationRoutingHandler(makeHandler("auth"), makeHandler("game"))
			uri := tc.path + "?cursor=abc%2Fdef"
			request := httptest.NewRequestWithContext(t.Context(), tc.method, uri, nil)
			request.Header.Set("Authorization", "Bearer test-token")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusAccepted || calls != 1 ||
				gotUpstream != tc.upstream || gotMethod != tc.method || gotURI != uri {
				t.Errorf("status = %d, calls = %d, upstream = %q, method = %q, URI = %q; want 202, 1, %q, %q, %q",
					recorder.Code, calls, gotUpstream, gotMethod, gotURI, tc.upstream, tc.method, uri)
			}
			if got := recorder.Header().Get("X-Request-Id"); got != "gateway-request-id" {
				t.Errorf("X-Request-Id = %q, want gateway-request-id", got)
			}

			wrongMethod := http.MethodGet
			if tc.method == http.MethodGet {
				wrongMethod = http.MethodHead
			}
			wrong := serveRequest(handler, wrongMethod, tc.path)
			if wrong.Code != http.StatusMethodNotAllowed || wrong.Header().Get("Allow") != tc.method || calls != 1 {
				t.Errorf("wrong method: status = %d, Allow = %q, calls = %d; want 405, %q, 1",
					wrong.Code, wrong.Header().Get("Allow"), calls, tc.method)
			}
		})
	}
}

func TestAuthAndGameRouteBoundaries(t *testing.T) {
	t.Parallel()

	handler := newOperationRoutingHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }),
	)
	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/auth/unknown",
		"/api/v1/user/update-user/",
		"/api/v1/user/update-user/a/b",
		"/api/v1/user/update-user/a%2Fb",
		"/api/v1/user/update-user/.",
		"/api/v1/players-extra/123",
		"/api/v1/players/123/",
		"/api/v1/players//matches",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if got := serveRequest(handler, http.MethodGet, path).Code; got != http.StatusNotFound {
				t.Errorf("status = %d, want 404", got)
			}
		})
	}
}

func TestAuthAndGamePreflightChecksDeclaredMethod(t *testing.T) {
	t.Parallel()

	calls := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	})
	handler := newOperationRoutingHandler(upstream, upstream)
	for _, tc := range []struct {
		path            string
		requestedMethod string
		wantStatus      int
		wantAllow       string
	}{
		{"/api/v1/auth/login", http.MethodPost, http.StatusNoContent, ""},
		{"/api/v1/auth/login", http.MethodGet, http.StatusMethodNotAllowed, http.MethodPost},
		{"/api/v1/players/123", http.MethodGet, http.StatusNoContent, ""},
		{"/api/v1/health", http.MethodGet, http.StatusNotFound, ""},
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, tc.path, nil)
		request.Header.Set("Origin", "https://app.example")
		request.Header.Set("Access-Control-Request-Method", tc.requestedMethod)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != tc.wantStatus || recorder.Header().Get("Allow") != tc.wantAllow {
			t.Errorf("OPTIONS %s (%s): status = %d, Allow = %q; want %d, %q",
				tc.path, tc.requestedMethod, recorder.Code, recorder.Header().Get("Allow"), tc.wantStatus, tc.wantAllow)
		}
	}
	if calls != 0 {
		t.Errorf("upstream calls = %d, want 0", calls)
	}
}

func newOperationRoutingHandler(authHandler, gameHandler http.Handler) http.Handler {
	return httpapi.NewHandler(httpapi.HandlerOptions{
		Readiness:       &httpapi.Readiness{},
		CheckDependency: func(context.Context) error { return nil },
		CoreProxy:       http.NotFoundHandler(),
		AuthHandler:     authHandler,
		GameHandler:     gameHandler,
		CoreTimeout:     time.Second,
		AuthTimeout:     time.Second,
		GameTimeout:     time.Second,
		WriteTimeout:    2 * time.Second,
		Verifier:        routeVerifier{},
		NewRequestID:    func() string { return "gateway-request-id" },
		CORSOrigins:     []string{"https://app.example"},
	})
}
