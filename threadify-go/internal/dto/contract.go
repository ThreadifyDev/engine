package dto

import (
	"encoding/json"
	"time"
)

// Contract represents a workflow contract for JSON serialization
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

// ContractVersion represents a specific version of a contract
type ContractVersion struct {
	Source             string          `json:"source"`
	SourceFormat       string          `json:"sourceFormat"`
	ID                 string          `json:"id"`
	Version            int             `json:"version"`
	Content            string          `json:"content"`
	YAMLContent        string          `json:"yamlContent"`
	ContentHash        string          `json:"contentHash"`
	ContractID         string          `json:"contractId"`
	ContractName       string          `json:"contractName"`
	CreatedBy          string          `json:"createdBy"`
	Graph              json.RawMessage `json:"graph"`
	ExpectedDurationMs *int64          `json:"expectedDurationMs"`
	IsDeleted          bool            `json:"isDeleted"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

// ContractListResponse represents paginated list of contracts
type ContractListResponse struct {
	Contracts []*Contract `json:"contracts"`
	Total     int         `json:"total"`
}

// ContractWithOwnershipResponse includes contract and ownership check
type ContractWithOwnershipResponse struct {
	Contract *Contract `json:"contract"`
	IsOwner  bool      `json:"isOwner"`
}

// ContractVersionsResponse represents list of versions for a contract
type ContractVersionsResponse struct {
	Versions      []*ContractVersion `json:"versions"`
	ContractID    string             `json:"contractId"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	LatestVersion int                `json:"latestVersion"`
	IsOwner       bool               `json:"isOwner"`
	Total         int                `json:"totalVersions"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}
