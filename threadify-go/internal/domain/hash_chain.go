package domain

import "time"

// HashChainStatus represents the verification status of a thread's activity log hash chain
type HashChainStatus struct {
	Verified       bool
	LastVerifiedAt time.Time
	TotalEvents    int
	BrokenAt       *time.Time
	Error          *string
}

// StepIntegrityStatus represents the verification status of a single step's hash
type StepIntegrityStatus struct {
	Verified bool
	Hash     string
	PrevHash string
	Error    *string
}
