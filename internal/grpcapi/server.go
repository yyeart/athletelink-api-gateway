package grpcapi

import (
	"context"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
)

type Core interface {
	GetRequestDetails(
		context.Context,
		coreclient.GetRequestDetailsInput,
	) (coreclient.ActivityRequestDetails, error)

	UpdateRequest(
		context.Context,
		coreclient.UpdateRequestInput,
	) (coreclient.ActivityRequestDetails, error)

	SearchNearbyRequests(
		context.Context,
		coreclient.SearchNearbyRequestsInput,
	) ([]coreclient.ActivityRequestFeed, error)

	CreateRequest(
		context.Context,
		coreclient.CreateRequestInput,
	) (string, error)
}

type Server struct {
	gatewayv1.UnimplementedCoreServiceServer
	core Core
}

func New(core Core) *Server {
	return &Server{core: core}
}
