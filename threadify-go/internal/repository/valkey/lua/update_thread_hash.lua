-- update_thread_hash.lua
-- Atomically updates thread hash with optimistic locking to prevent race conditions
-- Ensures hash chain integrity when multiple services publish steps concurrently

-- KEYS[1]: thread:{threadID}
-- ARGV[1]: expectedOldHash (the hash we read before)
-- ARGV[2]: newHash (calculated hash)
-- ARGV[3]: version (hash version)

-- Returns: {1} on success, {0} if retry needed (hash changed)

local threadKey = KEYS[1]
local expectedOldHash = ARGV[1]
local newHash = ARGV[2]
local version = ARGV[3]

local threadData = redis.call('GET', threadKey)
if not threadData then
    return redis.error_reply("Thread not found")
end

local threadObj = cjson.decode(threadData)
-- Admission and hash mutation share one atomic operation with completion.
local status = threadObj.status
local metaStatus = redis.call('HGET', threadKey .. ':meta', 'status')
local function terminal(value)
    return value == 'completed' or value == 'cancelled' or value == 'closed' or value == 'failed'
end
if terminal(status) or terminal(metaStatus) then
    return redis.error_reply('Cannot add steps to ' .. (terminal(metaStatus) and metaStatus or status) .. ' thread')
end
local currentHash = threadObj.lastHash or ""

-- Check if hash changed (race condition detected)
if currentHash ~= expectedOldHash then
    return {0}  -- Retry needed
end

-- Update atomically
threadObj.lastHash = newHash
threadObj.hashVersion = version
local ttl = redis.call('PTTL', threadKey)
if ttl >= 0 then
    if ttl == 0 then
        ttl = 1
    end
    redis.call('SET', threadKey, cjson.encode(threadObj), 'PX', ttl)
else
    redis.call('SET', threadKey, cjson.encode(threadObj))
end

return {1}  -- Success
