package httpapi

import (
	"context"
	"io"
	"net/http"
)

func NewHandler(
	readiness *Readiness,
	checkDependency func(context.Context) error,
) http.Handler {
	if checkDependency == nil {
		panic("httpapi.NewHandler: nil dependency check")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", ready(readiness, checkDependency))

	return Chain(mux) // TODO: ADD MIDDLEWARES
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
