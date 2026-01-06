-- validate_and_update_step_state.lua
-- Atomically validates step transitions, terminal states, retry limits, and updates step state
-- Returns JSON with all validation results

-- Load cjson library for JSON encoding
local cjson = require('cjson')

-- ============================================================================
-- KEYS
-- ============================================================================
-- KEYS[1]: thread:ID:meta (hash)
-- KEYS[2]: thread:ID:current_steps (sorted set)
-- KEYS[3]: thread:ID:steps:{stepName}:{idempKey} (hash)
-- KEYS[4]: thread:ID:violations (hash)

-- ============================================================================
-- ARGV
-- ============================================================================
-- ARGV[1]: stepKey (stepName:idempKey)
-- ARGV[2]: stepID
-- ARGV[3]: status ("completed" | "violated" | "failed" | "pending")
-- ARGV[4]: timestamp (RFC3339)
-- ARGV[5]: existingViolationsJSON (from Go, empty string if none)
-- ARGV[6]: isTerminalStep ("true" | "false")
-- ARGV[7]: maxRetries (from contract, "0" if not configured)
-- ARGV[8]: allowedTransitions (JSON array: ["step1","step2","step3"])
-- ARGV[9]: terminalSteps (JSON array: ["delivered","cancelled"])
-- ARGV[10]: allowMultipleTerminals ("true" | "false")

local metaKey = KEYS[1]
local currentStepsKey = KEYS[2]
local stepHashKey = KEYS[3]
local violationsKey = KEYS[4]

local stepKey = ARGV[1]
local stepID = ARGV[2]
local status = ARGV[3]
local timestamp = ARGV[4]
local existingViolationsJSON = ARGV[5]
local isTerminalStep = ARGV[6]
local maxRetries = tonumber(ARGV[7]) or 0
local allowedTransitionsJSON = ARGV[8]
local terminalStepsJSON = ARGV[9]
local allowMultipleTerminals = ARGV[10]

-- Extract stepName from stepKey (format: stepName:idempKey)
local stepName = string.match(stepKey, '([^:]+):')

-- Validate stepName extraction
if not stepName then
    return cjson.encode({
        status = 'error',
        violations = {{
            violationType = 'invalid_step_key',
            severity = 'critical',
            message = 'Invalid step key format: ' .. stepKey .. ' (expected format: stepName:idempKey)'
        }},
        retryCount = 0,
        hasCriticalViolation = true
    })
end

-- ============================================================================
-- SECTION 1: READ CURRENT STATE
-- ============================================================================

-- Get current steps from sorted set
local currentSteps = redis.call('ZRANGE', currentStepsKey, 0, -1, 'WITHSCORES')
local previousStepKey = ''
local previousStepName = ''

