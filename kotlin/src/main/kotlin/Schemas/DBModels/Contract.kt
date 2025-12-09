package dev.threadify.Schemas.DBModels

import dev.threadify.DAOs.ContractDAO
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.JsonObject
import org.jetbrains.exposed.dao.id.EntityID
import org.jetbrains.exposed.dao.Entity
import org.jetbrains.exposed.dao.EntityClass
import org.jetbrains.exposed.dao.id.UUIDTable
import org.jetbrains.exposed.sql.javatime.timestamp
import java.time.Instant
import java.util.UUID

/**
 * Exposed Table definition for contracts
 */
object Contracts : UUIDTable("contracts") {
    val name = varchar("name", 255)
    val description = text("description")
    val contentHash = varchar("content_hash", 64).nullable()
    val latestVersion = integer("latest_version").default(1)
    val ownerId = varchar("owner_id", 255)
    val isPublic = bool("is_public").default(false)
    val isDeleted = bool("is_deleted").default(false)
    val createdAt = timestamp("created_at").default(Instant.now())
    val updatedAt = timestamp("updated_at").default(Instant.now())

    init {
      // Unique constraint: one contract name per owner id
      uniqueIndex("unique_contract_name", name, ownerId)
    }
}

/**
 * Exposed DAO Entity for Contract
 */
class Contract(id: EntityID<UUID>) : Entity<UUID>(id) {
    companion object : EntityClass<UUID, Contract>(Contracts)
    
    var name by Contracts.name
    var description by Contracts.description
    var contentHash by Contracts.contentHash
    var latestVersion by Contracts.latestVersion
    var ownerId by Contracts.ownerId
    var isPublic by Contracts.isPublic
    var isDeleted by Contracts.isDeleted
    var createdAt by Contracts.createdAt
    var updatedAt by Contracts.updatedAt
    
    // Relationship: one contract has many versions
    val versions by ContractVersion referrersOn ContractVersions.contractId
}

fun Contract.toDAO(includes: List<String> = emptyList()): ContractDAO {
 // Eagerly access all properties within the transaction
 val idValue = id.value.toString()
 val nameValue = name
 val descriptionValue = description
 val latestVersionValue = latestVersion
 val ownerIdValue = if (includes.contains("ownerId")) ownerId else null
 val isPublicValue = isPublic
 val isDeletedValue = isDeleted
 val contentHashValue = if (includes.contains("contentHash")) contentHash else null
 val createdAtValue = if (includes.contains("createdAt")) createdAt.toString() else null
 val updatedAtValue = updatedAt.toString()
 
 return ContractDAO(
     id = idValue,
     name = nameValue,
     description = descriptionValue,
     contentHash = contentHashValue,
     latestVersion = latestVersionValue,
     ownerId = ownerIdValue,
     isPublic = isPublicValue,
     isDeleted = isDeletedValue,
     createdAt = createdAtValue,
     updatedAt = updatedAtValue
 )
}