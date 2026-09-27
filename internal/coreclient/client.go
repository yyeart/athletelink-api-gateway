package coreclient

import (
	"errors"
	"net/http"
	"net/url"
)

type Client struct {
	baseURL url.URL
	http    *http.Client
}

func New(baseURL *url.URL, httpClient *http.Client) (*Client, error) {
	if baseURL == nil || httpClient == nil {
		return nil, errors.New("core client dependencies are required")
	}

	client := *httpClient
	client.CheckRedirect = func(
		req *http.Request,
		via []*http.Request,
	) error {
		return http.ErrUseLastResponse
	}

	return &Client{
		baseURL: *baseURL,
		http:    &client,
	}, nil
}
