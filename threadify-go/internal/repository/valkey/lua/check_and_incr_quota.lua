-- check_and_incr_quota.lua
-- Atomically checks if a counter is under the limit and increments it.
-- 
-- KEYS[1] = count key (e.g., "plan:usage:contracts:company-123")
-- ARGV[1] = limit (-1 for unlimited)
-- ARGV[2] = increment amount (usually 1)
-- 
-- Returns: { new_count, allowed (1 if allowed, 0 if denied) }

local key = KEYS[1]
local limit = tonumber(ARGV[1])
local incr_amount = tonumber(ARGV[2])

local current = redis.call('GET', key)
if not current then
    return { 0, -1 } -- Signal for "key missing, please seed"
end

local actual_count = tonumber(current)

if limit ~= -1 and actual_count >= limit then
    return { actual_count, 0 } -- Denied
end

local new_count = redis.call('INCRBY', key, incr_amount)
return { new_count, 1 } -- Allowed
