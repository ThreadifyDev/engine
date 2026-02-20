package database

import (
	"context"
	"database/sql"
)

// InitSchema is deprecated - all schema initialization moved to main engine
// Web API now relies on the main engine to create all necessary tables
// This function is kept for backward compatibility but does nothing
func InitSchema(ctx context.Context, db *sql.DB) error {
	// All table creation is now handled by the main engine's InitSchema
	// at /threadify-go/internal/database/postgres.go
	return nil
}
