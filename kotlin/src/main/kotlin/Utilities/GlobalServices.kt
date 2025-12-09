package dev.threadify.Utilities

import io.lettuce.core.api.StatefulRedisConnection
import dev.threadify.Services.Authentication
import org.jetbrains.exposed.sql.Database

/**
 * Global singleton for storing application-wide services.
 * This provides centralized access to services like Authentication throughout the app.
 * 
 * Technical details:
 * - Uses Kotlin object (singleton pattern) for thread-safe global access
 * - lateinit allows initialization after object creation (in loadServices())
 * - Services are accessed directly via property: GlobalServices.authenticationService
 */
object GlobalServices {
    lateinit var authenticationService: Authentication
    lateinit var queueServer: StatefulRedisConnection<String, String>
    lateinit var persistedServer: Database
}