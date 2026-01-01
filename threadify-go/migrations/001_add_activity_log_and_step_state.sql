-- Migration: Add activity_log and thread_step_state tables
-- Date: 2025-12-31
-- Description: Consolidate audit trail into activity_log and add normalized step state tracking

-- ============================================================================
-- Activity Log Table
-- ============================================================================
-- Stores all thread activity events (step_recorded, invitation_created, etc.)
-- This is the single source of truth for audit trail
CREATE TABLE IF NOT EXISTS activity_log (
    id TEXT PRIMARY KEY,                    -- Stream entry ID from Valkey
    thread_id TEXT NOT NULL,                -- Reference to threads table
    step_id TEXT,                           -- Composite "step_name:idempotency_key" (nullable for non-step events)
    type TEXT NOT NULL,                     -- Event type: step_recorded, step_status_changed, invitation_created, etc.
    timestamp TIMESTAMP NOT NULL,           -- Event timestamp
    payload JSONB NOT NULL,                 -- Full event data (context, actor, status, etc.)
    hash TEXT,                              -- Cryptographic hash for tamper detection (for step events)
    created_at TIMESTAMP DEFAULT NOW()      -- Record creation time
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_activity_thread_time ON activity_log(thread_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_activity_step ON activity_log(step_id) WHERE step_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_activity_type ON activity_log(type);
CREATE INDEX IF NOT EXISTS idx_activity_timestamp ON activity_log(timestamp DESC);

-- Add foreign key constraint if threads table exists
-- Uncomment if you want cascade delete behavior
-- ALTER TABLE activity_log ADD CONSTRAINT fk_activity_thread 
--     FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE;

COMMENT ON TABLE activity_log IS 'Unified audit trail for all thread activities';
COMMENT ON COLUMN activity_log.id IS 'Valkey stream entry ID';
COMMENT ON COLUMN activity_log.step_id IS 'Composite key: step_name:idempotency_key';
COMMENT ON COLUMN activity_log.type IS 'Event type: step_recorded, step_status_changed, invitation_created, invitation_used, thread_created, thread_completed, access_granted, validation_warning';
COMMENT ON COLUMN activity_log.payload IS 'Full event data in JSONB format for flexible querying';
COMMENT ON COLUMN activity_log.hash IS 'SHA256 hash for step events to detect tampering';


-- ============================================================================
-- Thread Step State Table
-- ============================================================================
-- Stores current state of steps (normalized data for fast queries)
-- This is a materialized view derived from activity_log
CREATE TABLE IF NOT EXISTS thread_step_state (
    id TEXT PRIMARY KEY,                    -- Step UUID
    thread_id TEXT NOT NULL,                -- Reference to threads table
    step_name TEXT NOT NULL,                -- Base step name (e.g., "order_placed")
    idempotency_key TEXT NOT NULL,         -- User-provided or context hash
    status TEXT NOT NULL,                   -- Current status: pending, completed, failed, violated
    retry_count INTEGER DEFAULT 0,          -- Number of retry attempts
    first_seen_at TIMESTAMP NOT NULL,       -- When step was first recorded
    last_updated_at TIMESTAMP NOT NULL,     -- Last status change timestamp
    previous_step TEXT,                     -- Previous step name for transition tracking
    created_at TIMESTAMP DEFAULT NOW(),     -- Record creation time
    
    -- Ensure uniqueness per thread
    CONSTRAINT uq_thread_step_idemp UNIQUE(thread_id, step_name, idempotency_key)
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_step_state_thread ON thread_step_state(thread_id);
CREATE INDEX IF NOT EXISTS idx_step_state_status ON thread_step_state(status);
CREATE INDEX IF NOT EXISTS idx_step_state_updated ON thread_step_state(last_updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_step_state_name ON thread_step_state(step_name);

-- Add foreign key constraint if threads table exists
-- Uncomment if you want cascade delete behavior
-- ALTER TABLE thread_step_state ADD CONSTRAINT fk_step_state_thread 
--     FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE;

COMMENT ON TABLE thread_step_state IS 'Current state of steps for fast operational queries (idempotency, retry checks)';
COMMENT ON COLUMN thread_step_state.idempotency_key IS 'User-provided key or auto-generated from context hash';
COMMENT ON COLUMN thread_step_state.status IS 'Current step status: pending, completed, failed, violated';
COMMENT ON COLUMN thread_step_state.retry_count IS 'Number of times this step has been retried';
COMMENT ON COLUMN thread_step_state.previous_step IS 'Previous step name for contract transition validation';


-- ============================================================================
-- Query Examples
-- ============================================================================

-- Get all activity for a thread (chronological order)
-- SELECT * FROM activity_log WHERE thread_id = 'thread-123' ORDER BY timestamp ASC;

-- Get all step events for a thread
-- SELECT * FROM activity_log WHERE thread_id = 'thread-123' AND type = 'step_recorded' ORDER BY timestamp ASC;

-- Get activity for a specific step
-- SELECT * FROM activity_log WHERE step_id = 'order_placed:abc123' ORDER BY timestamp ASC;

-- Get current state of all steps in a thread
-- SELECT * FROM thread_step_state WHERE thread_id = 'thread-123' ORDER BY first_seen_at ASC;

-- Check if step exists (idempotency check)
-- SELECT status FROM thread_step_state WHERE thread_id = 'thread-123' AND step_name = 'order_placed' AND idempotency_key = 'abc123';

-- Get failed steps that need retry
-- SELECT * FROM thread_step_state WHERE status = 'failed' AND retry_count < 3;

-- Audit trail: Reconstruct step history from activity log
-- SELECT 
--     timestamp,
--     type,
--     payload->>'status' as status,
--     payload->>'actor' as actor
-- FROM activity_log 
-- WHERE step_id = 'order_placed:abc123' 
-- ORDER BY timestamp ASC;
