package dev.threadify

import io.ktor.server.application.*

fun main(args: Array<String>) {
    io.ktor.server.netty.EngineMain.main(args)
}

fun Application.module() {
    loadServices()  // Must be first - initializes Authentication service
    configureMonitoring()
    configureAdministration()
    configureSerialization()
    configureDatabases()
    configureHTTP()
    configureAuthentication()  // Sets up JWT bearer token middleware
    configureRateLimiting()
    configureSockets()  // WebSocket routes AFTER authentication to ensure they're not wrapped
    configureRouting()
}
