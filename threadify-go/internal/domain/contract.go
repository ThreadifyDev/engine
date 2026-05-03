package domain

import (
	"encoding/json"
	"time"
)

type Contract struct {
	ID            string
	Name          string
	CompanyID     string
	Description   string
	ContentHash   *string
	LatestVersion int
	OwnerID       string
	IsPublic      bool
	IsDeleted     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ContractVersion struct {
	ID                 string
	Version            int
	Content            string
	YAMLContent        string // Original YAML source code
	ContentHash        string
	ContractID         string
	CreatedBy          string
	Graph              json.RawMessage // Contract graph (JSON)
	ExpectedDurationMs *int64
	IsDeleted          bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
