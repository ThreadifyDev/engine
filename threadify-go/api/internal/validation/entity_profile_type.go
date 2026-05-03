package validation

import (
	"fmt"
	"strings"
	"threadify-go/api/internal/dto"
)

const maxTypes = 5

func ValidateCreateEntityProfileTypeRequest(req *dto.CreateEntityProfileTypeRequest) error {
	b := &validationBuilder{}

	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	if strings.TrimSpace(req.Name) == "" {
		b.add("name", "Name is required.")
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

	if len(req.Type) > maxTypes {
		b.add("type", fmt.Sprintf("Types cannot exceed %d values.", maxTypes))
	}

	return b.err()
}
