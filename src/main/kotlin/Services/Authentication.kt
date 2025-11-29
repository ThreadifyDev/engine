package dev.threadify.Services

import io.jsonwebtoken.Claims
import io.jsonwebtoken.Jwts
import io.jsonwebtoken.security.Keys
import java.util.*
import javax.crypto.SecretKey

/**
 * Authentication service for handling JWT token creation and validation.
 * This service manages all JWT operations including token generation, verification, and claims extraction.
 */
class Authentication(
    private val secret: String,
    private val issuer: String,
    private val audience: String,
    private val expirationMs: Long
) {
    // Generate a secure key from the secret string for HMAC-SHA256 signing
    private val secretKey: SecretKey = Keys.hmacShaKeyFor(secret.toByteArray())

    /**
     * Creates a JWT token for a given user ID with custom claims.
     * 
     * @param userId The unique identifier for the user
     * @param claims Additional custom claims to include in the token (e.g., roles, permissions)
     * @return A signed JWT token string
     */
    fun createToken(userId: String, claims: Map<String, Any> = emptyMap()): String {
        val now = Date()
        val expiryDate = Date(now.time + expirationMs)

        var builder = Jwts.builder()
            .subject(userId)  // Subject claim - the user ID
            .issuer(issuer)   // Issuer claim - who created the token
            .audience().add(audience).and()  // Audience claim - who the token is intended for
            .issuedAt(now)    // Issued at claim - when token was created
            .expiration(expiryDate)  // Expiration claim - when token expires

        // Add custom claims (e.g., user roles, permissions, metadata)
        claims.forEach { (key, value) ->
            builder = builder.claim(key, value)
        }

        return builder.signWith(secretKey).compact()  // Sign with HMAC-SHA256 (algorithm auto-detected)
    }

    /**
     * Verifies and parses a JWT token, extracting all claims.
     * 
     * @param token The JWT token string to verify
     * @return Claims object containing all token data, or null if invalid
     */
    fun verifyToken(token: String): Claims? {
        return try {
            Jwts.parser()
                .verifyWith(secretKey)  // Use the same key for verification
                .requireIssuer(issuer)     // Verify issuer matches
                .requireAudience(audience) // Verify audience matches
                .build()
                .parseSignedClaims(token)     // Parse and verify signature
                .payload                       // Extract claims
        } catch (e: Exception) {
            // Token is invalid, expired, or tampered with
            null
        }
    }

    /**
     * Extracts the user ID (subject) from a JWT token.
     * 
     * @param token The JWT token string
     * @return The user ID, or null if token is invalid
     */
    fun getUserIdFromToken(token: String): String? {
        return verifyToken(token)?.subject
    }

    /**
     * Checks if a token is valid and not expired.
     * 
     * @param token The JWT token string
     * @return true if token is valid, false otherwise
     */
    fun isTokenValid(token: String): Boolean {
        return verifyToken(token) != null
    }
}
