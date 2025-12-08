package dev.threadify.Schemas.Models

import kotlinx.serialization.Serializable

/**
 * Base WebSocket message with action type.
 */
@Serializable
sealed class WebSocketMessage {
    abstract val action: String
}

// ========== REQUEST MODELS ==========

/**
 * Connect request to authenticate and subscribe to events.
 */
@Serializable
data class ConnectRequest(
    override val action: String = "connect",
    val apiKey: String,
    val ownerId: String,
    val subscribedEvents: List<String> = emptyList() // onSuccess, onError, onViolation, onStepProgress
) : WebSocketMessage()

/**
 * Start thread request.
 */
@Serializable
data class StartThreadRequest(
    override val action: String = "startThread",
    val contractId: String? = null,
    val metadata: Map<String, String>? = null
) : WebSocketMessage()

/**
 * Record thread event request.
 */
@Serializable
data class RecordThreadEventRequest(
    override val action: String = "recordThreadEvent",
    val threadId: String,
    val startedAt: String? = null,
    val finishedAt: String? = null,
    val context: Map<String, String>? = null,
    val status: String? = null,
    val metadata: Map<String, String>? = null
) : WebSocketMessage()

/**
 * Close connection request.
 */
@Serializable
data class CloseConnectionRequest(
    override val action: String = "closeConnection"
) : WebSocketMessage()

// ========== RESPONSE MODELS ==========

/**
 * Base response with status.
 */
@Serializable
sealed class WebSocketResponse {
    abstract val action: String
    abstract val status: String
    abstract val message: String
}

/**
 * Connect response.
 */
@Serializable
data class ConnectResponse(
    override val action: String = "connect",
    override val status: String,
    override val message: String,
    val ownerId: String? = null,
    val subscribedEvents: List<String>? = null
) : WebSocketResponse()

/**
 * Start thread response.
 */
@Serializable
data class StartThreadResponse(
    override val action: String = "startThread",
    override val status: String,
    override val message: String,
    val threadId: String? = null,
    val contractId: String? = null
) : WebSocketResponse()

/**
 * Record thread event response.
 */
@Serializable
data class RecordThreadEventResponse(
    override val action: String = "recordThreadEvent",
    override val status: String,
    override val message: String,
    val threadId: String? = null
) : WebSocketResponse()

/**
 * Close connection response.
 */
@Serializable
data class CloseConnectionResponse(
    override val action: String = "closeConnection",
    override val status: String,
    override val message: String
) : WebSocketResponse()

/**
 * Generic error response.
 */
@Serializable
data class ErrorResponse(
    override val action: String = "error",
    override val status: String = "error",
    override val message: String,
    val details: String? = null
) : WebSocketResponse()
