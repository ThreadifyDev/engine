package validation

import (
	"fmt"
	"strings"
	"threadify-go/api/internal/dto"
)

const maxTypes = 5
const maxDescriptionLen = 255

func ValidateCreateEntityProfileTypeRequest(req *dto.CreateEntityProfileTypeRequest) error {
	b := &validationBuilder{}

	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	if strings.TrimSpace(req.Name) == "" {
		b.add("name", "Name is required.")
	}

	if len(req.Description) > maxDescriptionLen {
		b.add("description", fmt.Sprintf("Description cannot exceed %d characters.", maxDescriptionLen))
	}

	hasTypes := false
	for _, t := range req.Type {
		if strings.TrimSpace(t) != "" {
			hasTypes = true
			break
		}
	}

	if !hasTypes {
		b.add("type", "At least one type is required.")
	}

	if len(req.Type) > maxTypes {
		b.add("type", fmt.Sprintf("Types cannot exceed %d values.", maxTypes))
	}

	return b.err()
}

func ValidateUpdateEntityProfileTypeRequest(req *dto.UpdateEntityProfileTypeRequest) error {
	b := &validationBuilder{}

	if len(req.Description) > maxDescriptionLen {
		b.add("description", fmt.Sprintf("Description cannot exceed %d characters.", maxDescriptionLen))
	}

	if len(req.Type) > maxTypes {
		b.add("type", fmt.Sprintf("Types cannot exceed %d values.", maxTypes))
	}

	return b.err()
}
