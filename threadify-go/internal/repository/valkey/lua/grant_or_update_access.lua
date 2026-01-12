-- grant_or_update_access.lua
-- Unified access control with role uniqueness enforcement
-- Uses role_index for O(1) role conflict detection
-- Supports atomic thread creation to prevent orphaned threads
--
-- KEYS[1]: thread:{id}:role_index (hash: role -> userID)
-- KEYS[2]: thread:{id}:access (hash: userID -> access JSON)
-- KEYS[3]: thread:{id} (optional - thread hash key for creation)
-- ARGV[1]: invitedBy ("self" for creator, companyID/userID for others)
-- ARGV[2]: userID
-- ARGV[3]: role
-- ARGV[4]: permissions JSON array (e.g., '["read","write"]')
-- ARGV[5]: timestamp (RFC3339)
-- ARGV[6]: status
-- ARGV[7]: threadJSON (optional - thread data if creating, empty string if not)
-- ARGV[8]: threadTTL (optional - TTL in seconds for thread if creating)

local roleIndexKey = KEYS[1]
local accessKey = KEYS[2]
local threadKey = KEYS[3] or ""
local invitedBy = ARGV[1]
local userID = ARGV[2]
local newRole = ARGV[3]
local newPermissions = cjson.decode(ARGV[4])
local timestamp = ARGV[5]
local status = ARGV[6]
local threadJSON = ARGV[7] or ""
local threadTTL = tonumber(ARGV[8] or "0")

-- ATOMIC THREAD CREATION (if threadJSON provided)
-- This ensures thread and access are created together or not at all
if threadJSON ~= "" then
    -- Check if thread already exists
    if redis.call('EXISTS', threadKey) == 1 then
        return redis.error_reply("THREAD_ALREADY_EXISTS")
    end
    
    -- Create thread hash atomically with access grant
    redis.call('HSET', threadKey, 'data', threadJSON)
    
    -- Set TTL if provided
    if threadTTL > 0 then
        redis.call('EXPIRE', threadKey, threadTTL)
    end
end

-- Check role uniqueness: Only one user can hold a specific role
-- (Users can still have multiple roles, but each role is unique to one user)
local roleOwner = redis.call('HGET', roleIndexKey, newRole)
if roleOwner and roleOwner ~= userID then
    -- Role already assigned to another user
    return redis.error_reply("ROLE_ALREADY_ASSIGNED:" .. roleOwner)
end

-- Role is available or already owned by current user - proceed with granting access
local current = redis.call('HGET', accessKey, userID)

-- Multi-role accumulation logic - handles ALL cases where user already has access
if current then
    -- User already has access - check if they already have this role
    local access = cjson.decode(current)
    local hasRole = false
    for _, existingRole in ipairs(access.roles) do
        if existingRole == newRole then
            hasRole = true
            break
        end
    end
    
    if hasRole then
        -- User already has this specific role - idempotent, return current access
        return current
    else
        -- Add new role to existing access
        -- Safe to use HSET here because entire Lua script is atomic
        table.insert(access.roles, newRole)
        access.updated_at = timestamp
        access.version = (access.version or 0) + 1
        
        -- Update role index to claim this role
        redis.call('HSET', roleIndexKey, newRole, userID)
        
        local accessJSON = cjson.encode(access)
        redis.call('HSET', accessKey, userID, accessJSON)
        return accessJSON
    end
end

-- User doesn't have access - create new access with race detection
local access = {
    roles = {newRole},
    permissions = newPermissions,
    granted_by = invitedBy,
    granted_at = timestamp,
    status = status,
    version = 1
}

local accessJSON = cjson.encode(access)

-- Use HSETNX to detect concurrent creation (returns 1 if created, 0 if already exists)
local created = redis.call('HSETNX', accessKey, userID, accessJSON)

if created == 0 then
    -- Race detected: Another request created access first
    -- Return error to trigger retry in Go layer
    return redis.error_reply("CONCURRENT_ACCESS_CREATION")
end

-- Update role index to claim this role
redis.call('HSET', roleIndexKey, newRole, userID)

return accessJSON
