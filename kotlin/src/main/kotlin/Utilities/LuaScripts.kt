package dev.threadify.Services

/**
 * Collection of Lua scripts for atomic Valkey operations.
 * These scripts ensure atomicity and reduce round-trips to the server.
 */
object LuaScripts {
    
    /**
     * Atomically set a value only if the key doesn't exist.
     * Returns 1 if set, 0 if key already exists.
     * 
     * KEYS[1] = key
     * ARGV[1] = value
     */
    const val SET_IF_NOT_EXISTS = """
        if redis.call('exists', KEYS[1]) == 0 then
            redis.call('set', KEYS[1], ARGV[1])
            return 1
        else
            return 0
        end
    """
    
    /**
     * Atomically get and delete a key.
     * Returns the value before deletion or nil if key doesn't exist.
     * 
     * KEYS[1] = key
     */
    const val GET_AND_DELETE = """
        local value = redis.call('get', KEYS[1])
        if value then
            redis.call('del', KEYS[1])
        end
        return value
    """
    
    /**
     * Atomically increment a counter and set expiry if it's a new key.
     * Returns the new counter value.
     * 
     * KEYS[1] = key
     * ARGV[1] = increment amount
     * ARGV[2] = expiry in seconds (optional, only for new keys)
     */
    const val INCREMENT_WITH_EXPIRY = """
        local current = redis.call('incr', KEYS[1])
        if current == 1 and ARGV[2] then
            redis.call('expire', KEYS[1], tonumber(ARGV[2]))
        end
        return current
    """
    
    /**
     * Atomically check if a key exists and update it, or create it if it doesn't.
     * Returns 'updated' or 'created'.
     * 
     * KEYS[1] = key
     * ARGV[1] = new value
     */
    const val UPSERT = """
        local exists = redis.call('exists', KEYS[1])
        redis.call('set', KEYS[1], ARGV[1])
        if exists == 1 then
            return 'updated'
        else
            return 'created'
        end
    """
    
    /**
     * Atomically compare and swap (CAS) operation.
     * Only updates if the current value matches the expected value.
     * Returns 1 if updated, 0 if not (value mismatch or key doesn't exist).
     * 
     * KEYS[1] = key
     * ARGV[1] = expected value
     * ARGV[2] = new value
     */
    const val COMPARE_AND_SWAP = """
        local current = redis.call('get', KEYS[1])
        if current == ARGV[1] then
            redis.call('set', KEYS[1], ARGV[2])
            return 1
        else
            return 0
        end
    """
    
    /**
     * Atomically delete multiple keys matching a pattern.
     * Returns the number of keys deleted.
     * 
     * KEYS[1] = pattern (e.g., "prefix:*")
     */
    const val DELETE_BY_PATTERN = """
        local keys = redis.call('keys', KEYS[1])
        local count = 0
        for i, key in ipairs(keys) do
            redis.call('del', key)
            count = count + 1
        end
        return count
    """
    
    /**
     * Atomically add an item to a set only if it doesn't exceed max size.
     * Returns 1 if added, 0 if set is full, -1 if item already exists.
     * 
     * KEYS[1] = set key
     * ARGV[1] = member to add
     * ARGV[2] = max size
     */
    const val ADD_TO_SET_WITH_LIMIT = """
        local exists = redis.call('sismember', KEYS[1], ARGV[1])
        if exists == 1 then
            return -1
        end
        
        local size = redis.call('scard', KEYS[1])
        if size >= tonumber(ARGV[2]) then
            return 0
        end
        
        redis.call('sadd', KEYS[1], ARGV[1])
        return 1
    """
}
