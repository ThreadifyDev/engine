package service

import (
	serror "threadify-go/shared/errors"
)

// API-level sentinel errors (moved to shared/errors/api_errors.go)
var (
	ErrUserAlreadyExists            = serror.ErrUserAlreadyExists
	ErrInvalidCredentials           = serror.ErrInvalidCredentials
	ErrInvalidEmail                 = serror.ErrInvalidEmail
	ErrAccountStillProvisioning     = serror.ErrAccountStillProvisioning
	ErrPasswordResetRequired        = serror.ErrPasswordResetRequired
	ErrJwtVerificationNotConfigured = serror.ErrJwtVerificationNotConfigured
	ErrExpiredToken                 = serror.ErrExpiredToken
	ErrInvalidToken                 = serror.ErrInvalidToken
	ErrRateLimit                    = serror.ErrRateLimit
	ErrInternalServerError          = serror.ErrInternalServerError

	// API Key errors
	ErrApiKeyNameRequired        = serror.ErrApiKeyNameRequired
	ErrInvalidServiceAccountRole = serror.ErrInvalidServiceAccountRole
	ErrServiceAccountNotFound    = serror.ErrServiceAccountNotFound
	ErrUnauthorizedCompany       = serror.ErrUnauthorizedCompany
	ErrFailedToAssignRole        = serror.ErrFailedToAssignRole
	ErrApiKeyNotFound            = serror.ErrApiKeyNotFound
	ErrUnauthorized              = serror.ErrUnauthorized
	ErrInvalidApiKey             = serror.ErrInvalidApiKey
	ErrApiKeyRevoked             = serror.ErrApiKeyRevoked
	ErrApiKeyExpiredAPI          = serror.ErrApiKeyExpiredAPI

	// Service Account errors
	ErrServiceAccountNameRequired = serror.ErrServiceAccountNameRequired
	ErrInvalidRoleAPI             = serror.ErrInvalidRoleAPI
	ErrUserNotFound               = serror.ErrUserNotFound
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
