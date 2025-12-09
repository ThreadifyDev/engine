package dev.threadify.Schemas.Response

import dev.threadify.Schemas.Models.ValidationError
import kotlinx.serialization.Serializable

@Serializable
data class ErrorResponse(
    val message: String,
    val errors: List<ValidationError>? = null
)
