package dev.threadify

import io.ktor.http.*
import io.ktor.server.application.*
import io.ktor.server.plugins.cors.routing.*
import io.ktor.server.response.*
import io.ktor.server.routing.*
import io.ktor.server.sse.*
import io.ktor.sse.*
import io.ktor.websocket.*
import io.micrometer.prometheus.*
import java.sql.Connection
import java.sql.DriverManager
import java.time.Duration
import java.util.concurrent.TimeUnit
import kotlin.time.Duration.Companion.seconds
import org.jetbrains.exposed.sql.SchemaUtils
import org.jetbrains.exposed.sql.transactions.transaction
import org.slf4j.event.*
import dev.threadify.Routers.*

fun Application.configureRouting() {
    install(SSE)
    routing {
        route("/v1") {
            get("/") {
                call.respondText("Hello World!")
            }
            contracts()
        }
        // sse("/hello") {
        //     send(ServerSentEvent("world"))
        // }
    }
}
