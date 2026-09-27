package grpcapi

import (
	"errors"
	"fmt"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var requestStatuses = map[string]gatewayv1.ActivityRequestStatus{
	"PLANNED":      gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_PLANNED,
	"ACTIVE":       gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_ACTIVE,
	"CONFIRMATION": gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_CONFIRMATION,
	"COMPLETED":    gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_COMPLETED,
	"CANCELLED":    gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_CANCELLED,
	"FROZEN":       gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_FROZEN,
}

var participantRoles = map[string]gatewayv1.ParticipantRole{
	"ORGANIZER": gatewayv1.ParticipantRole_PARTICIPANT_ROLE_ORGANIZER,
	"PLAYER":    gatewayv1.ParticipantRole_PARTICIPANT_ROLE_PLAYER,
}

var participantStatuses = map[string]gatewayv1.ParticipantStatus{
	"PENDING":  gatewayv1.ParticipantStatus_PARTICIPANT_STATUS_PENDING,
	"ACCEPTED": gatewayv1.ParticipantStatus_PARTICIPANT_STATUS_ACCEPTED,
	"KICKED":   gatewayv1.ParticipantStatus_PARTICIPANT_STATUS_KICKED,
	"LEFT":     gatewayv1.ParticipantStatus_PARTICIPANT_STATUS_LEFT,
}

func toProtoTimestamp(value *time.Time) (*timestamppb.Timestamp, error) {
	if value == nil {
		return nil, nil
	}

	result := timestamppb.New(value.UTC())
	if err := result.CheckValid(); err != nil {
		return nil, errors.New("invalid timestamp")
	}

	return result, nil
}

func toProtoEnum[T ~int32](
	value *string,
	allowed map[string]T,
) (*T, error) {
	if value == nil {
		return nil, nil
	}

	result, ok := allowed[*value]
	if !ok {
		return nil, errors.New("unknown enum value")
	}

	return &result, nil
}

func toProtoParticipant(
	input coreclient.Participant,
) (*gatewayv1.Participant, error) {
	role, err := toProtoEnum(input.Role, participantRoles)
	if err != nil {
		return nil, fmt.Errorf("role: %w", err)
	}

	participantStatus, err := toProtoEnum(
		input.Status,
		participantStatuses,
	)
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	joinedAt, err := toProtoTimestamp(input.JoinedAt)
	if err != nil {
		return nil, fmt.Errorf("joinedAt: %w", err)
	}

	return &gatewayv1.Participant{
		UserId:   input.UserID,
		Role:     role,
		Status:   participantStatus,
		JoinedAt: joinedAt,
	}, nil
}

func toProtoParticipants(
	input *coreclient.ParticipantList,
) (*gatewayv1.ParticipantList, error) {
	if input == nil {
		return nil, nil
	}

	result := &gatewayv1.ParticipantList{
		Values: make([]*gatewayv1.Participant, 0, len(input.Values)),
	}

	for idx, participant := range input.Values {
		converted, err := toProtoParticipant(participant)
		if err != nil {
			return nil, fmt.Errorf("participant %d: %w", idx, err)
		}

		result.Values = append(result.Values, converted)
	}

	return result, nil
}

func toProtoRequestDetails(
	input coreclient.ActivityRequestDetails,
) (*gatewayv1.ActivityRequestDetails, error) {
	eventDate, err := toProtoTimestamp(input.EventDate)
	if err != nil {
		return nil, fmt.Errorf("eventDate: %w", err)
	}

	requestStatus, err := toProtoEnum(input.Status, requestStatuses)
	if err != nil {
		return nil, fmt.Errorf("requestStatus: %w", err)
	}

	participants, err := toProtoParticipants(input.Participants)
	if err != nil {
		return nil, fmt.Errorf("participants: %w", err)
	}

	return &gatewayv1.ActivityRequestDetails{
		Id:               input.ID,
		Title:            input.Title,
		Description:      input.Description,
		SportName:        input.SportName,
		MaxPlayers:       input.MaxPlayers,
		CurrentPlayers:   input.CurrentPlayers,
		EventDate:        eventDate,
		AddressText:      input.AddressText,
		NumberOfRounds:   input.NumberOfRounds,
		Latitude:         input.Latitude,
		Longitude:        input.Longitude,
		Status:           requestStatus,
		RegistrationOpen: input.RegistrationOpen,
		OrganizerId:      input.OrganizerID,
		Participants:     participants,
	}, nil
}
