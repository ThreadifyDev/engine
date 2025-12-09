package dev.threadify.Schemas.DTOs

import dev.threadify.Schemas.Models.ValidationError

data class ValidationResult(
    val isValid: Boolean,
    val errors: List<ValidationError>
) {
    fun returnErrors(): List<ValidationError>? {
        if (errors.isEmpty()) {
            return null
        } else {
            return errors
        }
    }
}