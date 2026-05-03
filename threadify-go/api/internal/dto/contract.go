package dto

import (
	"encoding/json"
	"time"
)

type Contract struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	CompanyID     string    `json:"companyId"`
	Description   string    `json:"description"`
	ContentHash   *string   `json:"contentHash"`
	LatestVersion int       `json:"latestVersion"`
	OwnerID       string    `json:"ownerId"`
	IsPublic      bool      `json:"isPublic"`
	IsDeleted     bool      `json:"isDeleted"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ContractVersion struct {
	ID                 string          `json:"id"`
	Version            int             `json:"version"`
	Content            string          `json:"content"`
	YAMLContent        string          `json:"yamlContent"`
	ContentHash        string          `json:"contentHash"`
	ContractID         string          `json:"contractId"`
	CreatedBy          string          `json:"createdBy"`
	Graph              json.RawMessage `json:"graph"`
	ExpectedDurationMs *int64          `json:"expectedDurationMs"`
	IsDeleted          bool            `json:"isDeleted"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type ContractListResponse struct {
	Contracts []*Contract `json:"contracts"`
	Total     int         `json:"total"`
}

type ContractWithOwnershipResponse struct {
	Contract *Contract `json:"contract"`
	IsOwner  bool      `json:"isOwner"`
}

type ContractVersionsResponse struct {
	Versions []*ContractVersion `json:"versions"`
	Name     string             `json:"name"`
	Total    int                `json:"totalVersions"`
}
