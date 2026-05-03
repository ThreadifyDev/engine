package domain

import "time"

type APIKey struct {
	ID               string
	KeyHash          string
	KeyPrefix        string
	Name             string
	UserID           *string
	ServiceAccountID *string
	CompanyID        string
	LastUsedAt       *time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
}
