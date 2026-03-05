package database

import (
	"context"
	"database/sql"
)

func InitSchema(ctx context.Context, db *sql.DB) error {
	// All table creation is now handled by the main engine's InitSchema
	// at /threadify-go/internal/database/postgres.go
	return nil
}
