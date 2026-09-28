package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

type corePath struct {
	path    string
	rawPath string
}

type successDecoder func(io.Reader) error

func (c *Client) callCore(
	ctx context.Context,
	method string,
	path corePath,
	query url.Values,
	body any,
	userID string,
	successStatus int,
	businessStatuses []int,
	decode successDecoder,
) error {
	requestID, ok := requestcontext.RequestIDFrom(ctx)
	if !ok {
		return errors.New("missing internal request ID")
	}

	endpoint := c.baseURL
	endpoint.Path = path.path
	endpoint.RawPath = path.rawPath
	endpoint.RawQuery = query.Encode()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}

		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Request-Id", requestID)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &TransportError{Cause: err}
	}
	resultErr := decodeCoreResponse(resp, successStatus, businessStatuses, decode)
	if closeErr := resp.Body.Close(); closeErr != nil {
		resultErr = errors.Join(resultErr, &TransportError{
			Cause: fmt.Errorf("close Core response body: %w", closeErr),
		})
	}
	return resultErr
}

func decodeCoreResponse(
	resp *http.Response,
	successStatus int,
	businessStatuses []int,
	decode successDecoder,
) error {
	if resp.StatusCode == successStatus {
		if decode == nil {
			return nil
		}

		if err := decode(resp.Body); err != nil {
			return &ContractError{Cause: err}
		}

		return nil
	}

	for _, allowed := range businessStatuses {
		if resp.StatusCode == allowed {
			if err := validateAPIError(resp.Body); err != nil {
				return &ContractError{Cause: err}
			}

			return &BusinessError{HTTPStatus: resp.StatusCode}
		}
	}

	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		return &TransportError{
			Cause: errors.New("core returned a server error"),
		}
	}

	return &ContractError{
		Cause: fmt.Errorf("unexpected Core HTTP status: %d", resp.StatusCode),
	}
}

func requestPath(id string) corePath {
	return corePath{
		path:    "/requests/" + id,
		rawPath: "/requests/" + url.PathEscape(id),
	}
}

func actionPath(requestID, suffix string) corePath {
	base := requestPath(requestID)
	base.path += "/" + suffix
	base.rawPath += "/" + suffix

	return base
}
