package errors

import (
	"errors"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	err := NewNotFoundError("Resource not found", nil)
	if err.Error() != "Resource not found" {
		t.Errorf("Expected 'Resource not found', got '%s'", err.Error())
	}
}

func TestAppError_Unwrap(t *testing.T) {
	originalErr := errors.New("database error")
	appErr := NewInternalError("Internal error", originalErr)

	if !errors.Is(appErr, originalErr) {
		t.Error("Expected errors.Is to find the original error")
	}
}

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name           string
		constructor    func() *AppError
		expectedType   ErrorType
		expectedStatus int
	}{
		{
			name:           "NotFoundError",
			constructor:    func() *AppError { return NewNotFoundError("not found", nil) },
			expectedType:   ErrorTypeNotFound,
			expectedStatus: 404,
		},
		{
			name:           "ValidationError",
			constructor:    func() *AppError { return NewValidationError("invalid", nil) },
			expectedType:   ErrorTypeValidation,
			expectedStatus: 400,
		},
		{
			name:           "UnauthorizedError",
			constructor:    func() *AppError { return NewUnauthorizedError("unauthorized", nil) },
			expectedType:   ErrorTypeUnauthorized,
			expectedStatus: 401,
		},
		{
			name:           "ForbiddenError",
			constructor:    func() *AppError { return NewForbiddenError("forbidden", nil) },
			expectedType:   ErrorTypeForbidden,
			expectedStatus: 403,
		},
		{
			name:           "ConflictError",
			constructor:    func() *AppError { return NewConflictError("conflict", nil) },
			expectedType:   ErrorTypeConflict,
			expectedStatus: 409,
		},
		{
			name:           "BadRequestError",
			constructor:    func() *AppError { return NewBadRequestError("bad request", nil) },
			expectedType:   ErrorTypeBadRequest,
			expectedStatus: 400,
		},
		{
			name:           "InternalError",
			constructor:    func() *AppError { return NewInternalError("internal", nil) },
			expectedType:   ErrorTypeInternal,
			expectedStatus: 500,
		},
		{
			name:           "UnavailableError",
			constructor:    func() *AppError { return NewUnavailableError("unavailable", nil) },
			expectedType:   ErrorTypeUnavailable,
			expectedStatus: 503,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.constructor()
			if err.Type != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, err.Type)
			}
			if err.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, err.StatusCode)
			}
		})
	}
}

func TestErrorTypeCheckers(t *testing.T) {
	notFoundErr := NewNotFoundError("not found", nil)
	validationErr := NewValidationError("invalid", nil)

	if !IsNotFound(notFoundErr) {
		t.Error("IsNotFound should return true for NotFoundError")
	}

	if IsNotFound(validationErr) {
		t.Error("IsNotFound should return false for ValidationError")
	}

	if !IsValidation(validationErr) {
		t.Error("IsValidation should return true for ValidationError")
	}
}

func TestGetStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "AppError",
			err:      NewNotFoundError("not found", nil),
			expected: 404,
		},
		{
			name:     "StandardError",
			err:      errors.New("standard error"),
			expected: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := GetStatusCode(tt.err)
			if status != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, status)
			}
		})
	}
}

func TestGetUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "AppError",
			err:      NewNotFoundError("Resource not found", nil),
			expected: "Resource not found",
		},
		{
			name:     "StandardError",
			err:      errors.New("internal error"),
			expected: "An unexpected error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := GetUserMessage(tt.err)
			if msg != tt.expected {
				t.Errorf("Expected message '%s', got '%s'", tt.expected, msg)
			}
		})
	}
}

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{
			name:     "NotFoundError",
			err:      NewNotFoundError("not found", nil),
			expected: ErrorTypeNotFound,
		},
		{
			name:     "StandardError",
			err:      errors.New("standard error"),
			expected: ErrorTypeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errType := GetErrorType(tt.err)
			if errType != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, errType)
			}
		})
	}
}
