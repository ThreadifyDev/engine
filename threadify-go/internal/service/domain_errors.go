package service

import (
	"errors"
)

// Domain-level sentinel errors for the service layer.
// Use errors.Is for comparison — these are typed and comparable.
var (
	ErrCannotManuallyCompleteThreadLinkedToContract = errors.New("cannot manually complete thread linked to contract: thread will auto-complete when terminal state is reached")
	ErrStepAlreadyCompleted                         = errors.New("step already completed")

	// Auth and Database errors
	ErrDatabaseNotConfigured        = errors.New("database not configured")
	ErrApiKeyInvalidOrInactive      = errors.New("API key not found or inactive")
	ErrApiKeyExpired                = errors.New("API key has expired")
	ErrJwtVerificationNotConfigured = errors.New("JWT verification not configured")

	// Contract and Validation errors
	ErrContractRepoNotAvailable = errors.New("contract repository not available")
	ErrContractNotFound         = errors.New("contract not found")
	ErrInvalidToken             = errors.New("invalid token")
	ErrInvalidThreadToken       = errors.New("invalid thread token")

	// Scope and Configuration errors
	ErrConfigMissingDefaultScope = errors.New("notification_system.default_scope is not set")
	ErrInvalidRuntimeRole        = errors.New("invalid runtime_role: cannot be empty")
	ErrInvalidThreadStatus       = errors.New("invalid status: must be 'cancelled' or 'completed'")
	ErrFailedToManualComplete    = errors.New("cannot manually complete thread linked to contract: thread will auto-complete when terminal state is reached")

	// Step processing errors
	ErrStepIdRequired      = errors.New("step_id is required")
	ErrThreadIdRequired    = errors.New("thread_id is required")
	ErrContextRequired     = errors.New("context is required")
	ErrFailedToProcessStep = errors.New("failed to process step event")
	ErrFailedToGetThread   = errors.New("failed to get thread")

	// Thread lifecycle errors
	ErrThreadAlreadyEnded = errors.New("thread has already been ended")
)
