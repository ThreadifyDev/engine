package dev.threadify.DAOs

import kotlinx.serialization.Serializable

/**
 * Data Access Object for Connected WebSocket Client.
 * Stores client connection details in Valkey queue.
 */
@Serializable
data class ConnectedClientDAO(
    val ownerId: String,
    val apiKey: String,
    val connectedAt: String,
    val subscribedEvents: List<String>, // onSuccess, onError, onViolation, onStepProgress
    val metadata: Map<String, String>? = null
)
