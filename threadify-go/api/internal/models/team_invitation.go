package models

import "time"

type TeamInvitation struct {
	ID               string
	CompanyID        string
	Email            string
	Role             string
	InvitedBy        string
	Status           string
	Token            string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	AcceptedAt       *time.Time
	AcceptedByUserID *string
}
