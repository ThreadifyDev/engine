package models

import "time"

type AuthInfo struct {
	OwnerID   string
	CompanyID string
	Role      string
	IsActive  bool
	ExpiresAt *time.Time
}
