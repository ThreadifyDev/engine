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

-- Check if role is already taken (O(1))
local roleOwner = redis.call('HGET', roleIndexKey, newRole)

if roleOwner then
    if roleOwner == userID then
        -- User already has this role - idempotent, return current access
        return redis.call('HGET', accessKey, userID)
    else
        -- Role taken by someone else
        return redis.error_reply("Role is already assigned")
    end
end

-- Role is available - proceed with granting access
local current = redis.call('HGET', accessKey, userID)

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
    redis.call('HSET', roleIndexKey, newRole, userID)
    return accessJSON
    
else
    -- Invitation or direct join
    
    if current then
        -- User already has access - add new role
        local access = cjson.decode(current)
        table.insert(access.roles, newRole)
        access.updated_at = timestamp
        
        local accessJSON = cjson.encode(access)
        redis.call('HSET', accessKey, userID, accessJSON)
        redis.call('HSET', roleIndexKey, newRole, userID)
        return accessJSON
        
    else
        -- User doesn't have access yet - create new
        local access = {
            roles = {newRole},
            permissions = newPermissions,
            granted_by = invitedBy,
            granted_at = timestamp,
            status = status
        }
        
        local accessJSON = cjson.encode(access)
        redis.call('HSET', accessKey, userID, accessJSON)
        redis.call('HSET', roleIndexKey, newRole, userID)
        return accessJSON
    end
end
