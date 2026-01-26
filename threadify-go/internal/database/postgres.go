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
		// Log internally but don't expose connection string details
		fmt.Printf("Failed to parse database config: %v\n", err)
		return nil, fmt.Errorf("failed to configure database connection")
	}

	config.MaxConns = int32(maxConns)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		fmt.Printf("Failed to create database pool: %v\n", err)
		return nil, fmt.Errorf("failed to connect to database")
	}

	if err := pool.Ping(context.Background()); err != nil {
		fmt.Printf("Failed to ping database: %v\n", err)
		return nil, fmt.Errorf("failed to connect to database")
	}

	return &PostgresDB{Pool: pool}, nil
}

func (db *PostgresDB) Close() {
	db.Pool.Close()
}

func (db *PostgresDB) InitSchema(ctx context.Context) error {
	schema := `
	-- Note: companies and users tables are now owned by Web API InitSchema
	
	CREATE TABLE IF NOT EXISTS contracts (
		id UUID PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		company_id VARCHAR(255) NOT NULL,
		description TEXT NOT NULL,
		content_hash VARCHAR(64),
		latest_version INT NOT NULL DEFAULT 1,
		owner_id VARCHAR(255) NOT NULL,
		is_public BOOLEAN NOT NULL DEFAULT FALSE,
		is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		FOREIGN KEY (company_id) REFERENCES companies(id)
	);

	-- Add company_id column if it doesn't exist (for existing databases)
	ALTER TABLE contracts ADD COLUMN IF NOT EXISTS company_id VARCHAR(255);
	
	-- Drop old constraint if it exists
	ALTER TABLE contracts DROP CONSTRAINT IF EXISTS unique_contract_name;
	DROP INDEX IF EXISTS idx_contracts_name_owner_active;
	
	-- Create partial unique index on (name, company_id) for non-deleted contracts
	CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_name_company_active 
		ON contracts(name, company_id) 
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

	CREATE TABLE IF NOT EXISTS threads (
		id VARCHAR(255) PRIMARY KEY,
		contract_id VARCHAR(255),
		contract_name VARCHAR(255),
		contract_version INT,
		owner_id VARCHAR(255) NOT NULL,
		company_id VARCHAR(255) NOT NULL,
		status VARCHAR(50) DEFAULT 'active',
		error TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		completed_at TIMESTAMP,
		FOREIGN KEY (owner_id) REFERENCES users(id),
		FOREIGN KEY (company_id) REFERENCES companies(id)
	);

	-- Add missing columns for existing databases (migration safety)
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS contract_name VARCHAR(255);
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'active';
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;

	-- Security-first indexes: company_id ALWAYS comes first to enforce isolation
	-- These replace the old indexes that didn't include company_id
	CREATE INDEX IF NOT EXISTS idx_threads_company_created 
		ON threads(company_id, created_at DESC);
	
	CREATE INDEX IF NOT EXISTS idx_threads_company_status_created 
		ON threads(company_id, status, created_at DESC);
	
	CREATE INDEX IF NOT EXISTS idx_threads_company_owner 
		ON threads(company_id, owner_id, created_at DESC);
	
	CREATE INDEX IF NOT EXISTS idx_threads_company_contract 
		ON threads(company_id, contract_name, created_at DESC);
	
	CREATE INDEX IF NOT EXISTS idx_threads_company_contract_version 
		ON threads(company_id, contract_name, contract_version, created_at DESC);
	
	-- Partial index for non-active threads (completed/failed queries)
	CREATE INDEX IF NOT EXISTS idx_threads_status_completed 
		ON threads(status, created_at DESC) WHERE status != 'active';
	
	-- Keep contract_id index for backward compatibility
	CREATE INDEX IF NOT EXISTS idx_threads_contract_id ON threads(contract_id);

	CREATE TABLE IF NOT EXISTS thread_refs (
		thread_id VARCHAR(255) NOT NULL,
		ref_key VARCHAR(255) NOT NULL,
		ref_value TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		PRIMARY KEY (thread_id, ref_key),
		FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_thread_refs_thread_id ON thread_refs(thread_id);
	CREATE INDEX IF NOT EXISTS idx_thread_refs_key ON thread_refs(ref_key);
	CREATE INDEX IF NOT EXISTS idx_thread_refs_value ON thread_refs(ref_value);
	CREATE INDEX IF NOT EXISTS idx_thread_refs_key_value ON thread_refs(ref_key, ref_value);
	
	-- Enhanced index for threadsByRef with date filtering
	CREATE INDEX IF NOT EXISTS idx_thread_refs_key_value_created 
		ON thread_refs(ref_key, ref_value, created_at DESC);

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
		prev_hash TEXT,
		status TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_thread_activities_thread_id ON thread_activities(thread_id, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_activity_type ON thread_activities(activity_type);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_step_id ON thread_activities(step_id) WHERE step_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_thread_activities_actor ON thread_activities(actor, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_service ON thread_activities(actor_service, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_payload_gin ON thread_activities USING gin(payload);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_prev_hash ON thread_activities(prev_hash);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_status ON thread_activities(status);
	
	-- Composite indexes for actor-based thread queries (critical for GraphQL performance)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_actor_thread 
		ON thread_activities(actor, thread_id, recorded_at DESC);
	
	CREATE INDEX IF NOT EXISTS idx_thread_activities_service_thread 
		ON thread_activities(actor_service, thread_id, recorded_at DESC);
	
	-- JSONB indexes for hash verification queries (step-level verification)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_step_name 
		ON thread_activities ((payload->>'step_name'));
	
	CREATE INDEX IF NOT EXISTS idx_thread_activities_idempotency_key 
		ON thread_activities ((payload->>'idempotency_key'));
	
	-- Composite index for step verification (thread_id + step_name + idempotency_key)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_step_verification 
		ON thread_activities (thread_id, (payload->>'step_name'), (payload->>'idempotency_key'));

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

	-- Add foreign key constraint for data integrity and query optimization
	DO $$ 
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint 
			WHERE conname = 'fk_thread_access_thread'
		) THEN
			ALTER TABLE thread_access 
				ADD CONSTRAINT fk_thread_access_thread 
				FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE;
		END IF;
	END $$;

	-- Basic indexes (keep for backward compatibility)
	CREATE INDEX IF NOT EXISTS idx_thread_access_thread_id ON thread_access(thread_id);
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_id ON thread_access(user_id);
	CREATE INDEX IF NOT EXISTS idx_thread_access_status ON thread_access(status);
	CREATE INDEX IF NOT EXISTS idx_thread_access_roles_gin ON thread_access USING GIN (roles);
	CREATE INDEX IF NOT EXISTS idx_thread_access_scope ON thread_access(scope) WHERE scope IS NOT NULL;

	-- Composite indexes for efficient JOIN queries with threads table
	-- For user-based thread filtering: "show me all threads user X has access to"
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_active_thread 
		ON thread_access(user_id, thread_id) 
		WHERE status = 'active';

	-- For thread-based user lookup: "who has access to thread Y"
	CREATE INDEX IF NOT EXISTS idx_thread_access_thread_active_user 
		ON thread_access(thread_id, user_id) 
		WHERE status = 'active';

	-- Covering index for permission checks without table lookup (PostgreSQL 11+)
	-- INCLUDE clause adds columns to index without making them part of the key
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_thread_covering 
		ON thread_access(user_id, thread_id) 
		INCLUDE (permissions, roles, status) 
		WHERE status = 'active';

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
	
	-- Composite index for GraphQL validationResults query (thread + step lookup)
	CREATE INDEX IF NOT EXISTS idx_thread_validations_thread_step 
		ON thread_validations(thread_id, step_name, idempotency_key, timestamp DESC);

	-- Step State Table: Archived snapshots of step state from Redis
	-- This provides fast queries for historical step state without reconstructing from activities
	CREATE TABLE IF NOT EXISTS thread_step_states (
		id VARCHAR(255) PRIMARY KEY,           -- Step UUID
		thread_id VARCHAR(255) NOT NULL,       -- Thread UUID
		step_name VARCHAR(255) NOT NULL,       -- Step name (e.g., 'order_placed')
		idempotency_key VARCHAR(255) NOT NULL, -- Idempotency key for deduplication
		status VARCHAR(50) NOT NULL,           -- Step status: success, failed, error
		retry_count INT NOT NULL DEFAULT 0,    -- Number of retries
		first_seen_at TIMESTAMP NOT NULL,      -- First time step was seen
		last_updated_at TIMESTAMP NOT NULL,    -- Last update timestamp
		previous_step VARCHAR(255),            -- Previous step name for transition tracking
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(thread_id, step_name, idempotency_key)
	);

	-- CRITICAL: GraphQL thread.steps() query - most common access pattern
	CREATE INDEX IF NOT EXISTS idx_step_states_thread_step 
		ON thread_step_states(thread_id, step_name);

	-- For filtering steps by status within a thread
	CREATE INDEX IF NOT EXISTS idx_step_states_thread_status 
		ON thread_step_states(thread_id, status);

	-- For retry analysis and queries filtering by retry count
	CREATE INDEX IF NOT EXISTS idx_step_states_retry 
		ON thread_step_states(retry_count) 
		WHERE retry_count > 0;

	-- For time-based queries and ordering by last update
	CREATE INDEX IF NOT EXISTS idx_step_states_thread_updated 
		ON thread_step_states(thread_id, last_updated_at DESC);

	-- For status-based analytics across all threads
	CREATE INDEX IF NOT EXISTS idx_step_states_status_updated 
		ON thread_step_states(status, last_updated_at DESC);

	-- Enhanced indexes for thread_activities to support GraphQL stepHistory query
	-- For stepHistory query with step_id filtering (critical performance!)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_step_recorded 
		ON thread_activities(thread_id, step_id, recorded_at DESC)
		WHERE step_id IS NOT NULL;

	-- For activityType filtering in stepHistory
	CREATE INDEX IF NOT EXISTS idx_thread_activities_thread_type_recorded 
		ON thread_activities(thread_id, activity_type, recorded_at DESC);

	-- Partial index for linkedThread lookups in threadChain queries
	CREATE INDEX IF NOT EXISTS idx_thread_refs_linked_threads 
		ON thread_refs(ref_value, created_at DESC)
		WHERE ref_key LIKE 'linkedThread:%';
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
