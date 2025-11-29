package dev.threadify

import io.ktor.serialization.gson.*
import io.ktor.serialization.kotlinx.json.*
import io.ktor.server.application.*
import io.ktor.server.plugins.contentnegotiation.*
import org.slf4j.event.*

fun Application.configureSerialization() {
    install(ContentNegotiation) {
        gson {
            }
    }
}
