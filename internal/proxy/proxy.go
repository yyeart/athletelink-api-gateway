package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
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

	proxy := &httputil.ReverseProxy{
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
