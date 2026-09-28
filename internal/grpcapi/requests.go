package grpcapi

import (
	"context"
	"math"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetRequestDetails(
	ctx context.Context,
	req *gatewayv1.GetRequestDetailsRequest,
) (*gatewayv1.GetRequestDetailsResponse, error) {
	requestID, _, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.RequestId == nil || req.GetRequestId() == "" {
		return nil, invalidRequest(requestID)
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
		return nil, mapCoreError(
			ctx,
			&coreclient.ContractError{Cause: err},
			requestID,
		)
	}

	return &gatewayv1.GetRequestDetailsResponse{
		Request: request,
	}, nil
}

func (s *Server) UpdateRequest(
	ctx context.Context,
	req *gatewayv1.UpdateRequestRequest,
) (*gatewayv1.UpdateRequestResponse, error) {
	requestID, identity, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.RequestId == nil || req.GetRequestId() == "" {
		return nil, invalidRequest(requestID)
	}

	eventDate, err := toCoreTime(req.EventDate)
	if err != nil {
		return nil, invalidRequest(requestID)
	}

	input := coreclient.UpdateRequestInput{
		RequestID:      req.GetRequestId(),
		UserID:         identity.UserID,
		Title:          req.Title,
		Description:    req.Description,
		EventDate:      eventDate,
		NumberOfRounds: req.NumberOfRounds,
		MaxPlayers:     req.MaxPlayers,
	}

	result, err := s.core.UpdateRequest(ctx, input)
	if err != nil {
		return nil, mapCoreError(ctx, err, requestID)
	}

	converted, err := toProtoRequestDetails(result)
	if err != nil {
		return nil, mapCoreError(
			ctx, &coreclient.ContractError{Cause: err}, requestID,
		)
	}

	return &gatewayv1.UpdateRequestResponse{Request: converted}, nil
}

func (s *Server) SearchNearbyRequests(
	ctx context.Context,
	req *gatewayv1.SearchNearbyRequestsRequest,
) (*gatewayv1.SearchNearbyRequestsResponse, error) {
	requestID, _, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, invalidRequest(requestID)
	}

	if !finite(req.Lat) || !finite(req.Lon) || !finite(req.Radius) {
		return nil, invalidRequest(requestID)
	}

	startDate, err := toCoreTime(req.StartDate)
	if err != nil {
		return nil, invalidRequest(requestID)
	}

	endDate, err := toCoreTime(req.EndDate)
	if err != nil {
		return nil, invalidRequest(requestID)
	}

	input := coreclient.SearchNearbyRequestsInput{
		Lat:       req.Lat,
		Lon:       req.Lon,
		Radius:    req.Radius,
		SportID:   req.SportId,
		StartDate: startDate,
		EndDate:   endDate,
	}

	results, err := s.core.SearchNearbyRequests(ctx, input)
	if err != nil {
		return nil, mapCoreError(ctx, err, requestID)
	}

	items, err := toProtoFeeds(results)
	if err != nil {
		return nil, mapCoreError(
			ctx, &coreclient.ContractError{Cause: err}, requestID,
		)
	}

	return &gatewayv1.SearchNearbyRequestsResponse{
		Requests: items,
	}, nil
}

func (s *Server) CreateRequest(
	ctx context.Context,
	req *gatewayv1.CreateRequestRequest,
) (*gatewayv1.CreateRequestResponse, error) {
	requestID, identity, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, invalidRequest(requestID)
	}

	if !finite(req.Latitude) || !finite(req.Longitude) {
		return nil, invalidRequest(requestID)
	}

	eventDate, err := toCoreTime(req.EventDate)
	if err != nil {
		return nil, invalidRequest(requestID)
	}

	input := coreclient.CreateRequestInput{
		UserID:         &identity.UserID,
		Title:          req.Title,
		Description:    req.Description,
		SportID:        req.SportId,
		MaxPlayers:     req.MaxPlayers,
		EventDate:      eventDate,
		NumberOfRounds: req.NumberOfRounds,
		AddressText:    req.AddressText,
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
	}

	createdID, err := s.core.CreateRequest(ctx, input)
	if err != nil {
		return nil, mapCoreError(ctx, err, requestID)
	}

	return &gatewayv1.CreateRequestResponse{RequestId: &createdID}, nil
}

func (s *Server) OpenRegistration(
	ctx context.Context,
	req *gatewayv1.OpenRegistrationRequest,
) (*gatewayv1.OpenRegistrationResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.OpenRegistration); err != nil {
		return nil, err
	}

	return &gatewayv1.OpenRegistrationResponse{}, nil
}

func (s *Server) CloseRegistration(
	ctx context.Context,
	req *gatewayv1.CloseRegistrationRequest,
) (*gatewayv1.CloseRegistrationResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.CloseRegistration); err != nil {
		return nil, err
	}

	return &gatewayv1.CloseRegistrationResponse{}, nil
}

func (s *Server) LeaveRequest(
	ctx context.Context,
	req *gatewayv1.LeaveRequestRequest,
) (*gatewayv1.LeaveRequestResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.LeaveRequest); err != nil {
		return nil, err
	}

	return &gatewayv1.LeaveRequestResponse{}, nil
}

