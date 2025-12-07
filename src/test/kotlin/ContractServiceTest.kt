package dev.threadify

import dev.threadify.Services.ContractService
import dev.threadify.Repository.ContractRepo
import dev.threadify.Schemas.DBModels.Contract
import dev.threadify.Schemas.DBModels.Contracts
import dev.threadify.Schemas.DBModels.ContractVersion
import dev.threadify.Schemas.DBModels.ContractVersions
import dev.threadify.Utilities.GlobalServices
import io.ktor.server.testing.*
import org.jetbrains.exposed.sql.Database
import org.jetbrains.exposed.sql.SchemaUtils
import org.jetbrains.exposed.sql.transactions.transaction
import kotlin.test.*
import java.util.UUID

/**
 * Unit tests for ContractService.
 * Tests the service layer methods for contract operations.
 */
class ContractServiceTest {
    
    private lateinit var contractService: ContractService
    private lateinit var contractRepo: ContractRepo
    private val testOwnerId = "test-owner-123"
    private val testUserId = "test-user-456"
    
    @BeforeTest
    fun setup() {
        // Setup in-memory database for testing
        Database.connect("jdbc:h2:mem:test;DB_CLOSE_DELAY=-1;", driver = "org.h2.Driver")
        
        transaction {
            SchemaUtils.create(Contracts, ContractVersions)
        }
        
        contractService = ContractService()
        contractRepo = ContractRepo()
    }
    
    @AfterTest
    fun teardown() {
        transaction {
            SchemaUtils.drop(Contracts, ContractVersions)
        }
    }
    
    // Helper function to create a valid contract YAML
    private fun createValidContractYaml(contractName: String = "test_contract"): String {
        return """
            contract_name: $contractName
            version: 1
            description: Test contract for unit testing
            
            parties:
              - merchant
              - payment_processor
            
            steps:
              - id: step1
                owner: merchant
                business_context:
                  amount: number
              
              - id: step2
                owner: payment_processor
                depends_on: step1
                timeout: 2s
            
            validation:
              max_duration: 10s
        """.trimIndent()
    }
    
    // ========== getContract Tests ==========
    
    @Test
    fun `test getContract returns contract successfully for owner`() {
        // Create a contract first
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        assertEquals(200, createResult.first)
        
        // Extract contract ID from response
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Get the contract as owner
        val getResult = contractService.getContract(contractId, testOwnerId)
        
        assertEquals(200, getResult.first)
        val getResponse = getResult.second as dev.threadify.Schemas.Response.ContractWithOwnershipResponse
        assertEquals(contractId, getResponse.contract.id)
        assertEquals("test_contract", getResponse.contract.name)
        assertTrue(getResponse.isOwner)
    }
    
    @Test
    fun `test getContract returns 403 for private contract accessed by non-owner`() {
        // Create a private contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Try to access as different user
        val getResult = contractService.getContract(contractId, "different-user")
        
        assertEquals(403, getResult.first)
        val errorResponse = getResult.second as Map<*, *>
        assertEquals("Access denied. This contract is private.", errorResponse["message"])
    }
    
    @Test
    fun `test getContract allows public contract access by non-owner`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Make the contract public
        transaction {
            val contract = Contract.findById(UUID.fromString(contractId))
            contract?.isPublic = true
        }
        
        // Access as different user
        val getResult = contractService.getContract(contractId, "different-user")
        
