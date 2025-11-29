package dev.threadify

import dev.threadify.Utilities.GlobalServices
import io.jsonwebtoken.Claims
import io.ktor.http.*
import io.ktor.server.application.*
import io.ktor.server.auth.*
import io.ktor.server.response.*

/**
 * Custom principal that holds JWT claims.
 * This allows us to access claims directly without re-verifying the token.
 * Stores the entire Claims object so we can extract any claim without re-parsing the JWT.
 */
data class JWTClaimsPrincipal(val claims: Claims)

/**
 * Configures bearer token authentication for the application.
 * This sets up JWT validation middleware using our custom Authentication service.
 * 
 * Technical details:
 * - Uses Ktor's built-in Authentication plugin with Bearer provider
 * - Validates bearer tokens from Authorization header (format: "Bearer <token>")
 * - Delegates token verification to our Authentication service (JJWT library)
 * - Makes user ID available in route handlers via UserIdPrincipal
 * - Returns 401 Unauthorized for invalid/expired tokens
 */
fun Application.configureAuthentication() {
    val jwtRealm = environment.config.property("jwt.realm").getString()

    install(Authentication) {
        // Configure bearer token authentication with name "auth-jwt"
        // This is the primary authentication method for protected routes
        bearer("auth-jwt") {
            realm = jwtRealm
            
            // Authenticate function is called for each request to protected routes
            authenticate { tokenCredential ->
                val token = tokenCredential.token
                val authService = GlobalServices.authenticationService
                
                // Use our custom Authentication service to verify the token
                // This validates signature, expiration, issuer, and audience
                val claims = authService.verifyToken(token)
                
                if (claims != null) {
                    // Token is valid - store claims in principal for efficient access
                    // No need to re-verify token when accessing claims in routes
                    JWTClaimsPrincipal(claims)
                } else {
                    // Token is invalid, expired, or tampered with
                    // Returning null triggers Ktor's default 401 Unauthorized response
                    null
                }
            }
        }
    }
}

/**
 * Extension function to get user ID from principal in route handlers.
 * Usage in routes: val userId = call.getUserId()
 * 
 * @return The user ID from the authenticated token, or null if not authenticated
 */
fun ApplicationCall.getUserId(): String? {
    return principal<JWTClaimsPrincipal>()?.claims?.subject
}

/**
 * Extension function to get all claims from the JWT principal.
 * This is efficient - claims are cached in the principal during authentication.
 * No token re-verification needed.
 * 
 * @return The Claims object, or null if not authenticated
 */
fun ApplicationCall.getClaims(): Claims? {
    return principal<JWTClaimsPrincipal>()?.claims
}

/**
 * Extension function to get custom claim from JWT token.
 * Usage: val role = call.getClaim("role")
 * 
 * This is efficient - retrieves from cached claims in principal.
 * No token re-verification or re-parsing needed.
 * 
 * @param name The name of the claim to extract
 * @return The claim value, or null if not present
 */
fun ApplicationCall.getClaim(name: String): Any? {
    return principal<JWTClaimsPrincipal>()?.claims?.get(name)
}

/**
 * Extension function to get the full token from the Authorization header.
 * Useful for passing the token to other services.
 * 
 * @return The JWT token string, or null if not present
 */
fun ApplicationCall.getToken(): String? {
    val authHeader = request.headers["Authorization"]
    return authHeader?.removePrefix("Bearer ")?.trim()
}
