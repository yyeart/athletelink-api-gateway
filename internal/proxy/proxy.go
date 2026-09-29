package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

type RewriteFunc func(*httputil.ProxyRequest)

type ErrorHandler func(
	http.ResponseWriter,
	*http.Request,
	error,
)

func NewReverseProxy(
	target *url.URL,
	rewrite RewriteFunc,
	onError ErrorHandler,
) (*httputil.ReverseProxy, error) {
	if target == nil {
		return nil, errors.New("proxy.NewReverseProxy: nil target")
	}

	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf(
			"proxy.NewReverseProxy: unsupported protocol scheme: %s",
			target.Scheme,
		)
	}

	if target.Host == "" {
		return nil, errors.New(
			"proxy.NewReverseProxy: empty target host",
		)
	}

	if onError == nil {
		return nil, errors.New(
			"proxy.NewReverseProxy: nil onError func",
		)
	}

	targetCopy := *target

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("unexpected default HTTP transport")
	}

	transport := base.Clone()
	transport.DisableKeepAlives = true
	transport.ForceAttemptHTTP2 = false
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)

	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(&targetCopy)
			if rewrite != nil {
				rewrite(request)
			}
		},
		ErrorHandler: onError,
	}

	return proxy, nil
}

func RewriteCore(pr *httputil.ProxyRequest) {
	in, out := pr.In, pr.Out

	out.URL.Path = in.URL.Path
	out.URL.RawPath = in.URL.RawPath
	out.URL.RawQuery = in.URL.RawQuery
	out.URL.ForceQuery = in.URL.ForceQuery

	for name := range out.Header {
		lower := strings.ToLower(name)
		if lower == "authorization" ||
			lower == "cookie" ||
			lower == "x-user-id" ||
			lower == "x-request-id" ||
			lower == "forwarded" ||
			lower == "x-real-ip" ||
			strings.HasPrefix(lower, "x-forwarded-") {
			delete(out.Header, name)
		}
	}

	identity, _ := requestcontext.IdentityFrom(in.Context())
	requestID, _ := requestcontext.RequestIDFrom(in.Context())
	out.Header.Set("X-User-Id", identity.UserID)
	out.Header.Set("X-Request-Id", requestID)
}

func HandleCoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}

	status := http.StatusBadGateway

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(r.Context().Err(), context.DeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout()) {
		status = http.StatusGatewayTimeout
	}

	if requestID, ok := requestcontext.RequestIDFrom(r.Context()); ok {
		w.Header().Set("X-Request-Id", requestID)
	}

	http.Error(w, http.StatusText(status), status)
}
