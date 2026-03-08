-- decrement_usage_with_outbox.lua
-- Atomically decrements a balance key with a floor check and appends an outbox event.
--
-- KEYS[1] = balance key
-- KEYS[2] = usage outbox stream key
-- ARGV[1] = decrement amount
-- ARGV[2] = floor
-- ARGV[3] = event_id
-- ARGV[4] = company_id
-- ARGV[5] = meter
-- ARGV[6] = amount (event payload)
-- ARGV[7] = billing_cycle_start (RFC3339)
-- ARGV[8] = timestamp (RFC3339)
--
-- Returns: { new_balance, allowed, stream_id }
-- allowed:
--   1  = allowed and decremented
--   0  = denied (would go below floor)
--  -1  = key missing (caller should seed and retry)
--  -2  = invalid input/current value

local balance_key = KEYS[1]
local stream_key = KEYS[2]

local decr_amount = tonumber(ARGV[1])
local floor = tonumber(ARGV[2])

local event_id = ARGV[3]
local company_id = ARGV[4]
local meter = ARGV[5]
local amount = ARGV[6]
local billing_cycle_start = ARGV[7]
local occurred_at = ARGV[8]

if not decr_amount or not floor then
  return {0, -2, ''}
end

local current = redis.call('GET', balance_key)
if not current then
  return {0, -1, ''}
end

local current_num = tonumber(current)
if not current_num then
  return {0, -2, ''}
end

local new_balance = current_num - decr_amount
if new_balance < floor then
  return {current_num, 0, ''}
end

redis.call('SET', balance_key, tostring(new_balance))
local stream_id = redis.call('XADD', stream_key, '*',
  'event_id', event_id,
  'company_id', company_id,
  'meter', meter,
  'amount', amount,
  'billing_cycle_start', billing_cycle_start,
  'timestamp', occurred_at
)

return {new_balance, 1, stream_id}
