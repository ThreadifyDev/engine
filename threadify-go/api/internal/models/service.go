package models

type CreateServiceAccountRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Role        string  `json:"role" binding:"required"`
}

type UpdateServiceAccountRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}
