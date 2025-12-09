package dev.threadify

import dev.threadify.Services.ThreadService
import io.ktor.server.application.*
import io.ktor.server.routing.*
import io.ktor.server.websocket.*
import kotlin.time.Duration.Companion.seconds

fun Application.configureSockets() {
    install(WebSockets) {
        pingPeriod = 15.seconds
        timeout = 15.seconds
        maxFrameSize = Long.MAX_VALUE
        masking = false
    }
    
    routing {
        // WebSocket endpoint - NO authentication required at connection level
        // Authentication happens via the "connect" message after WebSocket is established
        webSocket("/threads") {
            with(ThreadService) {
                handleWebSocketConnection()
            }
        }
    }
}
