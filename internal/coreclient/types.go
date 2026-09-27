package coreclient

import (
	"time"
)

type GetRequestDetailsInput struct {
	RequestID string
}

type ActivityRequestDetails struct {
	ID               *string
	Title            *string
	Description      *string
	SportName        *string
	MaxPlayers       *int32
	CurrentPlayers   *int32
	EventDate        *time.Time
	AddressText      *string
	NumberOfRounds   *int32
	Latitude         *float64
	Longitude        *float64
	Status           *string
	RegistrationOpen *bool
	OrganizerID      *string
	Participants     *ParticipantList
}

type ParticipantList struct {
	Values []Participant
}

type Participant struct {
	UserID   *string
	Role     *string
	Status   *string
	JoinedAt *time.Time
}
