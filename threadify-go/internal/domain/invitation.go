package domain

import (
	"github.com/golang-jwt/jwt/v5"
)

// ThreadInvitationClaims represents JWT claims for thread invitations.
type ThreadInvitationClaims struct {
	ThreadID    string
	Role        string // Business/contract role
	AccessLevel string // owner | participant | observer | external
	InvitedBy   string
	jwt.RegisteredClaims
}

// InvitePartyCmd represents a request to create an invitation token
type InvitePartyCmd struct {
	Action      string
	Role        string
	AccessLevel string
	ExpiresIn   string
}

// InvitePartyResponse represents the response from inviteParty
type InvitePartyResponse struct {
	Action      string
	Status      string
	ThreadToken string
	Role        string
	AccessLevel string
	ExpiresAt   int64
	Message     string
}

// JoinThreadCmd represents a request to join a thread with token or directly
type JoinThreadCmd struct {
	Action      string
	ThreadToken string
	ThreadID    string
	Role        string
}

// JoinThreadResponse represents the response from joinThread
type JoinThreadResponse struct {
	Action      string
	Status      string
	ThreadID    string
	ContractID  string
	Role        string
	AccessLevel string
	Message     string
}
