package domain

// UserRoleInfo contains minimal user info for notification routing
// Used by both Valkey and PostgreSQL repositories
type UserRoleInfo struct {
	UserID      string
	RuntimeRole string
}

// UserPermissionInfo contains user info with permissions for .own filtering
// Used for permission-based notification routing
type UserPermissionInfo struct {
	UserID      string
	Permissions []string
}
