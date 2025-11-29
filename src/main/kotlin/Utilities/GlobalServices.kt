package dev.threadify.Utilities

import dev.threadify.Services.Authentication

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
}