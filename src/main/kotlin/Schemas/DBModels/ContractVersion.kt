package dev.threadify.Schemas.DBModels

import dev.threadify.DAOs.ContractVersionDAO
import org.jetbrains.exposed.dao.id.EntityID
import org.jetbrains.exposed.dao.Entity
import org.jetbrains.exposed.dao.EntityClass
import org.jetbrains.exposed.dao.id.UUIDTable
import org.jetbrains.exposed.sql.javatime.timestamp
import java.time.Instant
import java.util.UUID
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.JsonObject

/**
 * Exposed Table definition for contract_versions
 */
object ContractVersions : UUIDTable("contract_versions") {
    val version = integer("version")
    val content = text("content")
    val contentHash = varchar("content_hash", 64)
    val contractId = reference("contract_id", Contracts)
    val createdBy = varchar("created_by", 255).default("Martins Joseph")
    val isDeleted = bool("is_deleted").default(false)
    val createdAt = timestamp("created_at").default(Instant.now())
    val updatedAt = timestamp("updated_at").default(Instant.now())
    
    init {
        // Unique constraint: one version number per contract
        uniqueIndex("unique_contract_version", contractId, version)
        // Index for faster lookups by contract_id
        index("idx_contract_versions_contract_id", false, contractId)
    }
}

/**
 * Exposed DAO Entity for ContractVersion
 */
class ContractVersion(id: EntityID<UUID>) : Entity<UUID>(id) {
    companion object : EntityClass<UUID, ContractVersion>(ContractVersions)
    
    var version by ContractVersions.version
    var content by ContractVersions.content
    var contentHash by ContractVersions.contentHash
    var contract by Contract referencedOn ContractVersions.contractId
    var createdBy by ContractVersions.createdBy
    var isDeleted by ContractVersions.isDeleted
    var createdAt by ContractVersions.createdAt
    var updatedAt by ContractVersions.updatedAt

}
