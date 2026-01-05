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
-- ARGV[7]: maxRetries (from contract, "0" if not configured)

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
local maxRetries = tonumber(ARGV[7]) or 0

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

-- 3. Check retry limit if this is a retry and maxRetries is configured
local retryLimitViolated = false
if isRetry and maxRetries > 0 then
    -- Get current retry count
    local currentRetryCountStr = redis.call('HGET', stepHashKey, 'retryCount')
    local currentRetryCount = tonumber(currentRetryCountStr) or 0
    local nextRetryCount = currentRetryCount + 1
    
    -- Check if next retry would exceed limit
    if nextRetryCount > maxRetries then
        retryLimitViolated = true
        
        -- Extract stepName from stepKey (format: stepName:idempKey)
        local stepName = string.match(stepKey, '([^:]+):')
        
        -- Create retry limit violation
        local retryViolation = string.format(
            '{"violationType":"retry_limit_exceeded","severity":"critical","message":"Step \'%s\' exceeded retry limit of %d (current: %d retries)","stepName":"%s","stepId":"%s","retryCount":%d,"maxRetries":%d,"violatedAt":"%s"}',
            stepName, maxRetries, nextRetryCount, stepName, stepID, nextRetryCount, maxRetries, timestamp
        )
        
        -- Merge with existing violations
        if violationJSON ~= '' then
            -- Remove trailing ] and add comma
            violationJSON = string.gsub(violationJSON, '%]$', '')
            violationJSON = violationJSON .. ',' .. retryViolation .. ']'
        else
            violationJSON = '[' .. retryViolation .. ']'
        end
        
        -- Override status to violated
        status = 'violated'
    end
end

-- 4. Update/create step state hash
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

-- 5. Update current_steps sorted set (only if completed)
if status == 'completed' then
    -- Remove previous step from current (if exists)
    if previousStep ~= '' then
        redis.call('ZREM', currentStepsKey, previousStep)
    end
    
    -- Add this step as current (use timestamp as score for ordering)
    local score = redis.call('TIME')[1]  -- Unix timestamp
    redis.call('ZADD', currentStepsKey, score, stepKey)
end

-- 6. Store violation if provided
if violationJSON ~= '' then
    redis.call('HSET', violationsKey, stepID, violationJSON)
end

-- 7. Update thread status if terminal step and completed
if isTerminalStep == 'true' and status == 'completed' then
    redis.call('HSET', metaKey,
        'status', 'completed',
        'completedAt', timestamp
    )
end

-- 8. Write step state change to activity log stream for archival
-- Extract thread_id from metaKey (thread:ID:meta)
local threadID = string.match(metaKey, 'thread:([^:]+):meta')
-- Extract step_name and idempotency_key from stepKey (stepName:idempKey)
local stepName, idempKey = string.match(stepKey, '([^:]+):(.+)')

-- Get current step state for payload
local retryCount = redis.call('HGET', stepHashKey, 'retryCount') or '0'
local firstSeenAt = redis.call('HGET', stepHashKey, 'firstSeenAt') or timestamp

-- Determine partition based on thread_id hash
local partition = tonumber(string.sub(threadID, 1, 8), 16) % 10

-- Write to partitioned activity log stream
redis.call('XADD', 'streams:activity_log:' .. partition, '*',
    'thread_id', threadID,
    'type', 'step_state_changed',
    'step_id', 'steps:' .. stepKey,  -- Composite: steps:stepName:idempKey
    'actor', 'threadify-validator',
    'actor_service', 'business-validation-service',
    'status', status,
    'retry_count', retryCount,
    'first_seen_at', firstSeenAt,
    'last_updated_at', timestamp,
    'previous_step', previousStep,
    'violations', violationJSON,
    'timestamp', timestamp
)

-- Return final status and retry violation flag (format: "status|retryViolated")
-- Go code will parse this to create validation notification if needed
if retryLimitViolated then
    return status .. '|retry_violated'
else
    return status
end
