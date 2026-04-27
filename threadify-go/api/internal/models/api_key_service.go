package models

import "time"

type APIKey struct {
	ID               string     `json:"id"`
	KeyHash          string     `json:"-"` // Never expose key hash
	KeyPrefix        string     `json:"key_prefix"`
	Name             string     `json:"name"`
	UserID           *string    `json:"user_id,omitempty"`
	ServiceAccountID *string    `json:"service_account_id,omitempty"`
	CompanyID        string     `json:"company_id"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
}

type CreateAPIKeyRequest struct {
	Name                 string  `json:"name" binding:"required"`
	ExpiresIn            *int    `json:"expires_in"`
	ServiceAccountID     *string `json:"service_account_id"`
	CreateServiceAccount bool    `json:"create_service_account"`
	ServiceAccountRole   *string `json:"service_account_role"`
}

type CreateAPIKeyResponse struct {
	Key       string  `json:"key"`
	KeyPrefix string  `json:"key_prefix"`
	APIKey    *APIKey `json:"api_key"`
}
