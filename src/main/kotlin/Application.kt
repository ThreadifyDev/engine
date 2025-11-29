package dev.threadify

import io.ktor.server.application.*

fun main(args: Array<String>) {
    io.ktor.server.netty.EngineMain.main(args)
}

fun Application.module() {
    loadServices()  // Must be first - initializes Authentication service
    configureMonitoring()
    configureAdministration()
    configureSockets()
    configureSerialization()
    configureDatabases()
    configureAuthentication()  // Sets up JWT bearer token middleware
    configureRateLimiting()
    configureHTTP()
    configureRouting()
}
