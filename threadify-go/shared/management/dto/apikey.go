package dto

import "time"

type CreateAPIKeyRequest struct {
	Name                 string  `json:"name" binding:"required"`
	ExpiresIn            *int    `json:"expires_in"`
	ServiceAccountID     *string `json:"service_account_id"`
	CreateServiceAccount bool    `json:"create_service_account"`
	ServiceAccountRole   *string `json:"service_account_role"`
}

type APIKeyInfo struct {
	CreatedAt        time.Time  `json:"created_at"`
	ID               string     `json:"id"`
	CompanyID        string     `json:"company_id"`
	KeyPrefix        string     `json:"key_prefix"`
	Name             string     `json:"name"`
	UserID           *string    `json:"user_id"`
	ServiceAccountID *string    `json:"service_account_id"`
	LastUsedAt       *time.Time `json:"last_used_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
}

type CreateAPIKeyResponse struct {
	Key       string      `json:"key"`
	KeyPrefix string      `json:"key_prefix"`
	APIKey    *APIKeyInfo `json:"api_key"`
}
