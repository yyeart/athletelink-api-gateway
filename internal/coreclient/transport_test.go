package coreclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

type closeErrorBody struct {
	io.Reader
	err error
}

func (body *closeErrorBody) Close() error { return body.err }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCallCoreReportsResponseBodyCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	baseURL, err := url.Parse("http://core.example")
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(baseURL, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       &closeErrorBody{Reader: strings.NewReader(""), err: closeErr},
		}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	ctx := requestcontext.WithRequestID(context.Background(), "test-request")
	err = client.callCore(ctx, http.MethodGet, corePath{path: "/sports"},
		nil, nil, "", http.StatusNoContent, nil, nil)
	var transportErr *TransportError
	if !errors.As(err, &transportErr) || !errors.Is(err, closeErr) {
		t.Errorf("callCore() error = %v, want transport error wrapping close failure", err)
	}
}
