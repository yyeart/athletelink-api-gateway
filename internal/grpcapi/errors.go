package grpcapi

import (
	"context"
	"errors"
	"net"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func businessFailure(
	err *coreclient.BusinessError,
	requestID string,
) error {
	var code codes.Code
	var publicCode string

	switch err.HTTPStatus {
	case 400:
		code, publicCode = codes.InvalidArgument, "INVALID_ARGUMENT"

	case 403:
		code, publicCode = codes.PermissionDenied, "PERMISSION_DENIED"

	case 404:
		code, publicCode = codes.NotFound, "NOT_FOUND"

	case 409:
		code, publicCode = codes.FailedPrecondition, "FAILED_PRECONDITION"

	default:
		return gatewayFailure(
			codes.Internal,
			"INTERNAL_ERROR",
			"Internal Gateway error",
			requestID,
		)
	}

	st, packErr := status.New(
		code,
		"Core request failed",
	).WithDetails(&gatewayv1.CoreErrorDetail{
		HttpStatus: int32(err.HTTPStatus),
		Code:       &publicCode,
		RequestId:  requestID,
	})
	if packErr != nil {
		return status.Error(codes.Internal, "Internal Gateway error")
	}

	return st.Err()
}

func gatewayFailure(
	grpcCode codes.Code,
	publicCode string,
	message string,
	requestID string,
) error {
	st, err := status.New(grpcCode, message).WithDetails(
		&gatewayv1.GatewayErrorDetail{
			Code:      publicCode,
			RequestId: requestID,
		},
	)
	if err != nil {
		return status.Error(codes.Internal, "Internal Gateway error")
	}

	return st.Err()
}

func mapCoreError(
	ctx context.Context,
	err error,
	requestID string,
) error {
	if err == nil {
		return nil
	}

	if contextErr := ctx.Err(); contextErr != nil {
		return mapContextError(contextErr, requestID)
	}

	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return mapContextError(err, requestID)
	}

	var businessErr *coreclient.BusinessError
	if errors.As(err, &businessErr) {
		return businessFailure(businessErr, requestID)
	}

	var contractErr *coreclient.ContractError
	if errors.As(err, &contractErr) {
		return gatewayFailure(
			codes.Internal,
			"UPSTREAM_CONTRACT_VIOLATION",
			"Invalid Core response",
			requestID,
		)
	}

	var transportErr *coreclient.TransportError
	if errors.As(err, &transportErr) {
		var networkErr net.Error
		if errors.As(transportErr, &networkErr) && networkErr.Timeout() {
			return gatewayFailure(
				codes.DeadlineExceeded,
				"DEADLINE_EXCEEDED",
				"Request deadline exceeded",
				requestID,
			)
		}

		return gatewayFailure(
			codes.Unavailable,
			"UPSTREAM_UNAVAILABLE",
			"Core service unavailable",
			requestID,
		)
	}

	return gatewayFailure(
		codes.Internal,
		"INTERNAL_ERROR",
		"Internal Gateway error",
		requestID,
	)
}

func mapContextError(err error, requestID string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return gatewayFailure(
			codes.DeadlineExceeded,
			"DEADLINE_EXCEEDED",
			"Request deadline exceeded",
			requestID,
		)
	}

	return gatewayFailure(
		codes.Canceled,
		"REQUEST_CANCELLED",
		"Request cancelled",
		requestID,
	)
}
