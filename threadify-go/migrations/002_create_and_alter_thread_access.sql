-- Migration: Create thread_access table and convert roles to JSONB
-- Date: 2026-01-03
-- Description: Create thread_access table if missing, then convert roles column to JSONB

-- ============================================================================
-- Create thread_access table (if it doesn't exist)
-- ============================================================================

CREATE TABLE IF NOT EXISTS thread_access (
    id SERIAL PRIMARY KEY,
    thread_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    roles VARCHAR(100) NOT NULL,
    permissions TEXT,
    granted_by VARCHAR(255),
    granted_at TIMESTAMP NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMP,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(thread_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_thread_access_thread_id ON thread_access(thread_id);
CREATE INDEX IF NOT EXISTS idx_thread_access_user_id ON thread_access(user_id);
CREATE INDEX IF NOT EXISTS idx_thread_access_status ON thread_access(status);

-- ============================================================================
-- Alter thread_access table - Convert roles to JSONB
-- ============================================================================

-- Convert roles column from VARCHAR to JSONB
-- The USING clause handles the conversion of existing JSON strings to JSONB
ALTER TABLE thread_access 
ALTER COLUMN roles TYPE jsonb USING roles::jsonb;

-- Add GIN index for efficient JSONB queries on roles
-- This enables fast queries like: WHERE roles @> '["merchant"]'::jsonb
CREATE INDEX IF NOT EXISTS idx_thread_access_roles_gin ON thread_access USING GIN (roles);

COMMENT ON TABLE thread_access IS 'Thread access control - tracks which users have access to which threads';
COMMENT ON COLUMN thread_access.roles IS 'User roles as JSONB array (e.g., ["merchant", "warehouse_manager"])';
COMMENT ON COLUMN thread_access.permissions IS 'Comma-separated permissions (e.g., "read,write")';
COMMENT ON COLUMN thread_access.status IS 'Access status: active, revoked';

-- ============================================================================
-- Query Examples with JSONB
-- ============================================================================

-- Find all access records with a specific role
-- SELECT * FROM thread_access WHERE roles @> '["merchant"]'::jsonb;

-- Find all access records with any of multiple roles
-- SELECT * FROM thread_access WHERE roles ?| array['merchant', 'warehouse_manager'];

-- Count users per role
-- SELECT jsonb_array_elements_text(roles) as role, COUNT(*) 
-- FROM thread_access 
-- GROUP BY role;
