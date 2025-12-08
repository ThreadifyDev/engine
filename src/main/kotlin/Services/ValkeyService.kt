package dev.threadify.Services

import dev.threadify.Utilities.GlobalServices
import io.lettuce.core.api.StatefulRedisConnection
import io.lettuce.core.ScanArgs

/**
 * Low-level Valkey service for generic key-value operations.
 * Provides basic Redis operations with pipeline and Lua script support.
 */
class ValkeyService(private val connection: StatefulRedisConnection<String, String>) {
    
    companion object {
        fun create(): ValkeyService {
            return ValkeyService(GlobalServices.queueServer)
        }
    }
    
    /**
     * Set a key-value pair.
     * @param key The key
     * @param value The value
     */
    suspend fun set(key: String, value: String) {
        connection.sync().set(key, value)
    }
    
    /**
     * Get value by key.
     * @param key The key
     * @return Value or null if not found
     */
    suspend fun get(key: String): String? {
        return connection.sync().get(key)
    }
    
    /**
     * Delete a key.
     * @param key The key
     */
    suspend fun delete(key: String) {
        connection.sync().del(key)
    }
    
    /**
     * Check if a key exists.
     * @param key The key
     * @return true if key exists
     */
    suspend fun exists(key: String): Boolean {
        return connection.sync().exists(key) > 0
    }
    
    /**
     * Set multiple key-value pairs using pipeline for better performance.
     * @param keyValuePairs Map of key to value
     */
    suspend fun setBatch(keyValuePairs: Map<String, String>) {
        val async = connection.async()
        async.setAutoFlushCommands(false)
        keyValuePairs.forEach { (key, value) ->
            async.set(key, value)
        }
        async.flushCommands()
        async.setAutoFlushCommands(true)
    }
    
    /**
     * Get keys matching a pattern.
     * @param pattern The pattern (e.g., "prefix:*")
     * @return List of matching keys
     */
    suspend fun getKeysByPattern(pattern: String): List<String> {
        val keys = mutableListOf<String>()
        val scanArgs = ScanArgs.Builder.matches(pattern)
        var cursor = connection.sync().scan(scanArgs)
        
        keys.addAll(cursor.keys)
        while (!cursor.isFinished) {
            cursor = connection.sync().scan(cursor, scanArgs)
            keys.addAll(cursor.keys)
        }
        
        return keys
    }
    
    /**
     * Execute a Lua script for atomic operations.
     * @param script The Lua script
     * @param keys List of keys to pass to the script (KEYS array in Lua)
     * @param values List of values to pass to the script (ARGV array in Lua)
     * @return Script result
     */
    suspend fun <T> evalScript(script: String, keys: List<String>, values: List<String>): T? {
        @Suppress("UNCHECKED_CAST")
        return connection.sync().eval(script, io.lettuce.core.ScriptOutputType.VALUE, keys.toTypedArray(), *values.toTypedArray()) as? T
    }
    
    /**
     * Execute a Lua script and return String result.
     * @param script The Lua script
     * @param keys List of keys to pass to the script
     * @param values List of values to pass to the script
     * @return Script result as String
     */
    suspend fun evalScriptString(script: String, keys: List<String>, values: List<String>): String? {
        return evalScript<String>(script, keys, values)
    }
    
    /**
     * Execute a Lua script and return Long result.
     * @param script The Lua script
     * @param keys List of keys to pass to the script
     * @param values List of values to pass to the script
     * @return Script result as Long
     */
    suspend fun evalScriptLong(script: String, keys: List<String>, values: List<String>): Long? {
        return evalScript<Long>(script, keys, values)
    }
    
    /**
     * Set a value only if the key doesn't exist (atomic operation).
     * @param key The key
     * @param value The value
     * @return true if set, false if key already exists
     */
    suspend fun setIfNotExists(key: String, value: String): Boolean {
        val result = evalScriptLong(LuaScripts.SET_IF_NOT_EXISTS, listOf(key), listOf(value))
        return result == 1L
    }
    
    /**
     * Get and delete a key atomically.
     * @param key The key
     * @return The value before deletion, or null if key doesn't exist
     */
    suspend fun getAndDelete(key: String): String? {
        return evalScriptString(LuaScripts.GET_AND_DELETE, listOf(key), emptyList())
    }
    
    /**
     * Upsert operation - update if exists, create if not (atomic).
     * @param key The key
     * @param value The value
     * @return "updated" or "created"
     */
    suspend fun upsert(key: String, value: String): String? {
        return evalScriptString(LuaScripts.UPSERT, listOf(key), listOf(value))
    }
    
    /**
     * Compare and swap operation (atomic).
     * @param key The key
     * @param expectedValue The expected current value
     * @param newValue The new value to set
     * @return true if updated, false if value mismatch or key doesn't exist
     */
    suspend fun compareAndSwap(key: String, expectedValue: String, newValue: String): Boolean {
        val result = evalScriptLong(LuaScripts.COMPARE_AND_SWAP, listOf(key), listOf(expectedValue, newValue))
        return result == 1L
    }
}
