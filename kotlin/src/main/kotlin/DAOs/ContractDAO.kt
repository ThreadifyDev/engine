package dev.threadify.DAOs

import dev.threadify.Schemas.DBModels.Contract
import kotlinx.serialization.Serializable

/**
 * Data Access Object for Contract entity.
 * Used to transfer Contract data between layers without exposing Exposed DAO internals.
 */
@Serializable
data class ContractDAO(
    val id: String,
    val name: String,
    val description: String,
    val contentHash: String?,
    val latestVersion: Int,
    val ownerId: String?,
    val isPublic: Boolean,
    val isDeleted: Boolean,
    val createdAt: String?,
    val updatedAt: String
)

