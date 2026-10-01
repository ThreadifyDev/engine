-- Cache a PostgreSQL thread snapshot only when no newer Valkey state exists.
-- A delayed read write-back must never revive a completed thread.
local threadKey, metaKey = KEYS[1], KEYS[2]
local snapshot, ttl = ARGV[1], tonumber(ARGV[2])
local status = redis.call('HGET', metaKey, 'status')
if redis.call('EXISTS', threadKey) == 1 or
   status == 'completed' or status == 'cancelled' or
   status == 'closed' or status == 'failed' then
    return 0
end

local thread = cjson.decode(snapshot)
redis.call('SET', threadKey, snapshot, 'EX', ttl)
redis.call('HSET', metaKey, 'status', thread.status)
if thread.completedAt then
    redis.call('HSET', metaKey, 'completedAt', thread.completedAt)
end
redis.call('EXPIRE', metaKey, ttl)
return 1
