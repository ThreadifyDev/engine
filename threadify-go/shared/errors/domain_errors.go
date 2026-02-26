package errors

import "errors"

// Shared sentinel errors accessible by the entire project (Engine, API, SDKs).
var (
	ErrThreadNotFound        = errors.New("thread not found")
	ErrAccessDenied          = errors.New("access denied")
	ErrThreadAlreadyComplete = errors.New("thread already completed")
	ErrNoActiveThread        = errors.New("no active thread found")
	ErrNotAuthenticated      = errors.New("not authenticated")
	ErrInvalidRole           = errors.New("invalid role")
)
