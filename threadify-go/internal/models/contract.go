package models

import "time"

type Contract struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	ContentHash   *string   `json:"contentHash,omitempty"`
	LatestVersion int       `json:"latestVersion"`
	OwnerID       string    `json:"ownerId,omitempty"`
	IsPublic      bool      `json:"isPublic"`
	IsDeleted     bool      `json:"isDeleted"`
	CreatedAt     time.Time `json:"createdAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ContractVersion struct {
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	Content     string    `json:"content"`
	ContentHash string    `json:"contentHash"`
	ContractID  string    `json:"contractId"`
	CreatedBy   string    `json:"createdBy"`
	IsDeleted   bool      `json:"isDeleted"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
