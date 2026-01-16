package models

import "time"

// HashChainStatus represents the verification status of a thread's activity log hash chain
type HashChainStatus struct {
	Verified       bool       `json:"verified"`
	LastVerifiedAt time.Time  `json:"lastVerifiedAt"`
	TotalEvents    int        `json:"totalEvents"`
	BrokenAt       *time.Time `json:"brokenAt,omitempty"`
	Error          *string    `json:"error,omitempty"`
}
