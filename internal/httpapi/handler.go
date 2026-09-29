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
	CoreTimeout     time.Duration
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

	timedCore := withCoreTimeout(options.CoreProxy, options.CoreTimeout, options.WriteTimeout)
	securedCore := withCoreAuth(timedCore, options.Verifier, options.WriteTimeout)
	coreHandler := withCorePreflight(securedCore, options.CORSOrigins)

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()

		switch {
		case path == "/requests",
			strings.HasPrefix(path, "/requests/"),
			path == "/sports":
			coreHandler.ServeHTTP(w, r)

		case path == "/healthz", path == "/readyz":
			healthMux.ServeHTTP(w, r)

		default:
			http.NotFound(w, r)
		}
	})

	return withRequestID(router, options.NewRequestID)
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
