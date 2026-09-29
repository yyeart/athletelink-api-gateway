package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/auth"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

type Middleware func(http.Handler) http.Handler

type AccessVerifier interface {
	VerifyAccessToken(context.Context, string) (requestcontext.Identity, error)
}

func Chain(next http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		next = middleware[i](next)
	}

	return next
}

func withCoreTimeout(next http.Handler, timeout, writeTimeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := setWriteDeadline(w, time.Now().Add(writeTimeout)); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withRequestID(next http.Handler, newID func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newID()
		if id == "" {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("X-Request-Id", id)
		ctx := requestcontext.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withCoreAuth(next http.Handler, verifier AccessVerifier, writeTimeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requestcontext.RequestIDFrom(r.Context()); !ok {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		values := r.Header.Values("Authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(values[0], "Bearer ")
		if token == "" || strings.ContainsAny(token, " \t\r\n") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// The server deadline starts before authentication. Clear it while the
		// verifier runs so the full Core timeout remains available afterward.
		if err := setWriteDeadline(w, time.Time{}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		identity, err := verifier.VerifyAccessToken(r.Context(), token)
		if r.Context().Err() != nil {
			return
		}
		if deadlineErr := setWriteDeadline(w, time.Now().Add(writeTimeout)); deadlineErr != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, auth.ErrCheckUnavailable) {
				status = http.StatusServiceUnavailable
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
		if identity.UserID == "" {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		ctx := requestcontext.WithIdentity(r.Context(), identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func setWriteDeadline(w http.ResponseWriter, deadline time.Time) error {
	err := http.NewResponseController(w).SetWriteDeadline(deadline)
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

func withCorePreflight(next http.Handler, origins []string) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		method := r.Header.Get("Access-Control-Request-Method")
		w.Header().Add("Vary", "Origin")

		_, originAllowed := allowed[origin]
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		if r.Method != http.MethodOptions || origin == "" || method == "" {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")

		if !originAllowed {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Access-Control-Allow-Methods", method)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})
}
