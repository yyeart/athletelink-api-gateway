package coreclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

func (c *Client) GetRequestDetails(
	ctx context.Context,
	input GetRequestDetailsInput,
) (ActivityRequestDetails, error) {
	var zero ActivityRequestDetails

	requestID, ok := requestcontext.RequestIDFrom(ctx)
	if !ok {
		return zero, errors.New("missing internal request ID")
	}

	endpoint := c.baseURL
	endpoint.Path = "/requests/" + input.RequestID
	endpoint.RawPath = "/requests/" + url.PathEscape(input.RequestID)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return zero, err
	}

	req.Header.Set("X-Request-Id", requestID)

	resp, err := c.http.Do(req)
	if err != nil {
		return zero, &TransportError{Cause: err}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		result, err := decodeRequestDetails(resp.Body)
		if err != nil {
			return zero, &ContractError{Cause: err}
		}

		return result, nil

	case http.StatusBadRequest, http.StatusNotFound:
		if err := validateAPIError(resp.Body); err != nil {
			return zero, &ContractError{Cause: err}
		}

		return zero, &BusinessError{HTTPStatus: resp.StatusCode}

	default:
		if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
			return zero, &TransportError{
				Cause: errors.New("core service returned service error"),
			}
		}

		return zero, &ContractError{
			Cause: fmt.Errorf(
				"unexpected core status: %d",
				resp.StatusCode,
			),
		}
	}
}
