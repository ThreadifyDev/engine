package dev.threadify.Schemas.Response

import dev.threadify.DAOs.ContractDAO
import dev.threadify.DAOs.ContractVersionDAO
import kotlinx.serialization.Serializable

@Serializable
data class ContractWithOwnershipResponse(
    val contract: ContractDAO,
    val contractVersion: ContractVersionDAO,
    val isOwner: Boolean
)
