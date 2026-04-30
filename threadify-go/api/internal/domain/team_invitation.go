package domain

import (
	"fmt"
	"time"
)

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

func (i *TeamInvitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

func (i *TeamInvitation) CanBeAccepted() error {
	if i.Status != "pending" {
		return fmt.Errorf("invitation already used or expired")
	}
	if i.IsExpired() {
		return fmt.Errorf("invitation has expired")
	}
	return nil
}
