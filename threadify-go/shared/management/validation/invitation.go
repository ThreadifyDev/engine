package validation

import (
	"strings"
)

// SendInvitationRequest is the struct from handlers, but duplicated here to avoid circular dependency
// or we can pass individual fields. Actually, it's better to pass what we need.

func ValidateSendInvitationRequest(email, role string) error {
	b := &validationBuilder{}

	email = strings.ToLower(strings.TrimSpace(email))
	validateEmail("email", email, b)

	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		b.add("role", "Role is required")
	} else if role != "admin" && role != "member" && role != "viewer" {
		b.add("role", "Invalid role. Must be admin, member, or viewer")
	}

	return b.err()
}

func ValidateValidateInvitationRequest(token string) error {
	b := &validationBuilder{}

	if strings.TrimSpace(token) == "" {
		b.add("token", "Invitation token is required")
	}

	return b.err()
}
