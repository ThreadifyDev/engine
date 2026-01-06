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

	CREATE TABLE IF NOT EXISTS contract_versions (
		id UUID PRIMARY KEY,
		version INT NOT NULL,
		content TEXT NOT NULL,
		yaml_content TEXT,
		content_hash VARCHAR(64) NOT NULL,
		contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
		created_by VARCHAR(255) NOT NULL DEFAULT 'Martins Joseph',
		graph JSONB,
		is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		CONSTRAINT unique_contract_version UNIQUE(contract_id, version)
	);

	ALTER TABLE contract_versions ADD COLUMN IF NOT EXISTS yaml_content TEXT;

	CREATE INDEX IF NOT EXISTS idx_contract_versions_contract_id ON contract_versions(contract_id);
	CREATE INDEX IF NOT EXISTS idx_contract_versions_graph ON contract_versions USING GIN (graph) WHERE graph IS NOT NULL;

	CREATE TABLE IF NOT EXISTS companies (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(255) PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		name VARCHAR(255),
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS threads (
		id VARCHAR(255) PRIMARY KEY,
		contract_id VARCHAR(255),
		contract_version INT,
		owner_id VARCHAR(255) NOT NULL,
		company_id VARCHAR(255) NOT NULL,
		error TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		FOREIGN KEY (owner_id) REFERENCES users(id),
		FOREIGN KEY (company_id) REFERENCES companies(id)
	);

	CREATE INDEX IF NOT EXISTS idx_threads_owner_id ON threads(owner_id);
	CREATE INDEX IF NOT EXISTS idx_threads_contract_id ON threads(contract_id);
	CREATE INDEX IF NOT EXISTS idx_threads_created_at ON threads(created_at DESC);

	CREATE TABLE IF NOT EXISTS thread_activities (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		thread_id TEXT NOT NULL,
		activity_type TEXT NOT NULL,
		step_id TEXT,
		actor TEXT NOT NULL,
		actor_service TEXT NOT NULL,
		payload JSONB NOT NULL,
		recorded_at TIMESTAMP NOT NULL DEFAULT NOW(),
		hash TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_thread_activities_thread_id ON thread_activities(thread_id, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_activity_type ON thread_activities(activity_type);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_step_id ON thread_activities(step_id) WHERE step_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_thread_activities_actor ON thread_activities(actor, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_service ON thread_activities(actor_service, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_payload_gin ON thread_activities USING gin(payload);

	CREATE TABLE IF NOT EXISTS thread_access (
		id SERIAL PRIMARY KEY,
		thread_id VARCHAR(255) NOT NULL,
		user_id VARCHAR(255) NOT NULL,
		roles JSONB NOT NULL,
		permissions TEXT,
		granted_by VARCHAR(255),
		granted_at TIMESTAMP NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMP,
		status VARCHAR(50) DEFAULT 'active',
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(thread_id, user_id)
	);

	-- Add scope column for notification access control
	ALTER TABLE thread_access ADD COLUMN IF NOT EXISTS scope TEXT;

	CREATE INDEX IF NOT EXISTS idx_thread_access_thread_id ON thread_access(thread_id);
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_id ON thread_access(user_id);
	CREATE INDEX IF NOT EXISTS idx_thread_access_status ON thread_access(status);
	CREATE INDEX IF NOT EXISTS idx_thread_access_roles_gin ON thread_access USING GIN (roles);
	CREATE INDEX IF NOT EXISTS idx_thread_access_scope ON thread_access(scope) WHERE scope IS NOT NULL;

	CREATE TABLE IF NOT EXISTS thread_validations (
		validation_id VARCHAR(255) PRIMARY KEY,
		thread_id VARCHAR(255) NOT NULL,
		step_id VARCHAR(255) NOT NULL,
		step_name VARCHAR(255) NOT NULL,
		idempotency_key VARCHAR(255),
		timestamp TIMESTAMP NOT NULL,
		validations JSONB NOT NULL,
		overall_status VARCHAR(50) NOT NULL,
		has_critical_violation BOOLEAN NOT NULL,
		critical_count INTEGER NOT NULL,
		warning_count INTEGER NOT NULL,
		minor_count INTEGER NOT NULL,
		info_count INTEGER NOT NULL,
		total_validations INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_thread_validations_thread_id ON thread_validations(thread_id);
	CREATE INDEX IF NOT EXISTS idx_thread_validations_step_id ON thread_validations(step_id);
	CREATE INDEX IF NOT EXISTS idx_thread_validations_timestamp ON thread_validations(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_validations_status ON thread_validations(overall_status);
	CREATE INDEX IF NOT EXISTS idx_thread_validations_critical ON thread_validations(has_critical_violation) WHERE has_critical_violation = true;
	`

	_, err := db.Pool.Exec(ctx, schema)
	if err != nil {
		return err
	}

	// Insert seed data for companies and users that correspond to static API keys
	err = db.insertSeedData(ctx)
	if err != nil {
		return fmt.Errorf("failed to insert seed data: %w", err)
	}

	return nil
}

// insertSeedData creates the companies and users that correspond to static API keys
func (db *PostgresDB) insertSeedData(ctx context.Context) error {
	// Insert companies
	companies := []struct {
		ID   string
		Name string
	}{
		{"company-abc", "Company ABC"},
		{"company-def", "Company DEF"},
		{"test-company", "Test Company"},
		{"demo-company", "Demo Company"},
	}

	for _, company := range companies {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO companies (id, name, created_at, updated_at)
			VALUES ($1, $2, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, company.ID, company.Name)
		if err != nil {
			return fmt.Errorf("failed to insert company %s: %w", company.ID, err)
		}
	}

	// Insert users
	users := []struct {
		ID    string
		Email string
		Name  string
	}{
		{"user-123", "user123@example.com", "User 123"},
		{"user-456", "user456@example.com", "User 456"},
		{"test-user", "test@example.com", "Test User"},
		{"demo-user", "demo@example.com", "Demo User"},
	}

	for _, user := range users {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO users (id, email, name, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, user.ID, user.Email, user.Name)
		if err != nil {
			return fmt.Errorf("failed to insert user %s: %w", user.ID, err)
		}
	}

	fmt.Printf("✅ Successfully inserted seed data for %d companies and %d users\n", len(companies), len(users))
	return nil
}
