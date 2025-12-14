package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresDB struct {
	Pool *pgxpool.Pool
}

func NewPostgresDB(connString string, maxConns int) (*PostgresDB, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	config.MaxConns = int32(maxConns)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &PostgresDB{Pool: pool}, nil
}

func (db *PostgresDB) Close() {
	db.Pool.Close()
}

func (db *PostgresDB) InitSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS contracts (
		id UUID PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL,
		content_hash VARCHAR(64),
		latest_version INT NOT NULL DEFAULT 1,
		owner_id VARCHAR(255) NOT NULL,
		is_public BOOLEAN NOT NULL DEFAULT FALSE,
		is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Drop old constraint if it exists
	ALTER TABLE contracts DROP CONSTRAINT IF EXISTS unique_contract_name;
	
	-- Create partial unique index that only applies to non-deleted contracts
	CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_name_owner_active 
		ON contracts(name, owner_id) 
		WHERE is_deleted = false;

	ALTER TABLE contract_versions ADD COLUMN IF NOT EXISTS graph JSONB;

	CREATE TABLE IF NOT EXISTS contract_versions (
		id UUID PRIMARY KEY,
		version INT NOT NULL,
		content TEXT NOT NULL,
		content_hash VARCHAR(64) NOT NULL,
		contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
		created_by VARCHAR(255) NOT NULL DEFAULT 'Martins Joseph',
		graph JSONB,
		is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		CONSTRAINT unique_contract_version UNIQUE(contract_id, version)
	);

	CREATE INDEX IF NOT EXISTS idx_contract_versions_contract_id ON contract_versions(contract_id);
	CREATE INDEX IF NOT EXISTS idx_contract_versions_graph ON contract_versions USING GIN (graph) WHERE graph IS NOT NULL;

	CREATE TABLE IF NOT EXISTS threads (
		id VARCHAR(255) PRIMARY KEY,
		contract_id VARCHAR(255),
		contract_version INT,
		owner_id VARCHAR(255) NOT NULL,
		status VARCHAR(50) NOT NULL,
		current_step VARCHAR(255),
		context JSONB,
		steps JSONB,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		error TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_threads_owner_id ON threads(owner_id);
	CREATE INDEX IF NOT EXISTS idx_threads_contract_id ON threads(contract_id);
	CREATE INDEX IF NOT EXISTS idx_threads_status ON threads(status);
	CREATE INDEX IF NOT EXISTS idx_threads_started_at ON threads(started_at DESC);
	`

	_, err := db.Pool.Exec(ctx, schema)
	return err
}
