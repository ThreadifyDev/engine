package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	// All table creation is now handled by the main engine's InitSchema
	// at /threadify-go/internal/database/postgres.go
	return nil
}
