package service

import (
	"fmt"
	"log"
	"strings"
)

// sanitizeError converts internal errors to user-friendly messages
// while preserving detailed error information for internal logging
func sanitizeError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Check for specific user input errors that are safe to expose
	switch {
	case strings.Contains(errMsg, "invalid step event"):
		return fmt.Errorf("invalid step data provided")
	case strings.Contains(errMsg, "serviceName"):
		return fmt.Errorf("serviceName is required")
	case strings.Contains(errMsg, "validation failed"):
		return fmt.Errorf("step validation failed")
	case strings.Contains(errMsg, "required"):
		return fmt.Errorf("missing required data")
	case strings.Contains(errMsg, "context"):
		return fmt.Errorf("invalid context data")
	default:
		// All internal errors get generic message
		return fmt.Errorf("failed to process step event")
	}
}

// logInternalError logs detailed internal errors while returning sanitized messages to users
func logInternalError(operation string, err error) {
	log.Printf("Internal error in %s: %v", operation, err)
}

// logInternalErrorWithDetails logs detailed internal errors with additional context
func logInternalErrorWithDetails(operation string, details string, err error) {
	log.Printf("Internal error in %s [%s]: %v", operation, details, err)
}
