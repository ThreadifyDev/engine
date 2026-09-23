-- Check lifecycle and update references in one atomic operation.
local metaKey = KEYS[1]
local threadJSON = redis.call('GET', KEYS[2])
if not threadJSON then return redis.error_reply('Thread not found') end
local thread = cjson.decode(threadJSON)
local status = redis.call('HGET', metaKey, 'status')
local function terminal(value)
    return value == 'completed' or value == 'cancelled' or value == 'closed' or value == 'failed'
end
if terminal(status) or terminal(thread.status) then
    return redis.error_reply('Cannot add refs to ' .. (terminal(status) and status or thread.status) .. ' thread')
end
-- Validate every key before writing any fields.
for i = 2, #ARGV, 2 do
    if ARGV[i] == 'threadify.thread_key' then
        local stored = redis.call('HGET', metaKey, 'refs:threadify.thread_key')
        if not stored then stored = (thread.refs or {})['threadify.thread_key'] end
        if stored ~= ARGV[i + 1] then
            return redis.error_reply('threadKey cannot be changed through refs')
        end
    end
end
for i = 2, #ARGV, 2 do
    redis.call('HSET', metaKey, 'refs:' .. ARGV[i], ARGV[i + 1])
end
redis.call('EXPIRE', metaKey, ARGV[1])
return 1
