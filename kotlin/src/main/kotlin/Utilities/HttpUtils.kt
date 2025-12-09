package dev.threadify.Utilities

import io.ktor.http.*

/**
 * Maps integer status codes to HttpStatusCode objects.
 * Centralizes status code mapping logic for consistency across endpoints.
 * 
 * @param statusCode The integer status code (e.g., 200, 404, 500)
 * @return The corresponding HttpStatusCode object
 */
fun mapToHttpStatusCode(statusCode: Int): HttpStatusCode {
    return when (statusCode) {
        200 -> HttpStatusCode.OK
        201 -> HttpStatusCode.Created
        400 -> HttpStatusCode.BadRequest
        403 -> HttpStatusCode.Forbidden
        404 -> HttpStatusCode.NotFound
        else -> HttpStatusCode.InternalServerError
    }
}
