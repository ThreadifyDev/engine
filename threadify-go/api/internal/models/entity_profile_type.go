package models

type CreateEntityProfileTypeRequest struct {
	Name        string   `json:"name" binding:"required"`
	Type        []string `json:"type"`
	Description string   `json:"description" binding:"required"`
}

type UpdateEntityProfileTypeRequest struct {
	Name        string   `json:"name" binding:"required"`
	Type        []string `json:"type"`
	Description string   `json:"description"`
}
