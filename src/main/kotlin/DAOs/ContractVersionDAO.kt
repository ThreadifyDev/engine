package dev.threadify.DAOs

import dev.threadify.Schemas.DBModels.ContractVersion
import kotlinx.serialization.Serializable

/**
 * Data Access Object for ContractVersion entity.
 * Used to transfer ContractVersion data between layers without exposing Exposed DAO internals.
 */
@Serializable
data class ContractVersionDAO(
    val id: String,
    val version: Int,
    val content: String,
    val contentHash: String?,
    val contractId: String?,
    val createdBy: String,
    val isDeleted: Boolean,
    val createdAt: String?,
    val updatedAt: String
)

/**
 * Extension function to convert ContractVersion entity to ContractVersionDAO.
 * MUST be called within a transaction block.
 */
fun ContractVersion.toDAO(includes: List<String> = emptyList()): ContractVersionDAO {
    // Eagerly access all properties within the transaction
    val idValue = id.value.toString()
    val versionValue = version
    val contentValue = content
    val contentHashValue = if (includes.contains("contentHash")) contentHash else null
    val contractIdValue = if (includes.contains("contractId")) contract.id.value.toString() else null
    val createdByValue = createdBy
    val isDeletedValue = isDeleted
    val createdAtValue = if (includes.contains("createdAt")) createdAt.toString() else null
    val updatedAtValue = updatedAt.toString()
    
    return ContractVersionDAO(
        id = idValue,
        version = versionValue,
        content = contentValue,
        contentHash = contentHashValue,
        contractId = contractIdValue,
        createdBy = createdByValue,
        isDeleted = isDeletedValue,
        createdAt = createdAtValue,
        updatedAt = updatedAtValue
    )
}
