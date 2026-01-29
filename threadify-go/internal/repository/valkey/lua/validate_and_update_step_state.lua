-- validate_and_update_step_state.lua
-- Atomically validates step transitions, terminal states, retry limits, and updates step state
-- Returns JSON with all validation results

-- Use cjson library for JSON encoding (available as global in Redis/Valkey)
local cjson = cjson

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
-- ARGV[11]: ttl (TTL in seconds from config)
-- ARGV[12]: threadID (passed from Go to avoid regex extraction)
-- ARGV[13]: idempotencyKey (passed from Go to avoid regex extraction)
-- ARGV[14]: actor (user who recorded this step, for .own permission filtering)

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
local ttl = tonumber(ARGV[11]) or 604800  -- Default to 7 days if not provided
local threadID = ARGV[12]  -- Passed from Go to avoid regex extraction
local idempotencyKey = ARGV[13]  -- Passed from Go to avoid regex extraction
local actor = ARGV[14] or ''  -- User who recorded this step (for .own permission filtering)

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

-- Parse transitions from pre-computed URL-encoded string format: "step1:next1,next2|step2:next3"
local transitionsMap = {}
if allowedTransitionsJSON ~= '' then
    for transitionPair in string.gmatch(allowedTransitionsJSON, '([^|]+)') do
        local fromStep, toStepsStr = string.match(transitionPair, '([^:]+):(.*)')
        if fromStep and toStepsStr then
            -- URL decode step names (handle both + and %XX patterns)
            local decodedFromStep = string.gsub(fromStep, '+', ' ')
            decodedFromStep = string.gsub(decodedFromStep, '%%([0-9A-Fa-f][0-9A-Fa-f])', function(hex)
                return string.char(tonumber(hex, 16))
            end)
            
            local allowedNextSteps = {}
            for encodedStep in string.gmatch(toStepsStr, '([^,]+)') do
                -- URL decode each step name
                local decodedStep = string.gsub(encodedStep, '+', ' ')
                decodedStep = string.gsub(decodedStep, '%%([0-9A-Fa-f][0-9A-Fa-f])', function(hex)
                    return string.char(tonumber(hex, 16))
                end)
                table.insert(allowedNextSteps, decodedStep)
            end
            transitionsMap[decodedFromStep] = allowedNextSteps
        end
    end
end

-- Parse terminal steps from pre-computed URL-encoded comma-separated string
local terminalSteps = {}
if terminalStepsJSON ~= '' then
    for encodedStep in string.gmatch(terminalStepsJSON, '([^,]+)') do
        -- URL decode step name (handle both + and %XX patterns)
        local decodedStep = string.gsub(encodedStep, '+', ' ')
        decodedStep = string.gsub(decodedStep, '%%([0-9A-Fa-f][0-9A-Fa-f])', function(hex)
            return string.char(tonumber(hex, 16))
        end)
        terminalSteps[decodedStep] = true
    end
end

