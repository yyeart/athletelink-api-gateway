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

type UpdateRequestInput struct {
	RequestID      string
	UserID         string
	Title          *string
	Description    *string
	EventDate      *time.Time
	NumberOfRounds *int32
	MaxPlayers     *int32
}

type SearchNearbyRequestsInput struct {
	Lat       *float64
	Lon       *float64
	Radius    *float64
	SportID   *int64
	StartDate *time.Time
	EndDate   *time.Time
}

type ActivityRequestFeed struct {
	ID               *string
	Title            *string
	SportName        *string
	MaxPlayers       *int32
	CurrentPlayers   *int32
	EventDate        *time.Time
	AddressText      *string
	NumberOfRounds   *int32
	RegistrationOpen *bool
	Latitude         *float64
	Longitude        *float64
}

type CreateRequestInput struct {
	UserID         *string
	Title          *string
	Description    *string
	SportID        *int64
	MaxPlayers     *int32
	EventDate      *time.Time
	NumberOfRounds *int32
	AddressText    *string
	Latitude       *float64
	Longitude      *float64
}
