package service

import "errors"

// API-level sentinel errors.
var (
	ErrUserAlreadyExists            = errors.New("user with this email already exists")
	ErrInvalidCredentials           = errors.New("invalid email or password")
	ErrInvalidEmail                 = errors.New("invalid email address")
	ErrAccountStillProvisioning     = errors.New("account is still being set up, please try again shortly")
	ErrJwtVerificationNotConfigured = errors.New("JWT verification not configured")

	// API Key errors
	ErrApiKeyNameRequired        = errors.New("API key name is required")
	ErrInvalidServiceAccountRole = errors.New("invalid service account role: must be from api_level")
	ErrServiceAccountNotFound    = errors.New("service account not found")
	ErrUnauthorizedCompany       = errors.New("unauthorized: service account belongs to different company")
	ErrFailedToAssignRole        = errors.New("failed to assign role to service account")
	ErrApiKeyNotFound            = errors.New("API key not found")
	ErrUnauthorized              = errors.New("unauthorized")
	ErrInvalidApiKey             = errors.New("invalid API key")
	ErrApiKeyRevoked             = errors.New("API key has been revoked")
	ErrApiKeyExpiredAPI          = errors.New("API key has expired")

	// Service Account errors
	ErrServiceAccountNameRequired = errors.New("service account name is required")
	ErrInvalidRoleAPI             = errors.New("invalid role: must be 'standard_service' or 'reader'")

	// System errors
	ErrDeprecatedPermissions = errors.New("deprecated: use role-based permissions instead")
)

// HTTP Header constants.
const (
	HeaderAuthorization = "Authorization"
	HeaderContentType   = "Content-Type"
	HeaderUserAgent     = "User-Agent"
	HeaderXAPIKey       = "X-API-Key"
	HeaderCacheControl  = "Cache-Control"
	HeaderConnection    = "Connection"
)

// Common payload types and roles.
const (
	ContentTypeJSON = "application/json"
	UserAgentAPI    = "Threadify-WebAPI/1.0"
)
