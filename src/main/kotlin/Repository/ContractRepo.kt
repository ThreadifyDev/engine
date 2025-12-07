package dev.threadify.Repository

import dev.threadify.IRepository.IContractRepo
import dev.threadify.Schemas.DBModels.Contract
import dev.threadify.Schemas.DBModels.Contracts
import dev.threadify.Schemas.DBModels.ContractVersion
import dev.threadify.Schemas.DBModels.ContractVersions
import dev.threadify.DAOs.ContractDAO
import dev.threadify.DAOs.ContractVersionDAO
import dev.threadify.Schemas.DBModels.toDAO
import dev.threadify.DAOs.toDAO
import org.jetbrains.exposed.sql.transactions.transaction
import org.jetbrains.exposed.sql.and
import java.time.Instant
import java.util.UUID

class ContractRepo : IContractRepo {
    
    override fun findContractById(contractId: UUID): Pair<Contract, ContractDAO>? = transaction {
        val contract = Contract.findById(contractId)
        if (contract == null) {
            return@transaction null
        }
        Pair(contract, contract.toDAO())
    }
    
    override fun findContractByIdAndOwner(contractId: UUID, ownerId: String): Pair<Contract, ContractDAO>? = transaction {
        val contract = Contract.find { 
            (Contracts.id eq contractId) and (Contracts.ownerId eq ownerId)
        }.firstOrNull()
        
        if (contract == null) {
            return@transaction null
        }
        Pair(contract, contract.toDAO())
    }
    
    override fun findContractByName(name: String): Contract? = transaction {
        Contract.find { Contracts.name eq name }.firstOrNull()
    }
    
    override fun createContract(name: String, description: String, latestContentHash: String, ownerId: String): Pair<Contract, ContractDAO> = transaction {
        val contract = Contract.new {
            this.name = name
            this.description = description
            this.latestVersion = 1
            this.contentHash = latestContentHash
            this.isDeleted = false
            this.isPublic = false
            this.createdAt = Instant.now()
            this.updatedAt = Instant.now()
            this.ownerId = ownerId
        }

        Pair(contract, contract.toDAO())
    }
    
    override fun updateContract(contract: Contract): Contract = transaction {
        contract.updatedAt = Instant.now()
        contract
    }
    
    override fun updateContractFields(contract: Contract, updates: Map<String, Any>): Pair<Contract, ContractDAO>? = transaction {
        updates.forEach { (field, value) ->
            when (field) {
                "description" -> contract.description = value as String
                "contentHash" -> contract.contentHash = value as String
                "latestVersion" -> contract.latestVersion = value as Int
                "isDeleted" -> contract.isDeleted = value as Boolean
                "isPublic" -> contract.isPublic = value as Boolean
                "name" -> contract.name = value as String
                // Add more fields as needed
                else -> throw IllegalArgumentException("Unknown field: $field")
            }
        }
        
        contract.updatedAt = Instant.now()
        Pair(contract, contract.toDAO())
    }
    
    override fun createContractVersion(
        contractId: UUID,
        version: Int,
        content: String,
        createdBy: String,
        contentHash: String
    ): Pair<ContractVersion, ContractVersionDAO> = transaction {
        val contractVersion = ContractVersion.new {
            this.version = version
            this.content = content
            this.contentHash = contentHash
            this.contract = Contract[contractId]
            this.isDeleted = false
            this.createdAt = Instant.now()
            this.updatedAt = Instant.now()
            this.createdBy = createdBy
        }
        
        return@transaction Pair(contractVersion, contractVersion.toDAO())
    }
    
    override fun getLatestVersion(contractId: UUID): Pair<ContractVersion, ContractVersionDAO>? = transaction {
        val contractVersion = ContractVersion.find { ContractVersions.contractId eq contractId }
            .orderBy(ContractVersions.version to org.jetbrains.exposed.sql.SortOrder.DESC)
            .firstOrNull()
        
        if (contractVersion == null) {
            return@transaction null
        }
        
        Pair(contractVersion, contractVersion.toDAO())
    }
    
    override fun findContractByIdNotDeleted(contractId: UUID): Pair<Contract, ContractDAO>? = transaction {
        val contract = Contract.find { 
            (Contracts.id eq contractId) and (Contracts.isDeleted eq false)
        }.firstOrNull()
        
        if (contract == null) {
            return@transaction null
        }
        Pair(contract, contract.toDAO(listOf("ownerId")))
    }
    
    override fun findContractByIdAndOwnerNotDeleted(contractId: UUID, ownerId: String): Pair<Contract, ContractDAO>? = transaction {
        val contract = Contract.find { 
            (Contracts.id eq contractId) and (Contracts.ownerId eq ownerId) and (Contracts.isDeleted eq false)
        }.firstOrNull()
        
        if (contract == null) {
            return@transaction null
        }
        Pair(contract, contract.toDAO())
    }
    
    override fun softDeleteContract(contract: Contract): Pair<Contract, ContractDAO> = transaction {
        contract.isDeleted = true
        contract.updatedAt = Instant.now()
        Pair(contract, contract.toDAO())
    }
    
    override fun getLatestVersionNotDeleted(contractId: UUID): Pair<ContractVersion, ContractVersionDAO>? = transaction {
        val contractVersion = ContractVersion.find { 
            (ContractVersions.contractId eq contractId) and (ContractVersions.isDeleted eq false)
        }
            .orderBy(ContractVersions.version to org.jetbrains.exposed.sql.SortOrder.DESC)
            .firstOrNull()
        
        if (contractVersion == null) {
            return@transaction null
        }
        
        Pair(contractVersion, contractVersion.toDAO(listOf("contentHash", "contractId", "createdAt")))
    }
    
    override fun getContractVersion(contractId: UUID, version: Int): Pair<ContractVersion, ContractVersionDAO>? = transaction {
        val contractVersion = ContractVersion.find { 
            (ContractVersions.contractId eq contractId) and (ContractVersions.version eq version)
        }.firstOrNull()
        
        if (contractVersion == null) {
            return@transaction null
        }
        
        Pair(contractVersion, contractVersion.toDAO(listOf("contentHash", "contractId", "createdAt")))
    }
    
    override fun getAllVersionsMetadata(contractId: UUID): List<ContractVersionDAO> = transaction {
        ContractVersion.find { 
            (ContractVersions.contractId eq contractId) and (ContractVersions.isDeleted eq false)
        }
            .orderBy(ContractVersions.version to org.jetbrains.exposed.sql.SortOrder.DESC)
            .map { it.toDAO(listOf("createdAt")) }
    }
    
    override fun softDeleteContractVersion(contractVersion: ContractVersion): Pair<ContractVersion, ContractVersionDAO> = transaction {
        contractVersion.isDeleted = true
        contractVersion.updatedAt = Instant.now()
        Pair(contractVersion, contractVersion.toDAO())
    }
}