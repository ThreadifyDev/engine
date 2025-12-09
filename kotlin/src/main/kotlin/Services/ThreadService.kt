package dev.threadify.Services

import dev.threadify.DAOs.ConnectedClientDAO
import dev.threadify.Queues.ClientQueue
import dev.threadify.Schemas.Models.*
import io.ktor.websocket.*
import io.ktor.server.websocket.*
import kotlinx.serialization.json.*
import kotlinx.serialization.encodeToString
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.SerializationException
import java.time.Instant
import java.util.UUID

/**
 * Service for handling WebSocket connections and thread operations.
 */
object ThreadService {
    
    private val json = Json { 
        ignoreUnknownKeys = true
        isLenient = true
        coerceInputValues = true
    }
    private val clientQueue = ClientQueue.create()
    
    /**
     * Handle WebSocket connection lifecycle.
     */
    suspend fun DefaultWebSocketServerSession.handleWebSocketConnection() {
        var currentOwnerId: String? = null
        
        try {
            for (frame in incoming) {
                if (frame is Frame.Text) {
                    val text = frame.readText()
                    val response = handleMessage(text, currentOwnerId)
                    
                    // Update currentOwnerId if connect was successful
                    if (response.contains("\"action\":\"connect\"") && response.contains("\"status\":\"success\"")) {
                        val messageData = json.decodeFromString<Map<String, Any>>(text)
                        currentOwnerId = messageData["ownerId"] as? String
                    }
                    
                    outgoing.send(Frame.Text(response))
                }
            }
        } catch (e: Exception) {
            outgoing.send(Frame.Text(json.encodeToString(mapOf(
                "action" to "error",
                "message" to (e.message ?: "Unknown error")
            ))))
        } finally {
            currentOwnerId?.let { ownerId ->
                closeConnection(ownerId)
            }
        }
    }
    
    /**
     * Route incoming WebSocket messages to appropriate handlers.
     */
    private suspend fun handleMessage(message: String, currentOwnerId: String?): String {
        return try {
            // Parse as generic JSON first to get action
            val jsonElement = Json.parseToJsonElement(message)
            val jsonObject = jsonElement.jsonObject
            val action = jsonObject["action"]?.jsonPrimitive?.content
            
            when (action) {
                "connect" -> {
                    val request = json.decodeFromString<ConnectRequest>(message)
                    val response = handleConnect(request)
                    json.encodeToString(response)
                }
                "startThread" -> {
                    val request = json.decodeFromString<StartThreadRequest>(message)
                    val response = handleStartThread(request, currentOwnerId)
                    json.encodeToString(response)
                }
                "recordThreadEvent" -> {
                    val request = json.decodeFromString<RecordThreadEventRequest>(message)
                    val response = handleRecordThreadEvent(request, currentOwnerId)
                    json.encodeToString(response)
                }
                "closeConnection" -> {
                    val response = handleCloseConnection(currentOwnerId)
                    json.encodeToString(response)
                }
                else -> {
                    val response = ErrorResponse(
                        message = "Unknown action: $action",
                        details = "Valid actions: connect, startThread, recordThreadEvent, closeConnection"
                    )
                    json.encodeToString(response)
                }
            }
        } catch (e: SerializationException) {
            val response = ErrorResponse(
                message = "Failed to parse message",
                details = e.message
            )
            json.encodeToString(response)
        } catch (e: Exception) {
            val response = ErrorResponse(
                message = "Internal error",
                details = e.message
            )
            json.encodeToString(response)
        }
    }
    
    /**
     * Handle connect action - authenticate and register client.
     * @param request ConnectRequest with apiKey, ownerId, and subscribedEvents
     */
    private suspend fun handleConnect(request: ConnectRequest): ConnectResponse {
        // Validate API key presence
        if (request.apiKey.isBlank()) {
            return ConnectResponse(
                status = "error",
                message = "API key is required"
            )
        }
        
        if (request.ownerId.isBlank()) {
            return ConnectResponse(
                status = "error",
                message = "Owner ID is required"
            )
        }
        
        // Create client data
        val clientData = ConnectedClientDAO(
            ownerId = request.ownerId,
            apiKey = request.apiKey,
            connectedAt = Instant.now().toString(),
            subscribedEvents = request.subscribedEvents
        )
        
        // Store in Valkey queue
        clientQueue.addClient(clientData)
        
        return ConnectResponse(
            status = "success",
            message = "Connected successfully",
            ownerId = request.ownerId,
            subscribedEvents = request.subscribedEvents
        )
    }
    
    /**
     * Handle startThread action.
     * @param request StartThreadRequest with optional contractId
     * @param ownerId Current authenticated owner ID
     */
    private suspend fun handleStartThread(request: StartThreadRequest, ownerId: String?): StartThreadResponse {
        if (ownerId == null) {
            return StartThreadResponse(
                status = "error",
                message = "Not authenticated. Please connect first."
            )
        }
        
        val threadId = UUID.randomUUID().toString()
        
        return StartThreadResponse(
            status = "success",
            message = "Thread started successfully",
            threadId = threadId,
            contractId = request.contractId
        )
    }
    
    /**
     * Handle recordThreadEvent action.
     * @param request RecordThreadEventRequest with event details
     * @param ownerId Current authenticated owner ID
     */
    private suspend fun handleRecordThreadEvent(request: RecordThreadEventRequest, ownerId: String?): RecordThreadEventResponse {
        if (ownerId == null) {
            return RecordThreadEventResponse(
                status = "error",
                message = "Not authenticated. Please connect first."
            )
        }
        
        if (request.threadId.isBlank()) {
            return RecordThreadEventResponse(
                status = "error",
                message = "Thread ID is required"
            )
        }
        
        // TODO: Persist event data (startedAt, finishedAt, context, status, metadata)
        // For now, just acknowledge receipt
        
        return RecordThreadEventResponse(
            status = "success",
            message = "Event recorded successfully",
            threadId = request.threadId
        )
    }
    
    /**
     * Handle closeConnection action.
     * @param ownerId Current authenticated owner ID
     */
    private suspend fun handleCloseConnection(ownerId: String?): CloseConnectionResponse {
        if (ownerId != null) {
            closeConnection(ownerId)
        }
        
        return CloseConnectionResponse(
            status = "success",
            message = "Connection closed successfully"
        )
    }
    
    /**
     * Close connection and remove client from Valkey queue.
     * @param ownerId Owner ID to remove
     */
    private suspend fun closeConnection(ownerId: String) {
        clientQueue.removeClient(ownerId)
    }
}
