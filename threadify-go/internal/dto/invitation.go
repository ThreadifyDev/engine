package dto

import (
	"github.com/golang-jwt/jwt/v5"
)

type ThreadInvitationClaims struct {
	ThreadID    string `json:"threadId"`
	Role        string `json:"role"`
	AccessLevel string `json:"accessLevel"`
	InvitedBy   string `json:"invitedBy"`
	jwt.RegisteredClaims
}

type InvitePartyRequest struct {
	Action      string `json:"action"`
	Role        string `json:"role"`
	AccessLevel string `json:"accessLevel"`
	ExpiresIn   string `json:"expiresIn"`
}

type InvitePartyResponse struct {
	Action      string `json:"action"`
	Status      string `json:"status"`
	ThreadToken string `json:"threadToken"`
	Role        string `json:"role"`
	AccessLevel string `json:"accessLevel"`
	ExpiresAt   int64  `json:"expiresAt"`
	Message     string `json:"message"`
}

type JoinThreadRequest struct {
	Action      string `json:"action"`
	ThreadToken string `json:"threadToken,omitempty"`
	ThreadID    string `json:"threadId,omitempty"`
	Role        string `json:"role,omitempty"`
}

type JoinThreadResponse struct {
	Action      string `json:"action"`
	Status      string `json:"status"`
	ThreadID    string `json:"threadId"`
	ContractID  string `json:"contractId"`
	Role        string `json:"role"`
	AccessLevel string `json:"accessLevel"`
	Message     string `json:"message"`
}
