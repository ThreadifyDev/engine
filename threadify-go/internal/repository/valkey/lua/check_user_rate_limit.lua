-- check_user_rate_limit.lua
-- Implements sliding window rate limiting for user requests
--
-- KEYS[1] = rate limit key (e.g., "ratelimit:user:user-123")
-- ARGV[1] = current timestamp (Unix seconds)
-- ARGV[2] = window start timestamp (current - window_seconds)
-- ARGV[3] = max requests allowed in window
-- ARGV[4] = TTL for the key (seconds)
--
-- Returns:
--   1 = request allowed (under limit)
--   0 = request denied (over limit)

local key = KEYS[1]
local now = tonumber(ARGV[1])
local window_start = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

-- Remove old entries outside the window
redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

-- Count current requests in window
local current = redis.call('ZCARD', key)

if current < limit then
    -- Under limit: add new request and update TTL
    redis.call('ZADD', key, now, now)
    redis.call('EXPIRE', key, ttl)
    return 1
else
    -- Over limit: deny request
    return 0
end
