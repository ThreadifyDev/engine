-- decrement_usage.lua
-- Atomically decrements a balance key with a floor check.
-- 
-- KEYS[1] = balance key (e.g., "plan:balance:ingress:company-123")
-- ARGV[1] = decrement amount (integer)
-- ARGV[2] = floor (minimum allowed balance, e.g., -500000 for 1.5M hard cap on 1M base)
-- 
-- Returns: { new_balance, allowed (1 if allowed, 0 if denied) }

local key = KEYS[1]
local decr_amount = tonumber(ARGV[1])
local floor = tonumber(ARGV[2])

-- If key does not exist, we treat it as 0 (but ideally it should be seeded)
-- Returning -99999 as a signal that the key was missing might be better for re-seeding
local current = redis.call('GET', key)
if not current then
    return { 0, -1 } -- Signal for "key missing, please seed"
end

local new_balance = tonumber(current) - decr_amount

if new_balance < floor then
    return { tonumber(current), 0 } -- Denied
end

redis.call('SET', key, tostring(new_balance))
return { new_balance, 1 } -- Allowed
