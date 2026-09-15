-- Backfill expected_duration_ms for existing contract_versions
-- This script adds the column if it doesn't exist, extracts max_duration from the graph JSONB column and converts it to milliseconds
-- Run this manually after deploying the schema changes

-- Ensure the column exists before updating
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'contract_versions'
        AND column_name = 'expected_duration_ms'
    ) THEN
        ALTER TABLE contract_versions ADD COLUMN expected_duration_ms BIGINT;
    END IF;
END $$;

-- Function to convert duration string to milliseconds
-- Supports: ms, s, m, h, d
CREATE OR REPLACE FUNCTION parse_duration_to_ms(duration TEXT)
RETURNS BIGINT AS $$
DECLARE
    num_part TEXT;
    unit_part TEXT;
    value NUMERIC;
    result BIGINT;
BEGIN
    -- Return NULL if input is NULL or empty
    IF duration IS NULL OR duration = '' THEN
        RETURN NULL;
    END IF;

    -- Trim whitespace
    duration := TRIM(duration);

    -- Extract numeric part and unit
    -- Match pattern: digits (with optional decimal) followed by unit
    IF duration ~ '^[0-9]+\.?[0-9]*[a-z]+$' THEN
        num_part := REGEXP_REPLACE(duration, '([0-9]+\.?[0-9]*)([a-z]+)', '\1');
        unit_part := REGEXP_REPLACE(duration, '([0-9]+\.?[0-9]*)([a-z]+)', '\2');
    ELSE
        RETURN NULL;
    END IF;

    -- Convert to numeric
    BEGIN
        value := num_part::NUMERIC;
    EXCEPTION WHEN OTHERS THEN
        RETURN NULL;
    END;

    -- Convert to milliseconds based on unit
    CASE unit_part
        WHEN 'ms' THEN result := value::BIGINT;
        WHEN 's' THEN result := (value * 1000)::BIGINT;
        WHEN 'm' THEN result := (value * 60 * 1000)::BIGINT;
        WHEN 'h' THEN result := (value * 60 * 60 * 1000)::BIGINT;
        WHEN 'd' THEN result := (value * 24 * 60 * 60 * 1000)::BIGINT;
        ELSE RETURN NULL;
    END CASE;

    RETURN result;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Update contract_versions with expected_duration_ms
-- This extracts graph->'validation'->>'max_duration' and converts it
UPDATE contract_versions
SET expected_duration_ms = parse_duration_to_ms(graph->'validation'->>'max_duration')
WHERE expected_duration_ms IS NULL
  AND graph IS NOT NULL
  AND graph->'validation'->>'max_duration' IS NOT NULL
  AND graph->'validation'->>'max_duration' != '';

-- Show results
SELECT
    COUNT(*) FILTER (WHERE expected_duration_ms IS NOT NULL) AS updated_count,
    COUNT(*) FILTER (WHERE expected_duration_ms IS NULL AND graph->'validation'->>'max_duration' IS NOT NULL) AS failed_count,
    COUNT(*) AS total_count
FROM contract_versions;

-- Optional: Drop the helper function after backfill
-- DROP FUNCTION IF EXISTS parse_duration_to_ms(TEXT);
