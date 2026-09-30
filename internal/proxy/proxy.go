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
		ModifyResponse: filterGatewayResponse,
		ErrorHandler:   onError,
	}

	return proxy, nil
}

func RewriteCore(pr *httputil.ProxyRequest) {
	rewriteAuthenticatedRequest(pr)
}

func RewriteAuth(pr *httputil.ProxyRequest) {
	rewriteRequest(pr, false)

	identity, identityOK := requestcontext.IdentityFrom(pr.In.Context())
	token, tokenOK := requestcontext.VerifiedAccessTokenFrom(pr.In.Context())
	if identityOK && tokenOK {
		pr.Out.Header.Set("Authorization", "Bearer "+token)
		pr.Out.Header.Set("X-User-Id", identity.UserID)
	}
}

func RewriteGame(pr *httputil.ProxyRequest) {
	rewriteAuthenticatedRequest(pr)
}

func rewriteAuthenticatedRequest(pr *httputil.ProxyRequest) {
	rewriteRequest(pr, true)

	identity, _ := requestcontext.IdentityFrom(pr.In.Context())
	pr.Out.Header.Set("X-User-Id", identity.UserID)
}

func rewriteRequest(pr *httputil.ProxyRequest, stripCookie bool) {
	in, out := pr.In, pr.Out
	out.URL.Path = in.URL.Path
	out.URL.RawPath = in.URL.RawPath
	out.URL.RawQuery = in.URL.RawQuery
	out.URL.ForceQuery = in.URL.ForceQuery

	for name := range out.Header {
		lower := strings.ToLower(name)
		if lower == "authorization" ||
			(stripCookie && lower == "cookie") ||
			lower == "x-user-id" ||
			lower == "x-request-id" ||
			lower == "forwarded" ||
			lower == "x-real-ip" ||
			strings.HasPrefix(lower, "x-forwarded-") {
			delete(out.Header, name)
		}
	}

	requestID, _ := requestcontext.RequestIDFrom(in.Context())
	out.Header.Set("X-Request-Id", requestID)
}

func filterGatewayResponse(resp *http.Response) error {
	for name := range resp.Header {
		lower := strings.ToLower(name)
		if lower == "x-request-id" || strings.HasPrefix(lower, "access-control-") {
			delete(resp.Header, name)
		}
	}
	return nil
}

func HandleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
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
