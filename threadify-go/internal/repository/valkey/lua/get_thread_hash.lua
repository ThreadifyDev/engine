-- get_thread_hash.lua
-- Atomically retrieves the current hash from a thread object
-- Used as part of the hash chain generation process

-- KEYS[1]: thread:{threadID}
-- Returns: {currentHash} or error if thread not found

local threadKey = KEYS[1]
local threadData = redis.call('GET', threadKey)

if not threadData then
    return redis.error_reply("Thread not found")
end

local threadObj = cjson.decode(threadData)
return {threadObj.lastHash or ""}
