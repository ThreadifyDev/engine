package models

import (
	"encoding/json"
	"time"
)

type Contract struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	CompanyID     string    `json:"companyId"`
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
	ID                 string          `json:"id"`
	Version            int             `json:"version"`
	Content            string          `json:"content"`
	YAMLContent        string          `json:"yamlContent"` // Original YAML source code
	ContentHash        string          `json:"contentHash"`
	ContractID         string          `json:"contractId"`
	CreatedBy          string          `json:"createdBy"`
	Graph              json.RawMessage `json:"graph,omitempty"` // Contract graph (JSON)
	ExpectedDurationMs *int64          `json:"expectedDurationMs,omitempty"`
	IsDeleted          bool            `json:"isDeleted"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}
