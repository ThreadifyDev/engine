package auth

import (
	"errors"
	"net/http"

	"threadify-go/shared/registry"
)

// Keep provider responses, identity claims, SQL values and credentials out of logs and HTTP errors.
type managedLoginError struct {
	stage string
	cause error
}

func (e *managedLoginError) Error() string { return "managed login failed at " + e.stage }
func (e *managedLoginError) Unwrap() error { return e.cause }

func managedLoginFailure(err error) (status int, code, stage string) {
	stage = "unknown"
	var failure *managedLoginError
	if errors.As(err, &failure) {
		stage = failure.stage
	}
	if errors.Is(err, registry.ErrIdentityUnavailable) || errors.Is(err, registry.ErrUnverified) {
		return http.StatusServiceUnavailable, "managed_login_unavailable", stage
	}
	if errors.Is(err, ErrBrowserAuth) {
		// The failure stage stays in server logs; HTTP must not reveal membership or identity state.
		return http.StatusForbidden, "managed_login_denied", stage
	}
	return http.StatusInternalServerError, "managed_login_internal_error", stage
}