func (s *Server) JoinRequest(
	ctx context.Context,
	req *gatewayv1.JoinRequestRequest,
) (*gatewayv1.JoinRequestResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.JoinRequest); err != nil {
		return nil, err
	}

	return &gatewayv1.JoinRequestResponse{}, nil
}

func (s *Server) KickParticipant(
	ctx context.Context,
	req *gatewayv1.KickParticipantRequest,
) (*gatewayv1.KickParticipantResponse, error) {
	correlationID, identity, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil ||
		req.RequestId == nil ||
		req.GetRequestId() == "" ||
		req.TargetUserId == nil ||
		req.GetTargetUserId() == "" {
		return nil, invalidRequest(correlationID)
	}

	input := coreclient.KickParticipantInput{
		RequestID:    req.GetRequestId(),
		UserID:       identity.UserID,
		TargetUserID: req.GetTargetUserId(),
	}

	if err := s.core.KickParticipant(ctx, input); err != nil {
		return nil, mapCoreError(ctx, err, correlationID)
	}

	return &gatewayv1.KickParticipantResponse{}, nil
}

func (s *Server) StartRequest(
	ctx context.Context,
	req *gatewayv1.StartRequestRequest,
) (*gatewayv1.StartRequestResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.StartRequest); err != nil {
		return nil, err
	}

	return &gatewayv1.StartRequestResponse{}, nil
}

func (s *Server) CancelRequest(
	ctx context.Context,
	req *gatewayv1.CancelRequestRequest,
) (*gatewayv1.CancelRequestResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.CancelRequest); err != nil {
		return nil, err
	}

	return &gatewayv1.CancelRequestResponse{}, nil
}

func (s *Server) CompleteRequest(
	ctx context.Context,
	req *gatewayv1.CompleteRequestRequest,
) (*gatewayv1.CompleteRequestResponse, error) {
	var id *string
	if req != nil {
		id = req.RequestId
	}

	if err := s.runAction(ctx, id, s.core.CompleteRequest); err != nil {
		return nil, err
	}

	return &gatewayv1.CompleteRequestResponse{}, nil
}

func (s *Server) RecordRoundResult(
	ctx context.Context,
	req *gatewayv1.RecordRoundResultRequest,
) (*gatewayv1.RecordRoundResultResponse, error) {
	correlationID, identity, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.RequestId == nil ||
		req.GetRequestId() == "" ||
		req.RoundNumber == nil {
		return nil, invalidRequest(correlationID)
	}

	input := coreclient.RecordRoundResultInput{
		RequestID:   req.GetRequestId(),
		RoundNumber: req.GetRoundNumber(),
		UserID:      identity.UserID,
		Winners:     toCoreUUIDList(req.Winners),
		Losers:      toCoreUUIDList(req.Losers),
	}

	result, err := s.core.RecordRoundResult(ctx, input)
	if err != nil {
		return nil, mapCoreError(ctx, err, correlationID)
	}

	converted, err := toProtoRoundResult(result)
	if err != nil {
		return nil, mapCoreError(
			ctx, &coreclient.ContractError{Cause: err}, correlationID,
		)
	}

	return &gatewayv1.RecordRoundResultResponse{
		Result: converted,
	}, nil
}

func (s *Server) GetRoundResults(
	ctx context.Context,
	req *gatewayv1.GetRoundResultsRequest,
) (*gatewayv1.GetRoundResultsResponse, error) {
	correlationID, _, err := requestContext(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.RequestId == nil || req.GetRequestId() == "" {
		return nil, invalidRequest(correlationID)
	}

	input := coreclient.GetRoundResultsInput{
		RequestID: req.GetRequestId(),
	}

	results, err := s.core.GetRoundResults(ctx, input)
	if err != nil {
		return nil, mapCoreError(ctx, err, correlationID)
	}

	converted := make([]*gatewayv1.RoundResult, 0, len(results))
	for _, item := range results {
		value, err := toProtoRoundResult(item)
		if err != nil {
			return nil, mapCoreError(
				ctx, &coreclient.ContractError{Cause: err}, correlationID,
			)
		}

		converted = append(converted, value)
	}

	return &gatewayv1.GetRoundResultsResponse{
		Results: converted,
	}, nil
}

func (s *Server) runAction(
	ctx context.Context,
	requestID *string,
	action func(context.Context, coreclient.ActionInput) error,
) error {
	correlationID, identity, err := requestContext(ctx)
	if err != nil {
		return err
	}

	if requestID == nil || *requestID == "" {
		return invalidRequest(correlationID)
	}

	err = action(ctx, coreclient.ActionInput{
		RequestID: *requestID,
		UserID:    identity.UserID,
	})

	return mapCoreError(ctx, err, correlationID)
}

func requestContext(
	ctx context.Context,
) (string, requestcontext.Identity, error) {
	requestID, ok := requestcontext.RequestIDFrom(ctx)
	if !ok {
		return "", requestcontext.Identity{}, status.Error(
			codes.Internal,
			"Internal Gateway error",
		)
	}

	identity, ok := requestcontext.IdentityFrom(ctx)
	if !ok {
		return "", requestcontext.Identity{}, gatewayFailure(
			codes.Unauthenticated,
			"AUTHENTICATION_REQUIRED",
			"Authentication required",
			requestID,
		)
	}

	return requestID, identity, nil
}

func finite(v *float64) bool {
	return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0))
}
