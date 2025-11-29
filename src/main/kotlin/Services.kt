package dev.threadify

import io.ktor.server.application.*
import dev.threadify.Services.Authentication
import dev.threadify.Utilities.GlobalServices

/**
 * Initializes and loads all application services.
 * This function reads configuration from application.yaml and creates service instances.
 * 
 * Technical details:
 * - Reads JWT configuration (secret, issuer, audience, expiration) from application.yaml
 * - Creates Authentication service instance with these parameters
 * - Stores service in GlobalServices singleton for access throughout the application
 * - Must be called before configureAuthentication() to ensure service is available
 */
fun Application.loadServices() {
    // Read JWT configuration from application.yaml
    val jwtSecret = environment.config.property("jwt.secret").getString()
    val jwtIssuer = environment.config.property("jwt.issuer").getString()
    val jwtAudience = environment.config.property("jwt.audience").getString()
    val jwtExpirationMs = environment.config.property("jwt.expirationMs").getString().toLong()
    
    // Create Authentication service with JWT configuration
    val authenticationService = Authentication(
        secret = jwtSecret,
        issuer = jwtIssuer,
        audience = jwtAudience,
        expirationMs = jwtExpirationMs
    )
    
    // Store in GlobalServices for access in middleware and routes
    GlobalServices.authenticationService = authenticationService
    
    log.info("Services loaded successfully")
}