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
		return nil, fmt.Errorf("failed to configure database connection: %w", err)
	}

	config.MaxConns = int32(maxConns)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return &PostgresDB{Pool: pool}, nil
}

func (db *PostgresDB) Close() {
	db.Pool.Close()
}

func (db *PostgresDB) InitSchema(ctx context.Context) error {
	schema := `
	-- Shared tables (also created by Web API for independence)
	CREATE TABLE IF NOT EXISTS companies (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		industry VARCHAR(100),
		size VARCHAR(50),
		use_case TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		external_customer_id VARCHAR(255)
	);

	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(255) PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		company_id VARCHAR(255),
		password_hash VARCHAR(255),
		auth_user_id VARCHAR(255),
		full_name VARCHAR(255),
		job_role VARCHAR(255),
		email_verified BOOLEAN DEFAULT FALSE,
		onboarding_completed BOOLEAN DEFAULT FALSE,
		first_instrumentation_done BOOLEAN DEFAULT FALSE,
		password_changed_at TIMESTAMP,
		last_login_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
	);
		ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_user_id VARCHAR(255);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_auth_user_id ON users(auth_user_id) WHERE auth_user_id IS NOT NULL;
	
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
		owner_id VARCHAR(255),  -- Service account ID (threads are always created by service accounts)
		company_id VARCHAR(255) NOT NULL,  -- Required - every thread must belong to a company
		status VARCHAR(50) DEFAULT 'active',
		error TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		completed_at TIMESTAMP,
		closed_at TIMESTAMP,
		FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
	);

	-- Add missing columns for existing databases (migration safety)
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS contract_name VARCHAR(255);
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'active';
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;
	ALTER TABLE threads ADD COLUMN IF NOT EXISTS closed_at TIMESTAMP;

	-- Migrate owner_id foreign key from users to service_accounts
	DO $$ 
	BEGIN
		-- Drop old constraint if it exists (references users)
		IF EXISTS (
			SELECT 1 FROM pg_constraint 
			WHERE conname = 'threads_owner_id_fkey'
			AND confrelid = 'users'::regclass
		) THEN
			ALTER TABLE threads DROP CONSTRAINT threads_owner_id_fkey;
		END IF;
		
		-- Add new constraint if it doesn't exist (references service_accounts)
		-- IMPORTANT: to_regclass returns NULL if the table doesn't exist yet, avoiding errors on fresh DBs.
		IF to_regclass('public.service_accounts') IS NOT NULL THEN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint 
				WHERE conname = 'threads_owner_id_fkey'
				AND confrelid = to_regclass('public.service_accounts')
			) THEN
				ALTER TABLE threads ADD CONSTRAINT threads_owner_id_fkey 
					FOREIGN KEY (owner_id) REFERENCES service_accounts(id) ON DELETE SET NULL;
			END IF;
		END IF;
	END $$;

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
		content_hash TEXT,
		status TEXT,
		started_at TIMESTAMP,
		finished_at TIMESTAMP,
		metadata JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Migration: Add content_hash column if it doesn't exist (for existing databases)
	ALTER TABLE thread_activities ADD COLUMN IF NOT EXISTS content_hash TEXT;
	
	-- Migration: Add metadata column if it doesn't exist (for SDK metadata from threadify_metadata)
	ALTER TABLE thread_activities ADD COLUMN IF NOT EXISTS metadata JSONB;

	CREATE INDEX IF NOT EXISTS idx_thread_activities_thread_id ON thread_activities(thread_id, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_activity_type ON thread_activities(activity_type);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_step_id ON thread_activities(step_id) WHERE step_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_thread_activities_actor ON thread_activities(actor, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_service ON thread_activities(actor_service, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_payload_gin ON thread_activities USING gin(payload);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_prev_hash ON thread_activities(prev_hash);
	CREATE INDEX IF NOT EXISTS idx_thread_activities_content_hash ON thread_activities(content_hash) WHERE content_hash IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_thread_activities_status ON thread_activities(status);
	
	-- GIN index for metadata JSONB column (for efficient querying of SDK metadata)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_metadata_gin 
		ON thread_activities USING gin(metadata) 
		WHERE metadata IS NOT NULL;
	
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
		runtime_role TEXT,
		permissions TEXT[] DEFAULT '{}',
		granted_by VARCHAR(255),
		granted_at TIMESTAMP NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMP,
		status VARCHAR(50) DEFAULT 'active',
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(thread_id, user_id)
	);

	-- Migrate existing scope data to runtime_role (if any)
	DO $$ 
	BEGIN
		-- Add runtime_role column if it doesn't exist
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_access' AND column_name = 'runtime_role'
		) THEN
			ALTER TABLE thread_access ADD COLUMN runtime_role TEXT;
		END IF;
		
		-- Migrate scope to runtime_role if scope column exists
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_access' AND column_name = 'scope'
		) THEN
			UPDATE thread_access SET runtime_role = scope WHERE scope IS NOT NULL AND runtime_role IS NULL;
			ALTER TABLE thread_access DROP COLUMN scope;
		END IF;
		
		-- Add permissions column if it doesn't exist
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_access' AND column_name = 'permissions'
		) THEN
			ALTER TABLE thread_access ADD COLUMN permissions TEXT[] DEFAULT '{}';
		END IF;
	END $$;

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

	-- Basic indexes
	CREATE INDEX IF NOT EXISTS idx_thread_access_thread_id ON thread_access(thread_id);
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_id ON thread_access(user_id);
	CREATE INDEX IF NOT EXISTS idx_thread_access_status ON thread_access(status);
	
	-- GIN index for thread roles (JSONB array) - for querying by specific thread role
	CREATE INDEX IF NOT EXISTS idx_thread_access_roles_gin ON thread_access USING GIN (roles);
	
	-- Index for runtime_role (notification/permission scope)
	CREATE INDEX IF NOT EXISTS idx_thread_access_runtime_role ON thread_access(runtime_role) WHERE runtime_role IS NOT NULL;
	
	-- GIN index for permissions array - for efficient permission-based notification queries
	CREATE INDEX IF NOT EXISTS idx_thread_access_permissions_gin ON thread_access USING GIN (permissions);
	
	-- Drop old scope index if it exists
	DROP INDEX IF EXISTS idx_thread_access_scope;

	-- Composite indexes for efficient JOIN queries with threads table
	-- For user-based thread filtering: "show me all threads user X has access to"
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_active_thread 
		ON thread_access(user_id, thread_id) 
		WHERE status = 'active';

	-- For thread-based user lookup: "who has access to thread Y"
	CREATE INDEX IF NOT EXISTS idx_thread_access_thread_active_user 
		ON thread_access(thread_id, user_id) 
		WHERE status = 'active';

	-- Covering index for access checks without table lookup (PostgreSQL 11+)
	-- INCLUDE clause adds columns to index without making them part of the key
	-- Drop old covering index with permissions
	DROP INDEX IF EXISTS idx_thread_access_user_thread_covering;
	
	-- Create new covering index with runtime_role instead of permissions
	CREATE INDEX IF NOT EXISTS idx_thread_access_user_thread_runtime_covering 
		ON thread_access(user_id, thread_id) 
		INCLUDE (runtime_role, roles, status) 
		WHERE status = 'active';

	-- Index for thread_access lookups (used for runtime_role-based data filtering)
	-- Note: Access control is company-wide (threads.company_id = user.company_id)
	-- thread_access is used to determine what data users can SEE, not whether they have access
	CREATE INDEX IF NOT EXISTS idx_thread_access_thread_user_status 
		ON thread_access(thread_id, user_id, status);

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

	-- Thread Notifications Table: Individual notifications from async validation system
	-- Stores full notification context (execution, validation, thread events)
	CREATE TABLE IF NOT EXISTS thread_notifications (
		notification_id VARCHAR(255) PRIMARY KEY,
		thread_id VARCHAR(255) NOT NULL,
		step_id VARCHAR(255) NOT NULL,
		step_name VARCHAR(255) NOT NULL,
		idempotency_key VARCHAR(255),
		
		-- Notification metadata
		source VARCHAR(50) NOT NULL,              -- 'execution', 'validation', 'thread'
		notification_type VARCHAR(100) NOT NULL,  -- 'execution.success', 'validation.violated', etc.
		
		-- Status fields
		step_status VARCHAR(50),                  -- 'success', 'failed', 'error' (from SDK)
		validation_status VARCHAR(50),            -- 'passed', 'violated', 'none'
		
		-- Violation details (nullable for non-violation notifications)
		violation_type VARCHAR(100),
		severity VARCHAR(50),                     -- 'critical', 'warning', 'info', 'major', 'minor'
		message TEXT NOT NULL,
		details JSONB,
		
		-- Timestamps
		timestamp TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Indexes for common query patterns
	CREATE INDEX IF NOT EXISTS idx_thread_notifications_thread ON thread_notifications(thread_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_notifications_step ON thread_notifications(step_id);
	CREATE INDEX IF NOT EXISTS idx_thread_notifications_source ON thread_notifications(thread_id, source);
	CREATE INDEX IF NOT EXISTS idx_thread_notifications_severity ON thread_notifications(thread_id, severity) WHERE severity IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_thread_notifications_type ON thread_notifications(notification_type);
	
	-- Composite index for filtered queries (e.g., critical violations for a thread)
	CREATE INDEX IF NOT EXISTS idx_thread_notifications_thread_severity 
		ON thread_notifications(thread_id, severity, timestamp DESC) WHERE severity IN ('critical', 'warning');

	-- Step Sub-Steps Table: Stores sub-steps for each parent step
	-- Sub-steps are granular operations within a step (e.g., validate_inventory, calculate_tax)
	-- Note: No FK constraint on thread_id to allow substeps to arrive before thread metadata
	CREATE TABLE IF NOT EXISTS step_substeps (
		id VARCHAR(255) PRIMARY KEY,
		thread_id VARCHAR(255) NOT NULL,
		step_id VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		status VARCHAR(50) NOT NULL,
		payload JSONB,
		recorded_at TIMESTAMP(6) NOT NULL,
		created_at TIMESTAMP(6) DEFAULT NOW()
	);

	-- Migration: Rename substep_name to name for existing databases
	DO $$ 
	BEGIN
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'step_substeps' AND column_name = 'substep_name'
		) THEN
			ALTER TABLE step_substeps RENAME COLUMN substep_name TO name;
		END IF;
	END $$;

	-- Drop FK constraint if it exists (for existing databases)
	ALTER TABLE step_substeps DROP CONSTRAINT IF EXISTS step_substeps_thread_id_fkey;

	-- Migration: Update timestamp columns to use microsecond precision
	DO $$ 
	BEGIN
		-- Update recorded_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'step_substeps' 
			AND column_name = 'recorded_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE step_substeps ALTER COLUMN recorded_at TYPE TIMESTAMP(6);
		END IF;

		-- Update created_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'step_substeps' 
			AND column_name = 'created_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE step_substeps ALTER COLUMN created_at TYPE TIMESTAMP(6);
		END IF;
	END $$;

	CREATE INDEX IF NOT EXISTS idx_substeps_step_id ON step_substeps(step_id);
	CREATE INDEX IF NOT EXISTS idx_substeps_thread_id ON step_substeps(thread_id);
	CREATE INDEX IF NOT EXISTS idx_substeps_recorded_at ON step_substeps(recorded_at DESC);

	-- Step State Table: Archived snapshots of step state from Redis
	-- This provides fast queries for historical step state without reconstructing from activities
	CREATE TABLE IF NOT EXISTS thread_step_states (
		id VARCHAR(255) PRIMARY KEY,           -- Step UUID
		thread_id VARCHAR(255) NOT NULL,       -- Thread UUID
		step_name VARCHAR(255) NOT NULL,       -- Step name (e.g., 'order_placed')
		idempotency_key VARCHAR(255) NOT NULL, -- Idempotency key for deduplication
		status VARCHAR(50) NOT NULL,           -- Step status: success, failed, error
		retry_count INT NOT NULL DEFAULT 0,    -- Number of retries
		actor_service VARCHAR(255),      						-- actor service from step activities
		latest_context JSONB,      												-- latest context from step activities
		first_seen_at TIMESTAMP(6) NOT NULL,      -- First time step was seen
		last_updated_at TIMESTAMP(6) NOT NULL,    -- Last update timestamp
		started_at TIMESTAMP(6),                  -- When step execution started
		finished_at TIMESTAMP(6),                 -- When step execution finished
		previous_step VARCHAR(255),            -- Previous step name for transition tracking
		actor VARCHAR(255),                    -- User who recorded this step (for .own permission filtering)
		created_at TIMESTAMP(6) NOT NULL DEFAULT NOW(),
		UNIQUE(thread_id, step_name, idempotency_key)
	);

	-- Add actor column if it doesn't exist (migration for existing databases)
	ALTER TABLE thread_step_states ADD COLUMN IF NOT EXISTS actor VARCHAR(255);
	
	-- Add actor_service and latest_context columns (migration for step state archival)
	ALTER TABLE thread_step_states ADD COLUMN IF NOT EXISTS actor_service VARCHAR(255);
	ALTER TABLE thread_step_states ADD COLUMN IF NOT EXISTS latest_context JSONB;
	
	-- Add started_at and finished_at columns (migration for timing data)
	ALTER TABLE thread_step_states ADD COLUMN IF NOT EXISTS started_at TIMESTAMP(6);
	ALTER TABLE thread_step_states ADD COLUMN IF NOT EXISTS finished_at TIMESTAMP(6);

	-- Migration: Update existing timestamp columns to use microsecond precision
	DO $$ 
	BEGIN
		-- Update first_seen_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_step_states' 
			AND column_name = 'first_seen_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE thread_step_states ALTER COLUMN first_seen_at TYPE TIMESTAMP(6);
		END IF;

		-- Update last_updated_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_step_states' 
			AND column_name = 'last_updated_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE thread_step_states ALTER COLUMN last_updated_at TYPE TIMESTAMP(6);
		END IF;

		-- Update started_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_step_states' 
			AND column_name = 'started_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE thread_step_states ALTER COLUMN started_at TYPE TIMESTAMP(6);
		END IF;

		-- Update finished_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_step_states' 
			AND column_name = 'finished_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE thread_step_states ALTER COLUMN finished_at TYPE TIMESTAMP(6);
		END IF;

		-- Update created_at column type
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name = 'thread_step_states' 
			AND column_name = 'created_at' 
			AND data_type = 'timestamp without time zone'
			AND datetime_precision IS DISTINCT FROM 6
		) THEN
			ALTER TABLE thread_step_states ALTER COLUMN created_at TYPE TIMESTAMP(6);
		END IF;
	END $$;

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

	-- For actor-based filtering (external user .own permission)
	CREATE INDEX IF NOT EXISTS idx_step_states_thread_actor 
		ON thread_step_states(thread_id, actor)
		WHERE actor IS NOT NULL;

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

	-- ========================================
	-- NOTIFICATION QUERY OPTIMIZATION INDEXES
	-- ========================================
	-- These indexes optimize GraphQL notification queries from thread_activities
	-- where activity_type='validation_result'

	-- 1. Composite index for base notification query (thread_id + activity_type + timestamp)
	-- Covers: WHERE thread_id = ? AND activity_type = 'validation_result' ORDER BY recorded_at DESC
	CREATE INDEX IF NOT EXISTS idx_thread_activities_notifications 
		ON thread_activities(thread_id, activity_type, recorded_at DESC)
		WHERE activity_type = 'validation_result';

	-- 2. Expression indexes for frequently filtered JSONB fields
	-- Note: source is always 'validation' for archived notifications (execution is ephemeral)
	
	-- Severity filter (critical/warning/major/minor/info)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_notif_severity 
		ON thread_activities(thread_id, (payload->>'severity'), recorded_at DESC)
		WHERE activity_type = 'validation_result';

	-- Notification type filter (validation.violated, validation.passed, etc.)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_notif_type 
		ON thread_activities(thread_id, (payload->>'notification_type'), recorded_at DESC)
		WHERE activity_type = 'validation_result';

	-- 3. Step-based notification queries
	-- For filtering by step_id or step_name
	CREATE INDEX IF NOT EXISTS idx_thread_activities_notif_step 
		ON thread_activities(thread_id, step_id, recorded_at DESC)
		WHERE activity_type = 'validation_result' AND step_id IS NOT NULL;

	-- 5. GIN index on payload for flexible JSONB queries (already exists globally, but add partial for notifications)
	CREATE INDEX IF NOT EXISTS idx_thread_activities_notif_payload_gin 
		ON thread_activities USING gin(payload)
		WHERE activity_type = 'validation_result';

	-- ========================================
	-- WEB API TABLES
	-- ========================================
	-- Tables specific to the Web API (authentication, service accounts, etc.)

	-- OTP codes table (for email verification)
	CREATE TABLE IF NOT EXISTS otp_codes (
		id VARCHAR(255) PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		code VARCHAR(255) NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		verified BOOLEAN NOT NULL DEFAULT FALSE,
		invalidated BOOLEAN NOT NULL DEFAULT FALSE,
		attempt_count INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);
	ALTER TABLE otp_codes ADD COLUMN IF NOT EXISTS invalidated BOOLEAN NOT NULL DEFAULT FALSE;
	ALTER TABLE otp_codes ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0;

	CREATE INDEX IF NOT EXISTS idx_otp_codes_email ON otp_codes(email);
	CREATE INDEX IF NOT EXISTS idx_otp_codes_expires ON otp_codes(expires_at);
	CREATE INDEX IF NOT EXISTS idx_otp_codes_active ON otp_codes(email, verified, invalidated, expires_at, created_at DESC);

	-- Service accounts table (for API access)
	CREATE TABLE IF NOT EXISTS service_accounts (
		id VARCHAR(255) PRIMARY KEY,
		company_id VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		is_active BOOLEAN DEFAULT TRUE,
		last_used_at TIMESTAMP,
		created_by VARCHAR(255),
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE,
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
	);

	-- Ensure threads.owner_id references service_accounts when possible.
	-- (threads table may be created before service_accounts on fresh DB init)
	DO $$
	BEGIN
		IF to_regclass('public.threads') IS NOT NULL AND to_regclass('public.service_accounts') IS NOT NULL THEN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'threads_owner_id_fkey'
				AND confrelid = to_regclass('public.service_accounts')
			) THEN
				ALTER TABLE threads ADD CONSTRAINT threads_owner_id_fkey
					FOREIGN KEY (owner_id) REFERENCES service_accounts(id) ON DELETE SET NULL;
			END IF;
		END IF;
	END $$;

	CREATE INDEX IF NOT EXISTS idx_service_accounts_company ON service_accounts(company_id);
	CREATE INDEX IF NOT EXISTS idx_service_accounts_company_id ON service_accounts(company_id);

	-- API keys table (for service account authentication)
	CREATE TABLE IF NOT EXISTS api_keys (
		id VARCHAR(255) PRIMARY KEY,
		service_account_id VARCHAR(255) NOT NULL,
		user_id VARCHAR(255),
		company_id VARCHAR(255) NOT NULL,
		key_hash VARCHAR(255) NOT NULL UNIQUE,
		key_prefix VARCHAR(20) NOT NULL,
		name VARCHAR(255),
		is_active BOOLEAN DEFAULT TRUE,
		expires_at TIMESTAMP,
		last_used_at TIMESTAMP,
		revoked_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		FOREIGN KEY (service_account_id) REFERENCES service_accounts(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
		FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_api_keys_service_account ON api_keys(service_account_id);
	CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);
	CREATE INDEX IF NOT EXISTS idx_api_keys_company ON api_keys(company_id);
	CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

	-- User roles table (for RBAC)
	-- Engine uses principal_id/principal_type for polymorphism (users + service accounts)
	CREATE TABLE IF NOT EXISTS user_roles (
		id VARCHAR(255),
		principal_id VARCHAR(255) NOT NULL,
		principal_type VARCHAR(50) NOT NULL DEFAULT 'user',
		role_name VARCHAR(100) NOT NULL,
		assigned_by VARCHAR(255) NOT NULL,
		assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
		PRIMARY KEY (principal_id, role_name)
	);

	CREATE INDEX IF NOT EXISTS idx_user_roles_principal ON user_roles(principal_id);
	CREATE INDEX IF NOT EXISTS idx_user_roles_type ON user_roles(principal_type);

	-- ========================================
	-- TRIGGERS FOR AUTO-UPDATED TIMESTAMPS
	-- ========================================

	-- Create trigger function for updated_at (idempotent)
	CREATE OR REPLACE FUNCTION update_updated_at_column()
	RETURNS TRIGGER AS $$
	BEGIN
		NEW.updated_at = NOW();
		RETURN NEW;
	END;
	$$ language 'plpgsql';

	-- Apply triggers to all tables with updated_at column
	DROP TRIGGER IF EXISTS update_users_updated_at ON users;
	CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
		FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

	DROP TRIGGER IF EXISTS update_companies_updated_at ON companies;
	CREATE TRIGGER update_companies_updated_at BEFORE UPDATE ON companies
		FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

	DROP TRIGGER IF EXISTS update_service_accounts_updated_at ON service_accounts;
	CREATE TRIGGER update_service_accounts_updated_at BEFORE UPDATE ON service_accounts
		FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

	DROP TRIGGER IF EXISTS update_api_keys_updated_at ON api_keys;
	CREATE TRIGGER update_api_keys_updated_at BEFORE UPDATE ON api_keys
		FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

	-- Agent AI Chat tables
	CREATE TABLE IF NOT EXISTS agent_conversations (
		id VARCHAR(255) PRIMARY KEY,
		user_id VARCHAR(255) NOT NULL,
		company_id VARCHAR(255) NOT NULL,
		title TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	-- Add message_count and token_count columns if they don't exist
	DO $$ 
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='agent_conversations' AND column_name='message_count') THEN
			ALTER TABLE agent_conversations ADD COLUMN message_count INT DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='agent_conversations' AND column_name='token_count') THEN
			ALTER TABLE agent_conversations ADD COLUMN token_count INT DEFAULT 0;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='agent_conversations' AND column_name='parent_conversation_id') THEN
			ALTER TABLE agent_conversations ADD COLUMN parent_conversation_id VARCHAR(255);
		END IF;
	END $$;

	CREATE TABLE IF NOT EXISTS agent_messages (
		id VARCHAR(255) PRIMARY KEY,
		conversation_id VARCHAR(255) NOT NULL,
		role VARCHAR(50) NOT NULL,
		content TEXT,
		tool_calls TEXT,
		tool_call_id VARCHAR(255),
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES agent_conversations(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS agent_context (
		id VARCHAR(255) PRIMARY KEY,
		conversation_id VARCHAR(255) NOT NULL,
		context_key VARCHAR(255) NOT NULL,
		context_value TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES agent_conversations(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_agent_messages_conversation ON agent_messages(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_agent_conversations_user ON agent_conversations(user_id);
	CREATE INDEX IF NOT EXISTS idx_agent_context_conversation ON agent_context(conversation_id);

	-- Outbox Pattern tables
	CREATE TABLE IF NOT EXISTS outbox_events (
		id UUID PRIMARY KEY,
		type VARCHAR(255) NOT NULL,
		payload BYTEA NOT NULL,
		status VARCHAR(50) NOT NULL DEFAULT 'pending',
		retry_count INT DEFAULT 0,
		max_retries INT DEFAULT 5,
		next_run_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		error_log TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_outbox_events_status_next_run 
		ON outbox_events(status, next_run_at) 
		WHERE status IN ('pending', 'failed');	

	ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS reference_id TEXT;
	CREATE INDEX IF NOT EXISTS idx_outbox_reference_id ON outbox_events(reference_id);

	ALTER TABLE companies ADD COLUMN IF NOT EXISTS external_customer_id VARCHAR(255);

	CREATE TABLE IF NOT EXISTS credit_accounts (
		id                       VARCHAR(255) PRIMARY KEY,
		company_id               VARCHAR(255) NOT NULL,
		billing_cycle_start      TIMESTAMP    NOT NULL,
		credit_balance_millicents BIGINT      NOT NULL DEFAULT 0,
		credit_min_balance_millicents BIGINT  NOT NULL DEFAULT 0,
		credit_max_monthly_charge_millicents BIGINT NOT NULL DEFAULT 0,
		credit_auto_topup_millicents BIGINT   NOT NULL DEFAULT 0,
		credit_monthly_charged_millicents BIGINT NOT NULL DEFAULT 0,
		last_sync_event_id       VARCHAR(255),
		rate_limit_tps           BIGINT,
		payload_limit_bytes      BIGINT,
		created_at               TIMESTAMP    NOT NULL DEFAULT NOW(),
		updated_at               TIMESTAMP    NOT NULL DEFAULT NOW(),
		UNIQUE(company_id, billing_cycle_start),
		FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_credit_accounts_company ON credit_accounts(company_id);
	
	CREATE TABLE IF NOT EXISTS billing_snapshots (
		id                       VARCHAR(255) PRIMARY KEY,
		company_id               VARCHAR(255) NOT NULL,
		reason                   TEXT,
		period_start             TIMESTAMPTZ  NOT NULL,
		period_end               TIMESTAMPTZ  NOT NULL,
		total_cents              BIGINT       NOT NULL DEFAULT 0,
		provider_name            VARCHAR(50)  NOT NULL DEFAULT '',
		external_invoice_id      VARCHAR(255) NOT NULL DEFAULT '',
		external_customer_id     VARCHAR(255) NOT NULL DEFAULT '',
		payment_status           VARCHAR(50)  NOT NULL DEFAULT 'no_charge',
		created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
		FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_billing_snapshots_company
		ON billing_snapshots(company_id, period_end DESC);

	CREATE INDEX IF NOT EXISTS idx_billing_snapshots_external_invoice_id
		ON billing_snapshots(external_invoice_id);
	`
	_, err := db.Pool.Exec(ctx, schema)
	return err
}
