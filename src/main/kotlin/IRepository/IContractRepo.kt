package dev.threadify.IRepository

import dev.threadify.Schemas.Models.Contract as ContractYaml
import dev.threadify.Schemas.DBModels.Contract
import dev.threadify.Schemas.DBModels.ContractVersion
import dev.threadify.DAOs.ContractDAO
import dev.threadify.DAOs.ContractVersionDAO
import java.util.UUID

interface IContractRepo {
    fun findContractById(contractId: UUID): Pair<Contract, ContractDAO>?
    fun findContractByIdAndOwner(contractId: UUID, ownerId: String): Pair<Contract, ContractDAO>?
    fun findContractByName(name: String): Contract?
    fun createContract(name: String, description: String, latestContentHash: String, ownerId: String): Pair<Contract, ContractDAO>
    fun updateContract(contract: Contract): Contract
    fun updateContractFields(contract: Contract, updates: Map<String, Any>): Pair<Contract, ContractDAO>?
    fun createContractVersion(contractId: UUID, version: Int, content: String, createdBy: String, contentHash: String): Pair<ContractVersion, ContractVersionDAO>
    fun getLatestVersion(contractId: UUID): Pair<ContractVersion, ContractVersionDAO>?
}