        assertEquals(200, getResult.first)
        val getResponse = getResult.second as dev.threadify.Schemas.Response.ContractWithOwnershipResponse
        assertEquals(contractId, getResponse.contract.id)
        assertFalse(getResponse.isOwner)
    }
    
    @Test
    fun `test getContract with specific version returns correct version`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Update contract to create version 2
        val yaml2 = createValidContractYaml().replace("version: 1", "version: 2")
        contractService.updateContract(contractId, testOwnerId, testUserId, yaml2)
        
        // Get version 1 specifically
        val getResult = contractService.getContract(contractId, testOwnerId, version = 1)
        
        assertEquals(200, getResult.first)
        val getResponse = getResult.second as dev.threadify.Schemas.Response.ContractWithOwnershipResponse
        assertEquals(1, getResponse.contractVersion.version)
    }
    
    @Test
    fun `test getContract returns 404 for non-existent contract`() {
        val fakeId = UUID.randomUUID().toString()
        val result = contractService.getContract(fakeId, testOwnerId)
        
        assertEquals(404, result.first)
        val response = result.second as Map<*, *>
        assertEquals("Contract not found", response["message"])
    }
    
    @Test
    fun `test getContract returns 404 for deleted contract`() {
        // Create and delete a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        contractService.deleteContract(contractId, testOwnerId)
        
        // Try to get the deleted contract
        val getResult = contractService.getContract(contractId, testOwnerId)
        
        assertEquals(404, getResult.first)
    }
    
    @Test
    fun `test getContract returns 400 for invalid UUID format`() {
        val result = contractService.getContract("invalid-uuid", testOwnerId)
        
        assertEquals(400, result.first)
        val response = result.second as Map<*, *>
        assertEquals("Invalid contract ID format", response["message"])
    }
    
    // ========== deleteContract Tests ==========
    
    @Test
    fun `test deleteContract soft deletes contract successfully`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Delete the contract
        val deleteResult = contractService.deleteContract(contractId, testOwnerId)
        
        assertEquals(200, deleteResult.first)
        val deleteResponse = deleteResult.second as Map<*, *>
        assertEquals("Contract deleted successfully", deleteResponse["message"])
        
        // Verify contract is marked as deleted
        val contract = deleteResponse["contract"] as dev.threadify.DAOs.ContractDAO
        assertTrue(contract.isDeleted)
    }
    
    @Test
    fun `test deleteContract returns 404 for non-existent contract`() {
        val fakeId = UUID.randomUUID().toString()
        val result = contractService.deleteContract(fakeId, testOwnerId)
        
        assertEquals(404, result.first)
    }
    
    @Test
    fun `test deleteContract returns 400 when already deleted`() {
        // Create and delete a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        contractService.deleteContract(contractId, testOwnerId)
        
        // Try to delete again
        val deleteResult = contractService.deleteContract(contractId, testOwnerId)
        
        assertEquals(400, deleteResult.first)
        val deleteResponse = deleteResult.second as Map<*, *>
        assertEquals("Contract is already deleted", deleteResponse["message"])
    }
    
    @Test
    fun `test deleteContract returns 404 for wrong owner`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Try to delete with different owner
        val deleteResult = contractService.deleteContract(contractId, "different-owner")
        
        assertEquals(404, deleteResult.first)
    }
    
    // ========== deleteContractVersion Tests ==========
    
    @Test
    fun `test deleteContractVersion soft deletes version successfully`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Delete version 1
        val deleteResult = contractService.deleteContractVersion(contractId, 1, testOwnerId)
        
        assertEquals(200, deleteResult.first)
        val deleteResponse = deleteResult.second as Map<*, *>
        assertEquals("Contract version deleted successfully", deleteResponse["message"])
        
        // Verify version is marked as deleted
        val version = deleteResponse["version"] as dev.threadify.DAOs.ContractVersionDAO
        assertTrue(version.isDeleted)
    }
    
    @Test
    fun `test deleteContractVersion returns 404 for non-existent version`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Try to delete non-existent version
        val deleteResult = contractService.deleteContractVersion(contractId, 999, testOwnerId)
        
        assertEquals(404, deleteResult.first)
    }
    
    @Test
    fun `test deleteContractVersion returns 400 when already deleted`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Delete version 1
        contractService.deleteContractVersion(contractId, 1, testOwnerId)
        
        // Try to delete again
        val deleteResult = contractService.deleteContractVersion(contractId, 1, testOwnerId)
        
        assertEquals(400, deleteResult.first)
        val deleteResponse = deleteResult.second as Map<*, *>
        assertEquals("Contract version is already deleted", deleteResponse["message"])
    }
    
    // ========== getAllContractVersions Tests ==========
    
    @Test
    fun `test getAllContractVersions returns all versions for owner`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Create version 2 - must change content, not just version
        val yaml2 = createValidContractYaml()
            .replace("version: 1", "version: 2")
            .replace("timeout: 2s", "timeout: 3s")  // Change content
        val updateResult = contractService.updateContract(contractId, testOwnerId, testUserId, yaml2)
        
        // Verify update succeeded
        assertEquals(200, updateResult.first, "Update should succeed")
        
        // Get all versions as owner
        val versionsResult = contractService.getAllContractVersions(contractId, testOwnerId)
        
        assertEquals(200, versionsResult.first)
        val versionsResponse = versionsResult.second as Map<*, *>
        assertEquals(2, versionsResponse["totalVersions"])
        assertTrue(versionsResponse["isOwner"] as Boolean)
        
        val versions = versionsResponse["versions"] as List<*>
        assertEquals(2, versions.size)
    }
    
    @Test
    fun `test getAllContractVersions returns 403 for private contract accessed by non-owner`() {
        // Create a private contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Try to access as different user
        val versionsResult = contractService.getAllContractVersions(contractId, "different-user")
        
        assertEquals(403, versionsResult.first)
        val errorResponse = versionsResult.second as Map<*, *>
        assertEquals("Access denied. This contract is private.", errorResponse["message"])
    }
    
    @Test
    fun `test getAllContractVersions allows public contract access by non-owner`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Make the contract public
        transaction {
            val contract = Contract.findById(UUID.fromString(contractId))
            contract?.isPublic = true
        }
        
        // Access as different user
        val versionsResult = contractService.getAllContractVersions(contractId, "different-user")
        
        assertEquals(200, versionsResult.first)
        val versionsResponse = versionsResult.second as Map<*, *>
        assertFalse(versionsResponse["isOwner"] as Boolean)
    }
    
    @Test
    fun `test getAllContractVersions excludes deleted versions`() {
        // Create a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Create version 2 - must change content
        val yaml2 = createValidContractYaml()
            .replace("version: 1", "version: 2")
            .replace("timeout: 2s", "timeout: 3s")  // Change content
        contractService.updateContract(contractId, testOwnerId, testUserId, yaml2)
        
        // Delete version 1
        contractService.deleteContractVersion(contractId, 1, testOwnerId)
        
        // Get all versions
        val versionsResult = contractService.getAllContractVersions(contractId, testOwnerId)
        
        assertEquals(200, versionsResult.first)
        val versionsResponse = versionsResult.second as Map<*, *>
        assertEquals(1, versionsResponse["totalVersions"])
    }
    
    @Test
    fun `test getAllContractVersions returns 404 for non-existent contract`() {
        val fakeId = UUID.randomUUID().toString()
        val result = contractService.getAllContractVersions(fakeId, testOwnerId)
        
        assertEquals(404, result.first)
    }
    
    @Test
    fun `test getAllContractVersions returns 404 for deleted contract`() {
        // Create and delete a contract
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        contractService.deleteContract(contractId, testOwnerId)
        
        // Try to get versions
        val versionsResult = contractService.getAllContractVersions(contractId, testOwnerId)
        
        assertEquals(404, versionsResult.first)
    }
    
    @Test
    fun `test getAllContractVersions returns empty list for contract with no versions`() {
        // This test ensures the method handles edge cases
        // In practice, every contract should have at least one version
        val yaml = createValidContractYaml()
        val createResult = contractService.createContract(testOwnerId, testUserId, yaml)
        val response = createResult.second as dev.threadify.Schemas.Response.ContractResponse
        val contractId = response.contract.id
        
        // Delete the only version
        contractService.deleteContractVersion(contractId, 1, testOwnerId)
        
        // Get all versions
        val versionsResult = contractService.getAllContractVersions(contractId, testOwnerId)
        
        assertEquals(200, versionsResult.first)
        val versionsResponse = versionsResult.second as Map<*, *>
        assertEquals(0, versionsResponse["totalVersions"])
    }
}
