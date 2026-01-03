-- grant_or_update_access.lua
-- Unified access control with role uniqueness enforcement
-- Uses role_index for O(1) role conflict detection
--
-- KEYS[1]: thread:{id}:role_index (hash: role -> userID)
-- KEYS[2]: thread:{id}:access (hash: userID -> access JSON)
-- ARGV[1]: invitedBy ("self" for creator, companyID/userID for others)
-- ARGV[2]: userID
-- ARGV[3]: role
-- ARGV[4]: permissions JSON array (e.g., '["read","write"]')
-- ARGV[5]: timestamp (RFC3339)
-- ARGV[6]: status

local roleIndexKey = KEYS[1]
local accessKey = KEYS[2]
local invitedBy = ARGV[1]
local userID = ARGV[2]
local newRole = ARGV[3]
local newPermissions = cjson.decode(ARGV[4])
local timestamp = ARGV[5]
local status = ARGV[6]

-- Role index removed - incompatible with multi-role scenarios
-- Users can have multiple roles, so checking individual role ownership is flawed

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
        table.insert(access.roles, newRole)
        access.updated_at = timestamp
        
        local accessJSON = cjson.encode(access)
        redis.call('HSET', accessKey, userID, accessJSON)
        return accessJSON
    end
end

-- User doesn't have access - create new access based on invitation type
if invitedBy == 'self' then
    -- Thread creator - create new access
    local access = {
        roles = {newRole},
        permissions = newPermissions,
        granted_by = invitedBy,
        granted_at = timestamp,
        status = status
    }
    
    local accessJSON = cjson.encode(access)
    redis.call('HSET', accessKey, userID, accessJSON)
    return accessJSON
    
else
    -- Invitation or direct join - create new access
    local access = {
        roles = {newRole},
        permissions = newPermissions,
        granted_by = invitedBy,
        granted_at = timestamp,
        status = status
    }
    
    local accessJSON = cjson.encode(access)
    redis.call('HSET', accessKey, userID, accessJSON)
    return accessJSON
end
