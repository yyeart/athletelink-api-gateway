package httpapi

import "net/http"

type Middleware func(http.Handler) http.Handler

func Chain(next http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		next = middleware[i](next)
	}

	return next
}
