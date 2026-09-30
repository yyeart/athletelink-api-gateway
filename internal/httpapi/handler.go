package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
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
}

func NewHandler(options HandlerOptions) http.Handler {
	if options.CheckDependency == nil {
		panic("httpapi.NewHandler: nil dependency check")
	}

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /healthz", health)
	healthMux.HandleFunc("GET /readyz", ready(options.Readiness, options.CheckDependency))

	timedCore := withUpstreamTimeout(options.CoreProxy, options.CoreTimeout, options.WriteTimeout)
	securedCore := withAccessAuth(timedCore, options.Verifier, options.WriteTimeout)
	coreHandler := withCorePreflight(securedCore, options.CORSOrigins)
	authTarget := withUpstreamTimeout(availableHandler(options.AuthHandler), options.AuthTimeout, options.WriteTimeout)
	authProtected := withAccessAuth(authTarget, options.Verifier, options.WriteTimeout)
	authOwnUser := withAccessAuth(withOwnUser(authTarget), options.Verifier, options.WriteTimeout)
	gameTarget := withUpstreamTimeout(availableHandler(options.GameHandler), options.GameTimeout, options.WriteTimeout)
	gameHandler := withAccessAuth(gameTarget, options.Verifier, options.WriteTimeout)

	apiHandler := withAPIPreflight(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match, _ := findAPIRoute(r.URL.EscapedPath())
		if r.Method != match.spec.method {
			w.Header().Set("Allow", match.spec.method)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		if match.spec.upstream == authUpstream {
			switch match.spec.access {
			case publicAccess:
				authTarget.ServeHTTP(w, r)
			case ownUserAccess:
				authOwnUser.ServeHTTP(w, r)
			default:
				authProtected.ServeHTTP(w, r)
			}
		} else {
			gameHandler.ServeHTTP(w, r)
		}
	}), options.CORSOrigins)

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()

		switch {
		case path == "/api/v1/requests",
			strings.HasPrefix(path, "/api/v1/requests/"),
			path == "/api/v1/sports":
			coreHandler.ServeHTTP(w, r)

		case path == "/healthz", path == "/readyz":
			healthMux.ServeHTTP(w, r)

		default:
			if _, ok := findAPIRoute(path); !ok {
				http.NotFound(w, r)
				return
			}
			apiHandler.ServeHTTP(w, r)
		}
	})

	return withRequestID(router, options.NewRequestID)
}

func availableHandler(handler http.Handler) http.Handler {
	if handler != nil {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
