package dev.threadify.Queues

import dev.threadify.Services.ValkeyService
import dev.threadify.DAOs.ConnectedClientDAO
import kotlinx.serialization.encodeToString
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json

/**
 * Queue for managing connected WebSocket clients.
 * Uses Valkey as the underlying storage with owner_id as the key.
 */
class ClientQueue(private val valkeyService: ValkeyService) {
    
    private val json = Json { ignoreUnknownKeys = true }
    
    companion object {
        private const val QUEUE_PREFIX = "connected_clients:"
        
        fun create(): ClientQueue {
            return ClientQueue(ValkeyService.create())
        }
    }
    
    /**
     * Add a connected client to the queue.
     * @param client The ConnectedClientDAO to store
     */
    suspend fun addClient(client: ConnectedClientDAO) {
        val key = "$QUEUE_PREFIX${client.ownerId}"
        val jsonData = json.encodeToString(client)
        valkeyService.set(key, jsonData)
    }
    
    /**
     * Get a connected client by owner ID.
     * @param ownerId The owner ID
     * @return ConnectedClientDAO or null if not found
     */
    suspend fun getClient(ownerId: String): ConnectedClientDAO? {
        val key = "$QUEUE_PREFIX$ownerId"
        val jsonData = valkeyService.get(key) ?: return null
        return json.decodeFromString<ConnectedClientDAO>(jsonData)
    }
    
    /**
     * Remove a client from the queue.
     * @param ownerId The owner ID
     */
    suspend fun removeClient(ownerId: String) {
        val key = "$QUEUE_PREFIX$ownerId"
        valkeyService.delete(key)
    }
    
    /**
     * Check if a client exists in the queue.
     * @param ownerId The owner ID
     * @return true if client exists
     */
    suspend fun clientExists(ownerId: String): Boolean {
        val key = "$QUEUE_PREFIX$ownerId"
        return valkeyService.exists(key)
    }
    
    /**
     * Add multiple clients using pipeline for better performance.
     * @param clients List of ConnectedClientDAO to add
     */
    suspend fun addClientsBatch(clients: List<ConnectedClientDAO>) {
        val keyValuePairs = clients.associate { client ->
            "$QUEUE_PREFIX${client.ownerId}" to json.encodeToString(client)
        }
        valkeyService.setBatch(keyValuePairs)
    }
    
    /**
     * Get all connected clients (use with caution for large datasets).
     * @return List of all connected clients
     */
    suspend fun getAllClients(): List<ConnectedClientDAO> {
        val keys = valkeyService.getKeysByPattern("$QUEUE_PREFIX*")
        return keys.mapNotNull { key ->
            val jsonData = valkeyService.get(key)
            if (jsonData != null) {
                json.decodeFromString<ConnectedClientDAO>(jsonData)
            } else {
                null
            }
        }
    }
}
