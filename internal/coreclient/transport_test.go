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

type failingBody struct {
	readErr   error
	closeErr  error
	delivered bool
}

func (body *failingBody) Read(p []byte) (int, error) {
	if !body.delivered {
		body.delivered = true
		return copy(p, `{"code":`), nil
	}
	return 0, body.readErr
}

func (body *failingBody) Close() error { return body.closeErr }

type timeoutError struct{}

func (timeoutError) Error() string   { return "read timed out" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type roundTripFunc func(*http.Request) (*http.Response, error)

type readFailureCase struct {
	name             string
	status           int
	businessStatuses []int
	decode           successDecoder
	readErr          error
	closeErr         error
}

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

func TestCallCoreClassifiesResponseBodyReadErrors(t *testing.T) {
	readFailure := errors.New("connection reset during read")
	closeFailure := errors.New("close failed")
	tests := []readFailureCase{
		{name: "success timeout", status: http.StatusOK, decode: validateAPIError, readErr: timeoutError{}},
		{name: "success network failure", status: http.StatusOK, decode: validateAPIError, readErr: readFailure},
		{name: "business timeout", status: http.StatusBadRequest, businessStatuses: []int{http.StatusBadRequest}, readErr: timeoutError{}},
		{name: "business network failure and close failure", status: http.StatusBadRequest, businessStatuses: []int{http.StatusBadRequest}, readErr: readFailure, closeErr: closeFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testReadFailureCase(t, tt)
		})
	}
}

func testReadFailureCase(t *testing.T, tt readFailureCase) {
	t.Helper()
	baseURL, err := url.Parse("http://core.example")
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(baseURL, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: tt.status,
			Body: &failingBody{
				readErr: tt.readErr, closeErr: tt.closeErr,
			},
		}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	ctx := requestcontext.WithRequestID(context.Background(), "test-request")
	err = client.callCore(ctx, http.MethodGet, corePath{path: "/sports"},
		nil, nil, "", http.StatusOK, tt.businessStatuses, tt.decode)
	var transportErr *TransportError
	if !errors.As(err, &transportErr) || !errors.Is(err, tt.readErr) {
		t.Fatalf("callCore() error = %v, want transport error wrapping read failure", err)
	}
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		t.Fatalf("callCore() error = %v, must not classify read failure as contract violation", err)
	}
	if tt.closeErr != nil && !errors.Is(err, tt.closeErr) {
		t.Fatalf("callCore() error = %v, want joined close failure", err)
	}
}

func TestDecodeCoreResponseMalformedJSONIsContractError(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		resp := &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"code":`))}
		err := decodeCoreResponse(resp, http.StatusOK, []int{http.StatusBadRequest}, validateAPIError)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) {
			t.Errorf("status %d: error = %v, want contract error", status, err)
		}
		var transportErr *TransportError
		if errors.As(err, &transportErr) {
			t.Errorf("status %d: error = %v, must not classify malformed JSON as transport failure", status, err)
		}
	}
}
