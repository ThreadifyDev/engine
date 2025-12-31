-- update_step_state.lua
-- Atomically updates step state with execution graph tracking
-- Handles step completion, retry tracking, and thread status updates

-- KEYS[1]: thread:ID:meta (hash)
-- KEYS[2]: thread:ID:current_steps (sorted set)
-- KEYS[3]: thread:ID:steps:{stepName}:{idempKey} (hash)
-- KEYS[4]: thread:ID:violations (hash)

-- ARGV[1]: stepKey (stepName:idempKey)
-- ARGV[2]: stepID
-- ARGV[3]: status ("completed" | "violated" | "failed" | "pending")
-- ARGV[4]: timestamp (RFC3339)
-- ARGV[5]: violationJSON (empty string if none)
-- ARGV[6]: isTerminalStep ("true" | "false")

local metaKey = KEYS[1]
local currentStepsKey = KEYS[2]
local stepHashKey = KEYS[3]
local violationsKey = KEYS[4]

local stepKey = ARGV[1]
local stepID = ARGV[2]
local status = ARGV[3]
local timestamp = ARGV[4]
local violationJSON = ARGV[5]
local isTerminalStep = ARGV[6]

-- 1. Get current step(s) from sorted set to determine previousStep
local currentSteps = redis.call('ZRANGE', currentStepsKey, 0, -1, 'WITHSCORES')
local previousStep = ''

if #currentSteps > 0 then
    -- Use the most recent (highest score/timestamp) as previous
    -- currentSteps is array like: [stepKey1, score1, stepKey2, score2, ...]
    previousStep = currentSteps[#currentSteps - 1]  -- Second to last element is the last stepKey
end

-- 2. Check if step hash exists (for retry detection)
local existingStatus = redis.call('HGET', stepHashKey, 'status')
local isRetry = (existingStatus ~= false)

-- 3. Update/create step state hash
if isRetry then
    -- Increment retry count
    redis.call('HINCRBY', stepHashKey, 'retryCount', 1)
    redis.call('HSET', stepHashKey,
        'status', status,
        'lastUpdatedAt', timestamp,
        'latestStepID', stepID
    )
else
    -- Create new step state
    redis.call('HSET', stepHashKey,
        'status', status,
        'retryCount', '0',
        'firstSeenAt', timestamp,
        'lastUpdatedAt', timestamp,
        'latestStepID', stepID,
        'previousStep', previousStep
    )
end

-- 4. Update current_steps sorted set (only if completed)
if status == 'completed' then
    -- Remove previous step from current (if exists)
    if previousStep ~= '' then
        redis.call('ZREM', currentStepsKey, previousStep)
    end
    
    -- Add this step as current (use timestamp as score for ordering)
    local score = redis.call('TIME')[1]  -- Unix timestamp
    redis.call('ZADD', currentStepsKey, score, stepKey)
end

-- 5. Store violation if provided
if violationJSON ~= '' then
    redis.call('HSET', violationsKey, stepID, violationJSON)
end

-- 6. Update thread status if terminal step and completed
if isTerminalStep == 'true' and status == 'completed' then
    redis.call('HSET', metaKey,
        'status', 'completed',
        'completedAt', timestamp
    )
end

-- Return final status
return status
