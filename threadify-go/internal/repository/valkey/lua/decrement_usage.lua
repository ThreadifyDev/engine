-- decrement_usage.lua
-- Atomically decrements a balance key with a floor check.
--
-- KEYS[1] = counter key (e.g., "plan:usage:users:<company-id>" or "plan:usage:contracts:<company-id>")
-- ARGV[1] = decrement amount (integer)
-- ARGV[2] = floor (minimum allowed balance, e.g., -500000 for 1.5M hard cap on 1M base)
--
-- Returns: { new_balance, allowed }
-- allowed:
--   1  = allowed and decremented
--   0  = denied (would go below floor)
--  -1  = key missing (caller should seed and retry)
--  -2  = invalid current value (non-numeric / corrupted)

local key = KEYS[1]
local decr_amount = tonumber(ARGV[1])
local floor = tonumber(ARGV[2])

if not decr_amount or not floor then
    return { 0, -2 }
end

local current = redis.call('GET', key)
if not current then
    return { 0, -1 } -- Signal for "key missing, please seed"
end

local current_num = tonumber(current)
if not current_num then
    return { 0, -2 } -- invalid numeric value stored in key
end

local new_balance = current_num - decr_amount

if new_balance < floor then
    return { current_num, 0 } -- Denied
end

redis.call('SET', key, tostring(new_balance))
return { new_balance, 1 } -- Allowed
