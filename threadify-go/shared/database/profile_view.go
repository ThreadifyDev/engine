package database

// ProfileViewSchema upgrades existing profile types without changing their metrics.
const ProfileViewSchema = `
	-- Shared Overview definition. Additive migration preserves existing types and metrics.
	ALTER TABLE entity_profile_type ADD COLUMN IF NOT EXISTS view_definition JSONB;
	ALTER TABLE entity_profile_type ADD COLUMN IF NOT EXISTS view_requests JSONB;
	ALTER TABLE entity_profile_type ADD COLUMN IF NOT EXISTS view_revision BIGINT NOT NULL DEFAULT 0;
	ALTER TABLE entity_profile_type ADD COLUMN IF NOT EXISTS view_updated_at TIMESTAMPTZ;
	ALTER TABLE entity_profile_type ADD COLUMN IF NOT EXISTS view_updated_by TEXT;

`
