package coreclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (c *Client) GetRequestDetails(
	ctx context.Context,
	input GetRequestDetailsInput,
) (ActivityRequestDetails, error) {
	var result ActivityRequestDetails

	err := c.callCore(
		ctx, http.MethodGet, requestPath(input.RequestID),
		nil, nil, "", http.StatusOK,
		[]int{400, 404},
		func(r io.Reader) error {
			var err error
			result, err = decodeRequestDetails(r)
			return err
		},
	)

	return result, err
}

func (c *Client) UpdateRequest(
	ctx context.Context,
	input UpdateRequestInput,
) (ActivityRequestDetails, error) {
	body := map[string]any{}

	if input.Title != nil {
		body["title"] = *input.Title
	}
	if input.Description != nil {
		body["description"] = *input.Description
	}
	if input.EventDate != nil {
		value, err := timestampUTC(input.EventDate)
		if err != nil {
			return ActivityRequestDetails{}, err
		}

		body["eventDate"] = value
	}
	if input.NumberOfRounds != nil {
		body["numberOfRounds"] = *input.NumberOfRounds
	}
	if input.MaxPlayers != nil {
		body["maxPlayers"] = *input.MaxPlayers
	}

	var result ActivityRequestDetails

	err := c.callCore(
		ctx, http.MethodPut,
		requestPath(input.RequestID),
		nil, body, input.UserID,
		http.StatusOK, []int{400, 403, 404, 409},
		func(r io.Reader) error {
			var err error
			result, err = decodeRequestDetails(r)
			return err
		},
	)

	return result, err
}

func (c *Client) SearchNearbyRequests(
	ctx context.Context,
	input SearchNearbyRequestsInput,
) ([]ActivityRequestFeed, error) {
	query := url.Values{}

	if input.Lat != nil {
		query.Set("lat", strconv.FormatFloat(*input.Lat, 'g', -1, 64))
	}
	if input.Lon != nil {
		query.Set("lon", strconv.FormatFloat(*input.Lon, 'g', -1, 64))
	}
	if input.Radius != nil {
		query.Set("radius", strconv.FormatFloat(*input.Radius, 'g', -1, 64))
	}
	if input.SportID != nil {
		query.Set("sportId", strconv.FormatInt(*input.SportID, 10))
	}
	if input.StartDate != nil {
		value, err := timestampUTC(input.StartDate)
		if err != nil {
			return nil, err
		}
		query.Set("startDate", value)
	}
	if input.EndDate != nil {
		value, err := timestampUTC(input.EndDate)
		if err != nil {
			return nil, err
		}
		query.Set("endDate", value)
	}

	var result []ActivityRequestFeed

	err := c.callCore(
		ctx, http.MethodGet,
		corePath{
			path:    "/requests",
			rawPath: "",
		}, query,
		nil, "", http.StatusOK,
		[]int{http.StatusBadRequest},
		func(r io.Reader) error {
			var err error
			result, err = decodeRequestFeeds(r)
			return err
		},
	)

	return result, err
}

func (c *Client) CreateRequest(
	ctx context.Context,
	input CreateRequestInput,
) (string, error) {
	body := map[string]any{}

	if input.Title != nil {
		body["title"] = *input.Title
	}
	if input.Description != nil {
		body["description"] = *input.Description // в том числе ""
	}
	if input.SportID != nil {
		body["sportId"] = *input.SportID
	}
	if input.MaxPlayers != nil {
		body["maxPlayers"] = *input.MaxPlayers
	}
	if input.EventDate != nil {
		value, err := timestampUTC(input.EventDate)
		if err != nil {
			return "", err
		}
		body["eventDate"] = value
	}
	if input.NumberOfRounds != nil {
		body["numberOfRounds"] = *input.NumberOfRounds
	}
	if input.AddressText != nil {
		body["addressText"] = *input.AddressText
	}
	if input.Latitude != nil {
		body["latitude"] = *input.Latitude
	}
	if input.Longitude != nil {
		body["longitude"] = *input.Longitude
	}

	var createdID string

	err := c.callCore(
		ctx, http.MethodPost,
		corePath{
			path:    "/requests",
			rawPath: "",
		}, nil, body,
		*input.UserID, http.StatusCreated,
		[]int{http.StatusBadRequest, http.StatusNotFound},
		func(r io.Reader) error {
			var err error
			createdID, err = decodeCreatedRequestID(r)
			return err
		},
	)

	return createdID, err
}

func (c *Client) OpenRegistration(
	ctx context.Context,
	input ActionInput,
) error {
	return c.callCore(
		ctx, http.MethodPost, actionPath(input.RequestID, "registration/open"),
		nil, nil, input.UserID,
		http.StatusNoContent, []int{400, 403, 404, 409}, nil,
	)
}

func (c *Client) CloseRegistration(
	ctx context.Context,
	input ActionInput,
) error {
	return c.callCore(
		ctx, http.MethodPost, actionPath(input.RequestID, "registration/close"),
		nil, nil, input.UserID,
		http.StatusNoContent, []int{400, 403, 404, 409}, nil,
	)
}

func (c *Client) LeaveRequest(
	ctx context.Context,
	input ActionInput,
) error {
	return c.callCore(
		ctx, http.MethodPost, actionPath(input.RequestID, "leave"),
		nil, nil, input.UserID,
		http.StatusOK, []int{400, 404, 409}, nil,
	)
}

func (c *Client) JoinRequest(
	ctx context.Context,
	input ActionInput,
) error {
	return c.callCore(
		ctx, http.MethodPost, actionPath(input.RequestID, "join"),
		nil, nil, input.UserID,
		http.StatusOK, []int{400, 404, 409}, nil,
	)
}

func (c *Client) KickParticipant(
	ctx context.Context,
	input KickParticipantInput,
) error {
	path := actionPath(input.RequestID, "kick")
	path.path += "/" + input.TargetUserID
	path.rawPath += "/" + url.PathEscape(input.TargetUserID)

	return c.callCore(
		ctx, http.MethodPost, path,
		nil, nil, input.UserID,
		http.StatusOK, []int{400, 403, 404, 409}, nil,
	)
}

func decodeCreatedRequestID(r io.Reader) (string, error) {
	obj, err := readObject(r)
	if err != nil {
		return "", err
	}

	var id *string
	if err := readField(obj, "requestId", &id, parseUUID); err != nil {
		return "", err
	}

	if id == nil {
		return "", errors.New("missing requestId in Core response")
	}

	return *id, nil
}

func timestampUTC(value *time.Time) (string, error) {
	if value == nil {
		return "", nil
	}

	utc := value.UTC()
	if utc.Year() < 1 || utc.Year() > 9999 {
		return "", errors.New("timestamp outside protobuf range")
	}

	return utc.Format(time.RFC3339Nano), nil
}