if #currentSteps > 0 then
    previousStepKey = currentSteps[#currentSteps - 1]
    previousStepName = string.match(previousStepKey, '([^:]+):')
end

-- Check if step hash exists (for retry detection)
local existingStatus = redis.call('HGET', stepHashKey, 'status')
local isRetry = (existingStatus ~= false)

-- Get current retry count
local currentRetryCount = tonumber(redis.call('HGET', stepHashKey, 'retryCount')) or 0

-- ============================================================================
-- SECTION 2: VALIDATION CHECKS
-- ============================================================================

local violations = {}
local hasCriticalViolation = false

-- Parse allowed transitions
local allowedTransitions = {}
if allowedTransitionsJSON ~= '' and allowedTransitionsJSON ~= '[]' then
    local allowedTransitionsTable = cjson.decode(allowedTransitionsJSON)
    for _, transition in ipairs(allowedTransitionsTable) do
        allowedTransitions[transition] = true
    end
end

-- Parse terminal steps
local terminalSteps = {}
if terminalStepsJSON ~= '' and terminalStepsJSON ~= '[]' then
    local terminalStepsTable = cjson.decode(terminalStepsJSON)
    for _, terminalStep in ipairs(terminalStepsTable) do
        terminalSteps[terminalStep] = true
    end
end

-- ---------------------------------------------------------------------------
-- VALIDATION 1: Invalid Transition
-- ---------------------------------------------------------------------------
if previousStepName ~= '' then
    local isValidTransition = allowedTransitions[stepName]
    
    if not isValidTransition then
        hasCriticalViolation = true
        table.insert(violations, {
            violationType = 'invalid_transition',
            severity = 'critical',
            message = string.format("Invalid transition from '%s' to '%s'", previousStepName, stepName),
            stepName = stepName,
            stepId = stepID,
            fromStep = previousStepName,
            toStep = stepName,
            allowedSteps = cjson.decode(allowedTransitionsJSON),
            violatedAt = timestamp
        })
    end
end

-- ---------------------------------------------------------------------------
-- VALIDATION 2: Multiple Terminal States
-- ---------------------------------------------------------------------------
if isTerminalStep == 'true' then
    local terminalCount = 0
    local existingTerminals = {}
    
    for i = 1, #currentSteps, 2 do
        local existingStepKey = currentSteps[i]
        local existingStepName = string.match(existingStepKey, '([^:]+):')
        
        if terminalSteps[existingStepName] then
            terminalCount = terminalCount + 1
            table.insert(existingTerminals, existingStepName)
        end
    end
    
    if terminalCount > 0 and allowMultipleTerminals ~= 'true' then
        hasCriticalViolation = true
        table.insert(violations, {
            violationType = 'multiple_terminal_states',
            severity = 'critical',
            message = string.format("Thread reached multiple terminal states (already at terminal, attempting '%s')", stepName),
            stepName = stepName,
            stepId = stepID,
            currentTerminalSteps = existingTerminals,
            newTerminalStep = stepName,
            violatedAt = timestamp
        })
    end
end

-- ---------------------------------------------------------------------------
-- VALIDATION 3: Retry Limit Exceeded
-- ---------------------------------------------------------------------------
local retryLimitViolated = false
if isRetry and maxRetries > 0 then
    local nextRetryCount = currentRetryCount + 1
    
    if nextRetryCount > maxRetries then
        retryLimitViolated = true
        hasCriticalViolation = true
        
        table.insert(violations, {
            violationType = 'retry_limit_exceeded',
            severity = 'critical',
            message = string.format("Step '%s' exceeded retry limit of %d (current: %d retries)", stepName, maxRetries, nextRetryCount),
            stepName = stepName,
            stepId = stepID,
            retryCount = nextRetryCount,
            maxRetries = maxRetries,
            violatedAt = timestamp
        })
    end
end

-- ---------------------------------------------------------------------------
-- Merge with existing violations from Go
-- ---------------------------------------------------------------------------
local allViolations = violations
if existingViolationsJSON ~= '' and existingViolationsJSON ~= '[]' then
    local existingViolations = cjson.decode(existingViolationsJSON)
    for _, v in ipairs(existingViolations) do
        table.insert(allViolations, v)
    end
end

-- Override status if critical violations found
if hasCriticalViolation then
    status = 'violated'
end

-- ============================================================================
-- SECTION 3: UPDATE STATE
-- ============================================================================

-- Update/create step state hash
if isRetry then
    redis.call('HINCRBY', stepHashKey, 'retryCount', 1)
    redis.call('HSET', stepHashKey,
        'status', status,
        'lastUpdatedAt', timestamp,
        'latestStepID', stepID
    )
else
    redis.call('HSET', stepHashKey,
        'status', status,
        'retryCount', '0',
        'firstSeenAt', timestamp,
        'lastUpdatedAt', timestamp,
        'latestStepID', stepID,
        'previousStep', previousStepKey
    )
end

-- Update current_steps sorted set (only if completed and no critical violations)
if status == 'completed' and not hasCriticalViolation then
    if previousStepKey ~= '' then
        redis.call('ZREM', currentStepsKey, previousStepKey)
    end
    
    local score = redis.call('TIME')[1]
    redis.call('ZADD', currentStepsKey, score, stepKey)
end

-- Store violations if any
if #allViolations > 0 then
    local violationsJSON = cjson.encode(allViolations)
    redis.call('HSET', violationsKey, stepID, violationsJSON)
end

-- Update thread status if terminal step and completed
if isTerminalStep == 'true' and status == 'completed' and not hasCriticalViolation then
    redis.call('HSET', metaKey,
        'status', 'completed',
        'completedAt', timestamp
    )
end

-- ============================================================================
-- SECTION 4: WRITE TO ACTIVITY LOG STREAM
-- ============================================================================

local threadID = string.match(metaKey, 'thread:([^:]+):meta')
local idempKey = string.match(stepKey, '[^:]+:(.+)')

local finalRetryCount = redis.call('HGET', stepHashKey, 'retryCount') or '0'
local firstSeenAt = redis.call('HGET', stepHashKey, 'firstSeenAt') or timestamp

local partition = tonumber(string.sub(threadID, 1, 8), 16) % 10

local violationsForStream = ''
if #allViolations > 0 then
    violationsForStream = cjson.encode(allViolations)
end

redis.call('XADD', 'streams:activity_log:' .. partition, '*',
    'thread_id', threadID,
    'type', 'step_state_changed',
    'step_id', 'steps:' .. stepKey,
    'actor', 'threadify-validator',
    'actor_service', 'business-validation-service',
    'status', status,
    'retry_count', finalRetryCount,
    'first_seen_at', firstSeenAt,
    'last_updated_at', timestamp,
    'previous_step', previousStepKey,
    'violations', violationsForStream,
    'timestamp', timestamp
)

-- ============================================================================
-- SECTION 5: RETURN JSON RESULT
-- ============================================================================

local result = {
    status = status,
    violations = allViolations,
    retryCount = tonumber(finalRetryCount),
    hasCriticalViolation = hasCriticalViolation
}

return cjson.encode(result)
