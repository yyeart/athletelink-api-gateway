package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

type HandlerOptions struct {
	Readiness       *Readiness
	CheckDependency func(context.Context) error
	CoreProxy       http.Handler
	AuthHandler     http.Handler
	GameHandler     http.Handler
	CoreTimeout     time.Duration
	AuthTimeout     time.Duration
	GameTimeout     time.Duration
	WriteTimeout    time.Duration
	Verifier        AccessVerifier
	NewRequestID    func() string
	CORSOrigins     []string
	Logger          *slog.Logger
}

func NewHandler(options HandlerOptions) http.Handler {
	if options.CheckDependency == nil {
		panic("httpapi.NewHandler: nil dependency check")
	}

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /healthz", health)
	healthMux.HandleFunc("GET /readyz", ready(options.Readiness, options.CheckDependency))
	documentation := newDocumentationHandler()

	timedCore := withUpstreamTimeout(availableHandler(options.CoreProxy), options.CoreTimeout, options.WriteTimeout)
	securedCore := withAccessAuth(timedCore, options.Verifier, options.WriteTimeout)
	authTarget := withUpstreamTimeout(availableHandler(options.AuthHandler), options.AuthTimeout, options.WriteTimeout)
	authProtected := withAccessAuth(authTarget, options.Verifier, options.WriteTimeout)
	authCurrentUser := withAccessAuth(withCurrentUser(authTarget), options.Verifier, options.WriteTimeout)
	authOwnUser := withAccessAuth(withOwnUser(authTarget), options.Verifier, options.WriteTimeout)
	gameTarget := withUpstreamTimeout(availableHandler(options.GameHandler), options.GameTimeout, options.WriteTimeout)
	gameHandler := withAccessAuth(gameTarget, options.Verifier, options.WriteTimeout)

	apiHandler := withAPIPreflight(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match, matched := findAPIRoute(r.Method, r.URL.EscapedPath())
		if !matched {
			requestcontext.UpdateDiagnostics(r.Context(), func(d *requestcontext.DiagnosticFields) {
				d.ErrorKind = "method_not_allowed"
			})
			w.Header().Set("Allow", allowedMethods(r.URL.EscapedPath()))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		switch match.spec.upstream {
		case authUpstream:
			switch match.spec.access {
			case publicAccess:
				authTarget.ServeHTTP(w, r)
			case currentUserAccess:
				authCurrentUser.ServeHTTP(w, r)
			case ownUserAccess:
				authOwnUser.ServeHTTP(w, r)
			default:
				authProtected.ServeHTTP(w, r)
			}
		case coreUpstream:
			securedCore.ServeHTTP(w, r)
		case gameUpstream:
			gameHandler.ServeHTTP(w, r)
		}
	}), options.CORSOrigins)

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()

		switch {
		case path == "/healthz", path == "/readyz":
			healthMux.ServeHTTP(w, r)

		case path == "/openapi.yaml", path == "/swagger", strings.HasPrefix(path, "/swagger/"):
			documentation.ServeHTTP(w, r)

		default:
			if _, ok := findAPIRoute("", path); !ok {
				requestcontext.UpdateDiagnostics(r.Context(), func(d *requestcontext.DiagnosticFields) {
					d.ErrorKind = "route_not_found"
				})
				http.NotFound(w, r)
				return
			}
			apiHandler.ServeHTTP(w, r)
		}
	})

	return withRequestLogging(withRequestID(router, options.NewRequestID), options.Logger)
}

func availableHandler(handler http.Handler) http.Handler {
	if handler != nil {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestcontext.UpdateDiagnostics(r.Context(), func(d *requestcontext.DiagnosticFields) {
			d.ErrorKind = "upstream_not_configured"
		})
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	writePlainText(w, http.StatusOK, "ok\n")
}

func ready(
	readiness *Readiness,
	checkDependency func(context.Context) error,
) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if !readiness.IsReady() || checkDependency(request.Context()) != nil {
			requestcontext.UpdateDiagnostics(request.Context(), func(d *requestcontext.DiagnosticFields) {
				if d.ErrorKind == "" {
					d.ErrorKind = "not_ready"
				}
			})
			writePlainText(
				w,
				http.StatusServiceUnavailable,
				"not ready\n",
			)

			return
		}

		writePlainText(w, http.StatusOK, "ok\n")
	}
}

func writePlainText(
	w http.ResponseWriter,
	status int,
	body string,
) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)

	if _, err := io.WriteString(w, body); err != nil {
		return
	}
}
