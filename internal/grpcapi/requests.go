package grpcapi

import (
	"context"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (s *Server) GetRequestDetails(
	ctx context.Context,
	req *gatewayv1.GetRequestDetailsRequest,
) (*gatewayv1.GetRequestDetailsResponse, error) {
	requestID, ok := requestcontext.RequestIDFrom(ctx)
	if !ok {
		return nil, status.Error(
			codes.Internal,
			"Internal Gateway error",
		)
	}

	if err := grpc.SetHeader(
		ctx,
		metadata.Pairs("x-request-id", requestID),
	); err != nil {
		return nil, gatewayFailure(
			codes.Internal,
			"INTERNAL_ERROR",
			"Internal Gateway error",
			requestID,
		)
	}

	if _, ok := requestcontext.IdentityFrom(ctx); !ok {
		return nil, gatewayFailure(
			codes.Unauthenticated,
			"AUTHENTICATION_REQUIRED",
			"Authentication required",
			requestID,
		)
	}

	if req == nil || req.RequestId == nil || req.GetRequestId() == "" {
		return nil, gatewayFailure(
			codes.InvalidArgument,
			"INVALID_REQUEST",
			"Invalid request",
			requestID,
		)
	}

	result, err := s.core.GetRequestDetails(
		ctx,
		coreclient.GetRequestDetailsInput{
			RequestID: req.GetRequestId(),
		},
	)
	if err != nil {
		return nil, mapCoreError(ctx, err, requestID)
	}

	request, err := toProtoRequestDetails(result)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"Internal Gateway error",
		)
	}

	return &gatewayv1.GetRequestDetailsResponse{
		Request: request,
	}, nil
}
