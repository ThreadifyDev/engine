package database

import (
	"context"
	"database/sql"
)

// InitSchema creates all necessary tables for the Web API
// Uses CREATE TABLE IF NOT EXISTS for idempotency
func InitSchema(ctx context.Context, db *sql.DB) error {
	schema := `
	-- Companies table (shared with Engine) - must be created first
	CREATE TABLE IF NOT EXISTS companies (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Add Web API specific columns to companies table
	ALTER TABLE companies ADD COLUMN IF NOT EXISTS industry VARCHAR(100);
	ALTER TABLE companies ADD COLUMN IF NOT EXISTS size VARCHAR(50);
	ALTER TABLE companies ADD COLUMN IF NOT EXISTS use_case TEXT;
	
	-- Add check constraint if it doesn't exist
	DO $$ 
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'companies_size_check') THEN
			ALTER TABLE companies ADD CONSTRAINT companies_size_check CHECK (size IN ('small', 'medium', 'large', 'enterprise'));
		END IF;
	END $$;

	-- Users table (shared with Engine)
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(255) PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Add Web API specific columns to users table
	ALTER TABLE users ADD COLUMN IF NOT EXISTS company_id VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS full_name VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS job_role VARCHAR(255);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN DEFAULT FALSE;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS onboarding_completed BOOLEAN DEFAULT FALSE;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS first_instrumentation_done BOOLEAN DEFAULT FALSE;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP;
	
	-- Add foreign key constraint if it doesn't exist
	DO $$ 
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_company_id_fkey') THEN
			ALTER TABLE users ADD CONSTRAINT users_company_id_fkey FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE;
		END IF;
	END $$;

	-- OTP codes table
	CREATE TABLE IF NOT EXISTS otp_codes (
		id VARCHAR(255) PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		code VARCHAR(255) NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		verified BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	-- Add password_changed_at column to users table for JWT-based password reset
	ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMP;

	-- Service accounts table
	CREATE TABLE IF NOT EXISTS service_accounts (
		id VARCHAR(255) PRIMARY KEY,
		company_id VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		role VARCHAR(100) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		last_used_at TIMESTAMP,
		created_by VARCHAR(255) NOT NULL
	);
	
	-- Add missing columns to service_accounts table
	ALTER TABLE service_accounts ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;
	ALTER TABLE service_accounts ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMP;
	ALTER TABLE service_accounts ADD COLUMN IF NOT EXISTS role VARCHAR(100);
	
	-- Drop old foreign key constraints that reference old table names
	ALTER TABLE service_accounts DROP CONSTRAINT IF EXISTS service_accounts_company_id_fkey;
	ALTER TABLE service_accounts DROP CONSTRAINT IF EXISTS service_accounts_created_by_fkey;
	
	-- Add correct foreign key constraints
	DO $$ 
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'service_accounts_company_fkey') THEN
			ALTER TABLE service_accounts ADD CONSTRAINT service_accounts_company_fkey 
				FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'service_accounts_created_by_users_fkey') THEN
			ALTER TABLE service_accounts ADD CONSTRAINT service_accounts_created_by_users_fkey 
				FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
		END IF;
	END $$;

	-- API keys table
	CREATE TABLE IF NOT EXISTS api_keys (
		id VARCHAR(255) PRIMARY KEY,
		service_account_id VARCHAR(255) NOT NULL,
		key_hash VARCHAR(255) NOT NULL UNIQUE,
		key_prefix VARCHAR(20) NOT NULL,
		name VARCHAR(255),
		expires_at TIMESTAMP,
		last_used_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		FOREIGN KEY (service_account_id) REFERENCES service_accounts(id) ON DELETE CASCADE
	);
	
	-- Add missing columns to api_keys table
	ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;
	ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMP;
	
	-- Drop old foreign key constraints that reference old table names
	ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_user_id_fkey;
	ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_company_id_fkey;

	-- User roles table (Engine uses principal_id/principal_type for polymorphism)
	CREATE TABLE IF NOT EXISTS user_roles (
		principal_id VARCHAR(255) NOT NULL,
		principal_type VARCHAR(50) NOT NULL DEFAULT 'user',
		role_name VARCHAR(100) NOT NULL,
		assigned_by VARCHAR(255) NOT NULL,
		assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
		PRIMARY KEY (principal_id, role_name)
	);
	
	-- Add id column if it doesn't exist (for Web API compatibility)
	ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS id VARCHAR(255);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_users_company ON users(company_id);
	CREATE INDEX IF NOT EXISTS idx_companies_created_at ON companies(created_at);
	CREATE INDEX IF NOT EXISTS idx_otp_codes_email ON otp_codes(email);
	CREATE INDEX IF NOT EXISTS idx_service_accounts_company ON service_accounts(company_id);
	CREATE INDEX IF NOT EXISTS idx_api_keys_service_account ON api_keys(service_account_id);
	CREATE INDEX IF NOT EXISTS idx_user_roles_principal ON user_roles(principal_id);

	-- Create trigger function for updated_at
	CREATE OR REPLACE FUNCTION update_updated_at_column()
	RETURNS TRIGGER AS $$
	BEGIN
		NEW.updated_at = NOW();
		RETURN NEW;
	END;
	$$ language 'plpgsql';

	-- Create triggers for updated_at
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
	`

	_, err := db.ExecContext(ctx, schema)
	return err
}
