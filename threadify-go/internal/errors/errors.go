package errors

import (
	"errors"
	"fmt"
)

// ErrorType represents the category of error
type ErrorType string

const (
	ErrorTypeNotFound     ErrorType = "NOT_FOUND"
	ErrorTypeValidation   ErrorType = "VALIDATION_ERROR"
	ErrorTypeUnauthorized ErrorType = "UNAUTHORIZED"
	ErrorTypeForbidden    ErrorType = "FORBIDDEN"
	ErrorTypeConflict     ErrorType = "CONFLICT"
	ErrorTypeInternal     ErrorType = "INTERNAL_ERROR"
	ErrorTypeBadRequest   ErrorType = "BAD_REQUEST"
	ErrorTypeUnavailable  ErrorType = "SERVICE_UNAVAILABLE"
)

// AppError represents an application error with context
type AppError struct {
	Type       ErrorType
	Message    string // User-friendly message
	StatusCode int    // HTTP status code
	Err        error  // Original error (for logging, not exposed to client)
}

func (e *AppError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error for errors.Is/As
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewNotFoundError creates a not found error
func NewNotFoundError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeNotFound,
		Message:    message,
		StatusCode: 404,
		Err:        err,
	}
}

// NewValidationError creates a validation error
func NewValidationError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeValidation,
		Message:    message,
		StatusCode: 400,
		Err:        err,
	}
}

// NewUnauthorizedError creates an unauthorized error
func NewUnauthorizedError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeUnauthorized,
		Message:    message,
		StatusCode: 401,
		Err:        err,
	}
}

// NewForbiddenError creates a forbidden error
func NewForbiddenError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeForbidden,
		Message:    message,
		StatusCode: 403,
		Err:        err,
	}
}

// NewConflictError creates a conflict error
func NewConflictError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeConflict,
		Message:    message,
		StatusCode: 409,
		Err:        err,
	}
}

// NewBadRequestError creates a bad request error
func NewBadRequestError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeBadRequest,
		Message:    message,
		StatusCode: 400,
		Err:        err,
	}
}

// NewInternalError creates an internal server error
func NewInternalError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeInternal,
		Message:    message,
		StatusCode: 500,
		Err:        err,
	}
}

// NewUnavailableError creates a service unavailable error
func NewUnavailableError(message string, err error) *AppError {
	return &AppError{
		Type:       ErrorTypeUnavailable,
		Message:    message,
		StatusCode: 503,
		Err:        err,
	}
}

// IsNotFound checks if error is a not found error
func IsNotFound(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrorTypeNotFound
	}
	return false
}

// IsValidation checks if error is a validation error
func IsValidation(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrorTypeValidation
	}
	return false
}

// IsUnauthorized checks if error is an unauthorized error
func IsUnauthorized(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrorTypeUnauthorized
	}
	return false
}

// IsForbidden checks if error is a forbidden error
func IsForbidden(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrorTypeForbidden
	}
	return false
}

// GetStatusCode extracts HTTP status code from error
func GetStatusCode(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.StatusCode
	}
	return 500 // Default to internal server error
}

// GetUserMessage extracts user-friendly message from error
func GetUserMessage(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return "An unexpected error occurred"
}

// GetErrorType extracts error type from error
func GetErrorType(err error) ErrorType {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type
	}
	return ErrorTypeInternal
}

// WrapWithContext wraps an error with additional context
func WrapWithContext(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// Common error messages
const (
	MsgContractNotFound        = "Contract not found"
	MsgContractInvalid         = "Contract configuration is invalid"
	MsgContractVersionNotFound = "Contract version not found"
	MsgContractIncomplete      = "Contract configuration is incomplete"
	MsgUnauthorized            = "Authentication required"
	MsgForbidden               = "Access denied"
	MsgInvalidInput            = "Invalid input provided"
	MsgInternalError           = "An internal error occurred"
	MsgServiceUnavailable      = "Service temporarily unavailable"
	MsgThreadNotFound          = "Thread not found"
	MsgInvalidCredentials      = "Invalid credentials"
	MsgResourceAlreadyExists   = "Resource already exists"
)
