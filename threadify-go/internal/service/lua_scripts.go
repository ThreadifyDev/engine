package service

// Lua scripts for atomic operations in Valkey
const (
	// Atomic step update script
	// KEYS[1]: step state key
	// KEYS[2]: activity list key
	// ARGV[1]: step data JSON
	// ARGV[2]: activity data JSON
	// ARGV[3]: current timestamp
	AtomicStepUpdateScript = `
		local stepKey = KEYS[1]
		local activityKey = KEYS[2]
		local stepData = ARGV[1]
		local activityData = ARGV[2]
		local timestamp = ARGV[3]
		
		-- Update step state
		redis.call('HSET', stepKey, 'data', stepData, 'updated_at', timestamp)
		redis.call('EXPIRE', stepKey, 604800) -- 7 days TTL
		
		-- Add to activity log
		redis.call('LPUSH', activityKey, activityData)
		redis.call('EXPIRE', activityKey, 604800) -- 7 days TTL
		
		return 'OK'
	`
)
