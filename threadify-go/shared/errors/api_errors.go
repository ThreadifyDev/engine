package serror

import (
	"net/http"
)

// API-level sentinel errors.
var (
	ErrUserAlreadyExists            = NewDomainError("user with this email already exists", http.StatusConflict)
	ErrInvalidCredentials           = NewDomainError("invalid email or password", http.StatusUnauthorized)
	ErrInvalidEmail                 = NewDomainError("invalid email address", http.StatusBadRequest)
	ErrAccountStillProvisioning     = NewDomainError("account is still being set up, please try again shortly", http.StatusServiceUnavailable)
	ErrPasswordResetRequired        = NewDomainError("please use 'Forgot Password' to set up your account", http.StatusUnauthorized)
	ErrJwtVerificationNotConfigured = NewDomainError("JWT verification not configured", http.StatusInternalServerError)
	ErrExpiredToken                 = NewDomainError("token has expired", http.StatusUnauthorized)
	ErrInvalidToken                 = NewDomainError("invalid or already used token", http.StatusUnauthorized)
	ErrRateLimit                    = NewDomainError("too many requests, please try again later", http.StatusTooManyRequests)

	// API Key errors
	ErrApiKeyNameRequired        = NewDomainError("API key name is required", http.StatusBadRequest)
	ErrInvalidServiceAccountRole = NewDomainError("invalid service account role: must be from api_level", http.StatusBadRequest)
	ErrServiceAccountNotFound    = NewDomainError("service account not found", http.StatusNotFound)
	ErrUnauthorizedCompany       = NewDomainError("unauthorized: service account belongs to different company", http.StatusForbidden)
	ErrFailedToAssignRole        = NewDomainError("failed to assign role to service account", http.StatusInternalServerError)
	ErrApiKeyNotFound            = NewDomainError("API key not found", http.StatusNotFound)
	ErrUnauthorized              = NewDomainError("unauthorized", http.StatusUnauthorized)
	ErrInvalidApiKey             = NewDomainError("invalid API key", http.StatusUnauthorized)
	ErrApiKeyRevoked             = NewDomainError("API key has been revoked", http.StatusUnauthorized)
	ErrApiKeyExpiredAPI          = NewDomainError("API key has expired", http.StatusUnauthorized)
	ErrInternalServerError       = NewDomainError("internal server error", http.StatusInternalServerError)

	// Generic errors
	ErrNotFound  = NewDomainError("not found", http.StatusNotFound)
	ErrForbidden = NewDomainError("forbidden", http.StatusForbidden)

	// Service Account errors
	ErrServiceAccountNameRequired = NewDomainError("service account name is required", http.StatusBadRequest)
	ErrInvalidRoleAPI             = NewDomainError("invalid role: must be 'standard_service' or 'reader'", http.StatusBadRequest)
	ErrUserNotFound               = NewDomainError("user not found", http.StatusNotFound)
	ErrPaymentRequired            = NewDomainError("payment required: insufficient credits", http.StatusPaymentRequired)
)
