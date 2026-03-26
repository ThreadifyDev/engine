-- decrement_credit_with_autotopup.lua
-- Atomically decrements credit balance, requests top-up if needed, and appends outbox events.
--
-- KEYS[1] = credit balance key
-- KEYS[2] = credit monthly charged key
-- KEYS[3] = usage outbox stream key
-- KEYS[4] = credit topup pending key
--
-- ARGV[1]  = cost (millicents)
-- ARGV[2]  = min_balance (millicents)
-- ARGV[3]  = topup_amount (millicents)
-- ARGV[4]  = max_monthly (millicents; 0 = disabled, no topup allowed)
-- ARGV[5]  = spend_event_id
-- ARGV[6]  = topup_event_id
-- ARGV[7]  = company_id
-- ARGV[8]  = billing_cycle_start (RFC3339)
-- ARGV[9]  = occurred_at (RFC3339)
-- ARGV[10] = allow_topup ("1" = allowed, "0" = disabled)
--
-- Returns: { new_balance, allowed, spend_stream_id, topup_stream_id, topup_applied }
--
-- allowed:
--   1  = debit accepted and applied
--   0  = denied (insufficient credit)
--  -1  = balance or charged key missing (caller should seed and retry)
--  -2  = invalid input
--
-- topup_applied:
--   1  = topup was requested this call
--   0  = no topup requested
 
local balance_key   = KEYS[1]
local charged_key   = KEYS[2]
local stream_key    = KEYS[3]
local pending_key   = KEYS[4]
 
local cost               = tonumber(ARGV[1])
local min_balance        = tonumber(ARGV[2])
local topup_amount       = tonumber(ARGV[3])
local max_monthly        = tonumber(ARGV[4])
local spend_event_id     = ARGV[5]
local topup_event_id     = ARGV[6]
local company_id         = ARGV[7]
local billing_cycle_start = ARGV[8]
local occurred_at        = ARGV[9]
local allow_topup        = (ARGV[10] == '1')
 
if not cost or not min_balance or not topup_amount or not max_monthly then
  return {0, -2, '', '', 0}
end
 
local raw_balance = redis.call('GET', balance_key)
local raw_charged = redis.call('GET', charged_key)
if not raw_balance or not raw_charged then
  return {0, -1, '', '', 0}
end
 
local balance_num = tonumber(raw_balance)
local charged_num = tonumber(raw_charged)
if not balance_num or not charged_num then
  return {0, -2, '', '', 0}
end
 
local pending_num = tonumber(redis.call('GET', pending_key) or '0') or 0
 
local new_balance = balance_num - cost
 
local topup_applied  = 0
local topup_stream_id = ''
 
-- Topup is only considered when explicitly allowed (active subscriptions only).
-- For cancelled/pending-cancellation plans allow_topup is false, so pending_num
-- is intentionally ignored for the topup path — it may reflect a stale pending
-- request from a prior cycle that was never fulfilled.
if allow_topup then
  local needs_topup = (new_balance < 0) or (min_balance > 0 and new_balance < min_balance)
  local under_monthly_cap = (max_monthly > 0) and ((charged_num + topup_amount) <= max_monthly)
  local can_request = (topup_amount > 0) and (pending_num <= 0) and under_monthly_cap
 
  if needs_topup and can_request then
    redis.call('SET', pending_key, tostring(topup_amount))
    topup_applied = 1
    topup_stream_id = redis.call('XADD', stream_key, '*',
      'event_id',            topup_event_id,
      'company_id',          company_id,
      'meter',               'credit_topup_request',
      'amount',              tostring(topup_amount),
      'billing_cycle_start', billing_cycle_start,
      'timestamp',           occurred_at
    )
  end
end
 

local available_cushion = 0
if allow_topup then
  available_cushion = pending_num
  if topup_applied == 1 then
    available_cushion = topup_amount
  end
end
 
if new_balance < 0 and (new_balance + available_cushion) < 0 then
  return {balance_num, 0, '', topup_stream_id, topup_applied}
end


redis.call('SET',    balance_key, tostring(new_balance))
redis.call('INCRBY', charged_key, cost)
 
local spend_stream_id = redis.call('XADD', stream_key, '*',
  'event_id',            spend_event_id,
  'company_id',          company_id,
  'meter',               'credit_spend',
  'amount',              tostring(-cost),
  'billing_cycle_start', billing_cycle_start,
  'timestamp',           occurred_at
)
 
return {new_balance, 1, spend_stream_id, topup_stream_id, topup_applied}