package models

import "time"

type SendInvitationRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required,oneof=admin member viewer"`
}

type SendInvitationResponse struct {
	Success      bool   `json:"success"`
	InvitationID string `json:"invitationId,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	Message      string `json:"message,omitempty"`
	Error        string `json:"error,omitempty"`
}

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

type ValidateTokenResult struct {
	CompanyName string
	Email       string
}

type ValidateInvitationRequest struct {
	Token string `json:"token" binding:"required"`
}

type ValidateInvitationResponse struct {
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
}

type InvitationListItem struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	InvitedBy string `json:"invited_by"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
}

type ListInvitationsResponse struct {
	Invitations []*InvitationListItem `json:"invitations"`
}
