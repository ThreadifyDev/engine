-- check_and_update_thread_status.lua
-- Atomically checks and updates thread status to prevent double-ending
-- Returns {1, new_status} on success, {0, current_status} if already terminal

-- Use cjson library for JSON encoding (available as global in Redis/Valkey)
local cjson = cjson

-- ============================================================================
-- KEYS
-- ============================================================================
-- KEYS[1]: thread:ID:meta (hash)
-- KEYS[2]: thread:ID (JSON string)

-- ============================================================================
-- ARGV
-- ============================================================================
-- ARGV[1]: new_status ("completed" | "cancelled")
-- ARGV[2]: timestamp (RFC3339)
-- ARGV[3]: ttl (TTL in seconds from config)

local metaKey = KEYS[1]
local threadKey = KEYS[2]
local newStatus = ARGV[1]
local timestamp = ARGV[2]
local ttl = tonumber(ARGV[3]) or 604800  -- Default to 7 days if not provided

-- ============================================================================
-- SECTION 1: CHECK CURRENT STATUS
-- ============================================================================

-- Get current thread status from metadata hash
local currentStatus = redis.call('HGET', metaKey, 'status')

-- If already terminal, return 0 (skip) with current status
if currentStatus == 'completed' or currentStatus == 'cancelled' then
    return {0, currentStatus}
end

-- ============================================================================
-- SECTION 2: UPDATE THREAD STATUS
-- ============================================================================

-- Update metadata hash
redis.call('HSET', metaKey, 'status', newStatus, 'completedAt', timestamp)
redis.call('EXPIRE', metaKey, ttl)

-- Update thread JSON if it exists
local threadJSON = redis.call('GET', threadKey)
if threadJSON then
    local thread = cjson.decode(threadJSON)
    thread.status = newStatus
    thread.completedAt = timestamp
    redis.call('SET', threadKey, cjson.encode(thread), 'EX', ttl)
end

-- ============================================================================
-- SECTION 3: RETURN SUCCESS
-- ============================================================================

return {1, newStatus}
