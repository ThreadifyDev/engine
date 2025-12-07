package dev.threadify.Services

import dev.threadify.Schemas.Models.Contract as ContractYaml
import dev.threadify.Schemas.Models.ContractContent
import dev.threadify.Schemas.DBModels.Contract
import dev.threadify.Schemas.DBModels.ContractVersion
import dev.threadify.Repository.ContractRepo
import dev.threadify.Utilities.ContractValidator
import dev.threadify.Schemas.Response.ContractResponse
import dev.threadify.Schemas.Response.ErrorResponse
import dev.threadify.Schemas.DTOs.ValidationResult
import dev.threadify.DAOs.ContractDAO
import dev.threadify.DAOs.ContractVersionDAO
import dev.threadify.Schemas.DBModels.toDAO
import kotlinx.serialization.json.Json
import java.security.MessageDigest
import java.util.UUID

class ContractService() {
    private val contractRepo: ContractRepo = ContractRepo()
    private val json = Json { prettyPrint = false }
    
    /**
     * Validates a contract YAML.
     * 
     * @param contractYaml The contract definition in String
     * @return Pair of Contract(ContractYaml) and ValidationResult
     */
     fun validateContract(contractYaml: String): Pair<ContractYaml?, ValidationResult> {
       val validator = ContractValidator()
       val result = validator.validate(contractYaml)
       return result
     }


    /**
     * Creates a new contract and its first version.
     * 
     * @param contractYaml The contract definition from YAML
     * @return Pair of Contract and ContractVersion
     */
    fun createContract(ownerId: String, createdBy: String, contractYaml: String): Pair<Int, Any> {
      try {
        // Validate contract yaml before creating
        val result = validateContract(contractYaml)
        val contractValidationResult = result.second

        // If contract is invalid, then we need to return an error back to user
        if (!contractValidationResult.isValid) {
          return Pair(400, ErrorResponse(
           message = "Contract is not valid",
           errors = contractValidationResult.errors
          ))
        }

        // Get the contractYaml as an object
        val contractYamlObject = result.first!!

        // Forcefully Set version to 1 for new Contracts created
        contractYamlObject.version = 1

        // Serialize contract as full contract and content only
        val (contentJson, contentOnlyJson) = serializeContract(contractYamlObject)
        
        // Calculate content hash (excluding metadata: name, version, description)
        val contentHash = calculateHash(contentOnlyJson)

        // Create new contract in Persisted Storage
        val (contractEntity, contractDAO) = contractRepo.createContract(
            name = contractYamlObject.contractName,
            description = contractYamlObject.description,
            latestContentHash = contentHash,
            ownerId = ownerId
        )
        
        // Create first version (version 1)
        val (_, contractVersionDAO) = contractRepo.createContractVersion(
            contractId = contractEntity.id.value,
            version = contractEntity.latestVersion,
            content = contentJson,
            contentHash = contentHash,
            createdBy = createdBy
        )
        
        return Pair(200, ContractResponse(
          contract = contractDAO,
          contractVersion = contractVersionDAO
        ))
      } catch (e: Exception) {
        println("E message ${e.message}")
        if (e.message?.contains("unique_contract_name") == true) {
          return Pair(400, mapOf<String, Any>(
            "message" to "Contract with this name already exists",
          ))
        }
         return Pair(500, mapOf<String, Any>(
           "message" to "Internal Server Error",
         ))
      }
    }
    
    /**
     * Updates an existing contract and creates a new version.
     * 
     * @param contractId The UUID of the existing contract
     * @param contractYaml The updated contract definition
     * @return Pair of updated Contract and new ContractVersion
     */
    fun updateContract(contractId: String, ownerId: String, createdBy: String, contractYaml: String): Pair<Int, Any> {
        try {
          // Find existing contract and verify ownership
          val (contractEntity) = contractRepo.findContractByIdAndOwner(UUID.fromString(contractId), ownerId)
              ?: throw IllegalArgumentException("Contract with id $contractId not found or you don't have permission to update it")

          // Validate contract yaml before creating
          val result = validateContract(contractYaml)
          val contractValidationResult = result.second

          // If contract is invalid, then we need to return an error back to user
          if (!contractValidationResult.isValid) {
            return Pair(400, ErrorResponse(
             message = "Contract is not valid",
             errors = contractValidationResult.errors
            ))
          }

          // Get the contractYaml as an object
          val contractYamlObject = result.first!!

          // Confirm that the contract version is greater than the latest version
          if (contractYamlObject.version <= contractEntity.latestVersion) {
            throw IllegalArgumentException("Set Contract version: (${contractYamlObject.version}) cannot be less or equal to the last created version of this contract (${contractEntity.latestVersion})")
          }

          // Serialize contract as full contract and content only
          val (contentJson, contentOnlyJson) = serializeContract(contractYamlObject)
          
          // Calculate content hash (excluding metadata: name, version, description)
          val contentHash = calculateHash(contentOnlyJson)
          
          // Return an error because contract.contentHash matches existing contract
          if (contractEntity.contentHash == contentHash) {
            throw IllegalArgumentException("Contract content has not changed")
          }
          
          // Get latest version
          val nextVersion = contractEntity.latestVersion + 1

          // Update contract's contentHash and latestVersion and description
          val (_, updatedContractDAO) = contractRepo.updateContractFields(contractEntity, mapOf(
            "description" to contractYamlObject.description,
            "contentHash" to contentHash,
            "latestVersion" to nextVersion
          )) ?: throw IllegalArgumentException("Failed to update contract")
          
          // Create new version
          val (_, newVersionDAO) = contractRepo.createContractVersion(
              contractId = contractEntity.id.value,
              version = nextVersion,
              content = contentJson,
              contentHash = contentHash,
              createdBy = createdBy
          )
          return Pair(200, ContractResponse(
            contract = updatedContractDAO,
            contractVersion = newVersionDAO
          ))
         } catch (e: Exception) {
          println("E message ${e.message}")
            if (e is IllegalArgumentException) {
              return Pair(400, mapOf<String, String>(
                "message" to (e.message ?: "Bad Request")
              ))
            } else {
             return Pair(500, mapOf<String, String>(
               "message" to "Internal Server Error"
             ))
            }
         }
    }
    
    /**
     * Serializes contract to JSON string as full contract and content only
     */
    private fun serializeContract(contractYaml: ContractYaml): Pair<String, String> {
        val contentOnly = ContractContent(
          parties = contractYaml.parties,
          steps = contractYaml.steps,
          groups = contractYaml.groups,
          validation = contractYaml.validation
        )
        return Pair(json.encodeToString(contractYaml), json.encodeToString(contentOnly))
    }
    
    /**
     * Calculates SHA-256 hash of content for deduplication
     */
    private fun calculateHash(content: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
        val hashBytes = digest.digest(content.toByteArray())
        return hashBytes.joinToString("") { "%02x".format(it) }
    }
    
    /**
     * Retrieves a contract by ID
     */
    fun getContractById(contractId: UUID): Pair<Contract, ContractDAO>? {
        return contractRepo.findContractById(contractId)
    }
    
    /**
     * Retrieves the latest version of a contract
     */
    fun getLatestContractVersion(contractId: UUID): Pair<ContractVersion, ContractVersionDAO>? {
        return contractRepo.getLatestVersion(contractId)
    }
}