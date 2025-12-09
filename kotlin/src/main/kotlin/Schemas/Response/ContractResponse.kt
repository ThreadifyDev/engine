package dev.threadify.Schemas.Response

import dev.threadify.DAOs.ContractDAO
import dev.threadify.DAOs.ContractVersionDAO
import kotlinx.serialization.Serializable

@Serializable
data class ContractResponse(
    val contract: ContractDAO,
    val contractVersion: ContractVersionDAO
)
