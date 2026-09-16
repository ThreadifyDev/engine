package dto

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type CreateServiceAccountRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Role        string  `json:"role" binding:"required"`
}

type UpdateServiceAccountRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`

	hasName        bool
	hasDescription bool
	hasIsActive    bool
}

func (r *UpdateServiceAccountRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateServiceAccountRequest

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*r = UpdateServiceAccountRequest(decoded)
	_, r.hasName = raw["name"]
	_, r.hasDescription = raw["description"]
	_, r.hasIsActive = raw["is_active"]

	return nil
}

func (r UpdateServiceAccountRequest) Validate() error {
	switch {
	case !r.hasName:
		return errors.New("name is required")
	case !r.hasDescription:
		return errors.New("description is required")
	case !r.hasIsActive:
		return errors.New("is_active is required")
	case strings.TrimSpace(r.Name) == "":
		return errors.New("name is required")
	default:
		return nil
	}
}

type ServiceAccount struct {
	ID          string     `json:"id"`
	CompanyID   string     `json:"company_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	IsActive    bool       `json:"is_active"`
	CreatedBy   *string    `json:"created_by"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
