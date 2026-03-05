package service

import (
	"errors"
	"strings"
)

func sanitizeError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "invalid step event"):
		return ErrFailedToProcessStep
	case strings.Contains(errMsg, "serviceName"):
		return errors.New("serviceName is required") // Keep as is if it's very specific, or use a sentinel
	case strings.Contains(errMsg, "validation failed"):
		return ErrFailedToProcessStep
	case strings.Contains(errMsg, "required"):
		return ErrFailedToProcessStep
	case strings.Contains(errMsg, "context"):
		return ErrContextRequired
	default:
		return ErrFailedToProcessStep
	}
}
