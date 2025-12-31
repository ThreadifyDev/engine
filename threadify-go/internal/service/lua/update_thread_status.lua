-- update_thread_status.lua
-- Atomically updates thread status with priority-based logic
-- Ensures failed status takes precedence over completed

-- KEYS[1]: thread hash key (e.g., "thread:123")
-- KEYS[2]: violations hash key (e.g., "thread:123:violations")
-- KEYS[3]: step hash key (e.g., "thread:123:steps:step-456")

-- ARGV[1]: new status ("failed" or "completed")
-- ARGV[2]: step ID
-- ARGV[3]: violation data (JSON string, empty if no violation)
-- ARGV[4]: timestamp (RFC3339)
-- ARGV[5]: step status ("failed" or "completed")

local threadKey = KEYS[1]
local violationKey = KEYS[2]
local stepKey = KEYS[3]

local newStatus = ARGV[1]
local stepID = ARGV[2]
local violationData = ARGV[3]
local timestamp = ARGV[4]
local stepStatus = ARGV[5]

-- Get current thread status
local currentStatus = redis.call('HGET', threadKey, 'status')
if not currentStatus then
    currentStatus = 'active'
end

-- Status priority: failed (3) > completed (2) > active (1)
local statusPriority = {
    failed = 3,
    completed = 2,
    active = 1
}

local currentPriority = statusPriority[currentStatus] or 1
local newPriority = statusPriority[newStatus] or 1

-- Only update thread status if new status has higher or equal priority
if newPriority >= currentPriority then
    redis.call('HSET', threadKey, 'status', newStatus)
    
    -- Set completion time if status is completed
    if newStatus == 'completed' then
        redis.call('HSET', threadKey, 'completedAt', timestamp)
    end
end

-- Always store violation if provided (even if status doesn't change)
if violationData ~= '' then
    redis.call('HSET', violationKey, stepID, violationData)
end

-- Update step status only if stepID is provided
if stepID ~= '' then
    redis.call('HSET', stepKey, 'status', stepStatus, 'updatedAt', timestamp)
end

-- Return final thread status
return redis.call('HGET', threadKey, 'status')
