package dev.threadify.Utilities

import org.yaml.snakeyaml.Yaml
import org.yaml.snakeyaml.error.YAMLException
import dev.threadify.Schemas.DTOs.ValidationResult
import dev.threadify.Schemas.Models.*

class ContractValidator {
    
    fun validate(yamlString: String): Pair<Contract?, ValidationResult> {
        val errors = mutableListOf<ValidationError>()
        
        // Parse YAML
        val contract = try {
            parseContract(yamlString)
        } catch (e: YAMLException) {
            return Pair(null, ValidationResult(false, listOf(
                ValidationError("yaml", "Failed to parse YAML: ${e.message}")
            )))
        } catch (e: Exception) {
            return Pair(null, ValidationResult(false, listOf(
                ValidationError("yaml", "Failed to parse YAML: ${e.message}")
            )))
        }
        
        // Validate contract_name format (alphanumeric with underscores)
        if (!contract.contractName.matches(Regex("^[a-zA-Z0-9_]+$"))) {
            errors.add(ValidationError("contract_name", 
                "Must contain only alphanumeric characters and underscores"))
        }
        
        // Validate version is positive
        if (contract.version < 1) {
            errors.add(ValidationError("version", "Must be a positive integer"))
     }
     
     // Validate description is not empty
     if (contract.description.isBlank()) {
         errors.add(ValidationError("description", "Cannot be empty"))
     }
     
     // Get all party IDs
    val partyIds = contract.parties.toSet()
    
    // Get all step IDs
    val stepIds = contract.steps.map { it.id }.toSet()
    
    // Validate no duplicate step IDs
    val duplicateSteps = contract.steps.groupBy { it.id }
        .filter { it.value.size > 1 }
        .keys
    duplicateSteps.forEach { stepId ->
        errors.add(ValidationError("steps.$stepId", 
            "Duplicate step ID found. Each step must have a unique ID"))
    }
     
     // Validate steps
     contract.steps.forEach { step ->
         // Validate owner is a defined party
         if (step.owner !in partyIds) {
             errors.add(ValidationError("steps.${step.id}.owner", 
                 "Owner '${step.owner}' is not a defined party"))
         }
         
         // Validate depends_on references exist
         step.dependsOn?.forEach { depId ->
             if (depId !in stepIds) {
                errors.add(ValidationError("steps.${step.id}.depends_on", 
                 "Referenced step '$depId' does not exist"))
             }
         }
         
         // Validate timeout format if present
         step.timeout?.let { timeout ->
             if (!isValidDuration(timeout)) {
                 errors.add(ValidationError("steps.${step.id}.timeout", 
                 "Invalid duration format. Use: s, ms, us, or m (e.g., '2s', '100ms')"))
             }
         }
         
         // Validate business_context field types
         step.businessContext?.forEach { (field, type) ->
             if (!isValidFieldType(type)) {
                 errors.add(ValidationError("steps.${step.id}.business_context.$field", 
                     "Invalid type '$type'. Must be: string, number, boolean, object, or array"))
             }
         }
     }
     
     // Validate all party members are assigned to at least one step
     val assignedParties = contract.steps.map { it.owner }.toSet()
     partyIds.forEach { partyId ->
         if (partyId !in assignedParties) {
             errors.add(ValidationError("parties.$partyId", 
                 "Party is not assigned to any step"))
         }
     }
     
     // Validate groups if present
     contract.groups?.forEach { group ->
         group.steps.forEach { stepId ->
             if (stepId !in stepIds) {
                 errors.add(ValidationError("groups.${group.id}.steps", 
                     "Referenced step '$stepId' does not exist"))
             }
         }
         
         // Validate max_combined_duration format if present
         group.rules.maxCombinedDuration?.let { duration ->
             if (!isValidDuration(duration)) {
                 errors.add(ValidationError("groups.${group.id}.rules.max_combined_duration", 
                     "Invalid duration format"))
             }
         }
     }
     
     // Validate max_duration in validation section
     if (!isValidDuration(contract.validation.maxDuration)) {
         errors.add(ValidationError("validation.max_duration", 
             "Invalid duration format. Use: s, ms, us, or m (e.g., '10s', '500ms')"))
     }
     
     return Pair(contract, ValidationResult(errors.isEmpty(), errors))
 }
 
    private fun parseContract(yamlString: String): Contract {
        val yaml = Yaml()
        val data = yaml.load<Map<String, Any>>(yamlString)
        
        return Contract(
            contractName = data["contract_name"] as String,
            version = data["version"] as Int,
            description = data["description"] as String,
            parties = parseParties(data["parties"] as List<*>),
            steps = parseSteps(data["steps"] as List<*>),
            groups = (data["groups"] as? List<*>)?.let { parseGroups(it) },
            validation = parseValidationRules(data["validation"] as Map<*, *>)
        )
    }
    
    private fun parseParties(list: List<*>): List<String> {
        return list.map { item ->
            item.toString()
        }
    }
    
    private fun parseSteps(list: List<*>): List<Step> {
        return list.map { item ->
            val map = item as Map<*, *>
            Step(
                id = map["id"] as String,
                owner = map["owner"] as String,
                dependsOn = parseDependsOn(map["depends_on"]),
                timeout = map["timeout"]?.toString(),
                businessContext = (map["business_context"] as? Map<*, *>)?.let { bc ->
                    bc.entries.associate { it.key.toString() to it.value.toString() }
                }
            )
        }
    }
    
    private fun parseDependsOn(value: Any?): List<String>? {
        return when (value) {
            null -> null
            is String -> listOf(value)
            is List<*> -> value.map { it.toString() }
            else -> throw IllegalArgumentException("depends_on must be a string or list of strings")
        }
    }
    
    private fun parseGroups(list: List<*>): List<Group> {
        return list.map { item ->
            val map = item as Map<*, *>
            val rulesMap = map["rules"] as Map<*, *>
            Group(
                id = map["id"] as String,
                steps = (map["steps"] as List<*>).map { it.toString() },
                rules = GroupRules(
                    allMustSucceed = rulesMap["all_must_succeed"] as? Boolean,
                    maxCombinedDuration = rulesMap["max_combined_duration"]?.toString()
                )
            )
        }
    }
    
    private fun parseValidationRules(map: Map<*, *>): ValidationRules {
        return ValidationRules(
            maxDuration = map["max_duration"].toString()
        )
    }
    
    private fun isValidDuration(duration: String): Boolean {
        // Matches patterns like: 2s, 100ms, 50us, 5m
        return duration.matches(Regex("^\\d+(\\.\\d+)?(s|ms|us|m)$"))
    }
    
    private fun isValidFieldType(type: String): Boolean {
        return type in setOf("string", "number", "boolean", "object", "array")
    }
}