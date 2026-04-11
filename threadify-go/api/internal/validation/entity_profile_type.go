package validation

import (
	"strings"
	"threadify-go/api/internal/models"
)

func ValidateCreateEntityProfileTypeRequest(req *models.CreateEntityProfileTypeRequest) error {
	b := &validationBuilder{}

	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	if strings.TrimSpace(req.Name) == "" {
		b.add("name", "Name is required.")
	}

	if strings.TrimSpace(req.Type) == "" {
		b.add("type", "Type is required.")
	}

	if strings.TrimSpace(req.Description) == "" {
		b.add("description", "Description is required.")
	}

	return b.err()
}