-- ---------------------------------------------------------------------------
-- VALIDATION 1: Invalid Transition
-- ---------------------------------------------------------------------------
if previousStepName ~= '' then
    -- Look up allowed transitions FROM the previous step
    local allowedNextSteps = transitionsMap[previousStepName] or {}
    
    -- Check if current step is in the allowed list
    local isValidTransition = false
    for _, allowedStep in ipairs(allowedNextSteps) do
        if allowedStep == stepName then
            isValidTransition = true
            break
        end
    end
    
    if not isValidTransition then
        hasCriticalViolation = true
        -- Convert allowed next steps to comma-separated string
        local allowedStepsStr = table.concat(allowedNextSteps, ',')
        
        -- Build details as simple key-value pairs (no nested arrays/objects)
        local details = {
            stepName = stepName,
            stepId = stepID,
            fromStep = previousStepName,
            toStep = stepName,
            allowedSteps = allowedStepsStr,
            violatedAt = timestamp
        }
        
        table.insert(violations, {
            violationType = 'invalid_transition',
            severity = 'critical',
            message = string.format("Invalid transition from '%s' to '%s'", previousStepName, stepName),
            details = details
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
        -- Convert existingTerminals array to comma-separated string
        local existingTerminalsStr = table.concat(existingTerminals, ',')
        
        local details = {
            stepName = stepName,
            stepId = stepID,
            currentTerminalSteps = existingTerminalsStr,
            newTerminalStep = stepName,
            violatedAt = timestamp
        }
        
        table.insert(violations, {
            violationType = 'multiple_terminal_states',
            severity = 'critical',
            message = string.format("Thread reached multiple terminal states (already at terminal, attempting '%s')", stepName),
            details = details
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
        
        local details = {
            stepName = stepName,
            stepId = stepID,
            retryCount = tostring(nextRetryCount),
            maxRetries = tostring(maxRetries),
            violatedAt = timestamp
        }
        
        table.insert(violations, {
            violationType = 'retry_limit_exceeded',
            severity = 'critical',
            message = string.format("Step '%s' exceeded retry limit of %d (current: %d retries)", stepName, maxRetries, nextRetryCount),
            details = details
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
-- IMPORTANT: If you change these HSET fields, also update the write-back code:
-- See: /internal/repository/valkey/thread_helpers.go (writeBackAllStepsToValkey function)
-- The write-back must use the SAME field names to maintain format consistency!
if isRetry then
    redis.call('HINCRBY', stepHashKey, 'retryCount', 1)
    redis.call('HSET', stepHashKey,
        'status', status,
        'lastUpdatedAt', timestamp,
        'latestStepID', stepID,
        'actor', actor
    )
else
    redis.call('HSET', stepHashKey,
        'status', status,
        'retryCount', '0',
        'firstSeenAt', timestamp,
        'lastUpdatedAt', timestamp,
        'latestStepID', stepID,
        'previousStep', previousStepKey,
        'actor', actor
    )
end

-- Update current_steps sorted set (only if success and no critical violations)
if status == 'success' and not hasCriticalViolation then
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

-- Update thread status if terminal step and success
if isTerminalStep == 'true' and status == 'success' and not hasCriticalViolation then
    redis.call('HSET', metaKey,
        'status', 'completed',
        'completedAt', timestamp
    )
end

-- ============================================================================
-- SECTION 4: WRITE TO ACTIVITY LOG STREAM
-- ============================================================================

-- threadID and idempotencyKey now passed as ARGV[12] and ARGV[13] to avoid regex extraction

local finalRetryCount = redis.call('HGET', stepHashKey, 'retryCount') or '0'
local firstSeenAt = redis.call('HGET', stepHashKey, 'firstSeenAt') or timestamp
local previousStepStored = redis.call('HGET', stepHashKey, 'previousStep') or ''

-- Redis stream writes removed - now using NATS JetStream for archival
-- Activity log events are published to NATS by the Go application layer

-- ============================================================================
-- SECTION 5: EXTEND TTL ON ALL THREAD KEYS
-- ============================================================================
-- Extend TTL on all thread-related keys to prevent partial expiration
-- This ensures all thread data expires together, maintaining consistency
-- KEYS command removed for performance - only extend known keys

-- threadID already passed as ARGV[12] to avoid regex extraction

if threadID then
    -- Core thread keys (these always exist)
    redis.call('EXPIRE', 'thread:' .. threadID, ttl)
    redis.call('EXPIRE', metaKey, ttl)
    redis.call('EXPIRE', currentStepsKey, ttl)
    redis.call('EXPIRE', violationsKey, ttl)
    redis.call('EXPIRE', stepHashKey, ttl)
    
    -- Optional keys (may not exist, but EXPIRE is safe)
    redis.call('EXPIRE', 'thread:' .. threadID .. ':access', ttl)
    redis.call('EXPIRE', 'thread:' .. threadID .. ':role_index', ttl)
    redis.call('EXPIRE', 'thread:' .. threadID .. ':activity', ttl)
    
    -- Note: Removed KEYS pattern matching for performance
    -- Step hash TTLs are extended when they are created/updated
end

-- ============================================================================
-- SECTION 6: RETURN JSON RESULT
-- ============================================================================

-- Ensure violations is always encoded as an array, even when empty
if #allViolations == 0 then
    allViolations = cjson.empty_array
end

local result = {
    status = status,
    violations = allViolations,
    retryCount = tonumber(finalRetryCount),
    hasCriticalViolation = hasCriticalViolation,
    firstSeenAt = firstSeenAt,
    previousStep = previousStepStored
}

return cjson.encode(result)
