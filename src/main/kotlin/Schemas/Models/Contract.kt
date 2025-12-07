package dev.threadify.Schemas.Models

@kotlinx.serialization.Serializable
data class Contract(
    val contractName: String,
    var version: Int,
    val description: String,
    val parties: List<String>,
    val steps: List<Step>,
    val groups: List<Group>?,
    val validation: ValidationRules
)

@kotlinx.serialization.Serializable
data class Step(
    val id: String,
    val owner: String,
    val dependsOn: List<String>?,
    val timeout: String?,
    val businessContext: Map<String, String>?
)

@kotlinx.serialization.Serializable
data class Group(
    val id: String,
    val steps: List<String>,
    val rules: GroupRules
)

@kotlinx.serialization.Serializable
data class GroupRules(
    val allMustSucceed: Boolean?,
    val maxCombinedDuration: String?
)

@kotlinx.serialization.Serializable
data class ValidationRules(
    val maxDuration: String
)

@kotlinx.serialization.Serializable
data class ValidationError(val field: String, val message: String)

@kotlinx.serialization.Serializable
data class ContractContent(
    val parties: List<String>,
    val steps: List<Step>,
    val groups: List<Group>?,
    val validation: ValidationRules
)