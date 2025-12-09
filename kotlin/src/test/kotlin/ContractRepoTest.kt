package dev.threadify

import dev.threadify.Repository.ContractRepo
import dev.threadify.Schemas.DBModels.Contract
import dev.threadify.Schemas.DBModels.Contracts
import dev.threadify.Schemas.DBModels.ContractVersion
import dev.threadify.Schemas.DBModels.ContractVersions
import org.jetbrains.exposed.sql.Database
import org.jetbrains.exposed.sql.SchemaUtils
import org.jetbrains.exposed.sql.transactions.transaction
import kotlin.test.*
import java.util.UUID

/**
 * Unit tests for ContractRepo.
 * Tests the repository layer methods for database operations.
 */
class ContractRepoTest {
    
    private lateinit var contractRepo: ContractRepo
    private val testOwnerId = "test-owner-123"
    private val testCreatedBy = "test-user-456"
    
    @BeforeTest
    fun setup() {
        // Setup in-memory database for testing
        Database.connect("jdbc:h2:mem:test;DB_CLOSE_DELAY=-1;", driver = "org.h2.Driver")
        
        transaction {
            SchemaUtils.create(Contracts, ContractVersions)
        }
        
        contractRepo = ContractRepo()
    }
    
    @AfterTest
    fun teardown() {
        transaction {
            SchemaUtils.drop(Contracts, ContractVersions)
        }
    }
    
    // ========== findContractByIdAndOwnerNotDeleted Tests ==========
    
    @Test
    fun `test findContractByIdAndOwnerNotDeleted returns contract when not deleted`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Find the contract
        val result = contractRepo.findContractByIdAndOwnerNotDeleted(contract.id.value, testOwnerId)
        
        assertNotNull(result)
        assertEquals(contract.id.value, result.first.id.value)
        assertEquals("test_contract", result.second.name)
    }
    
    @Test
    fun `test findContractByIdAndOwnerNotDeleted returns null when deleted`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Soft delete the contract
        contractRepo.softDeleteContract(contract)
        
        // Try to find the deleted contract
        val result = contractRepo.findContractByIdAndOwnerNotDeleted(contract.id.value, testOwnerId)
        
        assertNull(result)
    }
    
    @Test
    fun `test findContractByIdAndOwnerNotDeleted returns null for wrong owner`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Try to find with different owner
        val result = contractRepo.findContractByIdAndOwnerNotDeleted(contract.id.value, "different-owner")
        
        assertNull(result)
    }
    
    // ========== softDeleteContract Tests ==========
    
    @Test
    fun `test softDeleteContract marks contract as deleted`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        assertFalse(contract.isDeleted)
        
        // Soft delete the contract
        val (deletedContract, deletedDAO) = contractRepo.softDeleteContract(contract)
        
        assertTrue(deletedContract.isDeleted)
        assertTrue(deletedDAO.isDeleted)
    }
    
    @Test
    fun `test softDeleteContract updates updatedAt timestamp`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        val originalUpdatedAt = contract.updatedAt
        
        // Wait a tiny bit to ensure timestamp difference
        Thread.sleep(10)
        
        // Soft delete the contract
        val (deletedContract, _) = contractRepo.softDeleteContract(contract)
        
        assertTrue(deletedContract.updatedAt > originalUpdatedAt)
    }
    
    // ========== getLatestVersionNotDeleted Tests ==========
    
    @Test
    fun `test getLatestVersionNotDeleted returns latest non-deleted version`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create version 1
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        // Create version 2
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 2,
            content = "content2",
            createdBy = testCreatedBy,
            contentHash = "hash2"
        )
        
        // Get latest version
        val result = contractRepo.getLatestVersionNotDeleted(contract.id.value)
        
        assertNotNull(result)
        assertEquals(2, result.second.version)
    }
    
    @Test
    fun `test getLatestVersionNotDeleted excludes deleted versions`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create version 1
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        // Create version 2
        val (version2, _) = contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 2,
            content = "content2",
            createdBy = testCreatedBy,
            contentHash = "hash2"
        )
        
        // Delete version 2
        contractRepo.softDeleteContractVersion(version2)
        
        // Get latest version (should be version 1 now)
        val result = contractRepo.getLatestVersionNotDeleted(contract.id.value)
        
        assertNotNull(result)
        assertEquals(1, result.second.version)
    }
    
    @Test
    fun `test getLatestVersionNotDeleted returns null when all versions deleted`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create version 1
        val (version1, _) = contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        // Delete version 1
        contractRepo.softDeleteContractVersion(version1)
        
        // Get latest version
        val result = contractRepo.getLatestVersionNotDeleted(contract.id.value)
        
        assertNull(result)
    }
    
    // ========== getContractVersion Tests ==========
    
    @Test
    fun `test getContractVersion returns specific version`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create version 1
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        // Create version 2
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 2,
            content = "content2",
            createdBy = testCreatedBy,
            contentHash = "hash2"
        )
        
        // Get version 1 specifically
        val result = contractRepo.getContractVersion(contract.id.value, 1)
        
        assertNotNull(result)
        assertEquals(1, result.second.version)
        assertEquals("content1", result.second.content)
    }
    
    @Test
    fun `test getContractVersion returns null for non-existent version`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Try to get non-existent version
        val result = contractRepo.getContractVersion(contract.id.value, 999)
        
        assertNull(result)
    }
    
    // ========== getAllVersionsMetadata Tests ==========
    
    @Test
    fun `test getAllVersionsMetadata returns all non-deleted versions`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create multiple versions
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 2,
            content = "content2",
            createdBy = testCreatedBy,
            contentHash = "hash2"
        )
        
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 3,
            content = "content3",
            createdBy = testCreatedBy,
            contentHash = "hash3"
        )
        
        // Get all versions
        val versions = contractRepo.getAllVersionsMetadata(contract.id.value)
        
        assertEquals(3, versions.size)
        // Should be ordered by version DESC
        assertEquals(3, versions[0].version)
        assertEquals(2, versions[1].version)
        assertEquals(1, versions[2].version)
    }
    
    @Test
    fun `test getAllVersionsMetadata excludes deleted versions`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create multiple versions
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        val (version2, _) = contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 2,
            content = "content2",
            createdBy = testCreatedBy,
            contentHash = "hash2"
        )
        
        contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 3,
            content = "content3",
            createdBy = testCreatedBy,
            contentHash = "hash3"
        )
        
        // Delete version 2
        contractRepo.softDeleteContractVersion(version2)
        
        // Get all versions
        val versions = contractRepo.getAllVersionsMetadata(contract.id.value)
        
        assertEquals(2, versions.size)
        assertEquals(3, versions[0].version)
        assertEquals(1, versions[1].version)
    }
    
    @Test
    fun `test getAllVersionsMetadata returns empty list for no versions`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Get all versions (none exist)
        val versions = contractRepo.getAllVersionsMetadata(contract.id.value)
        
        assertEquals(0, versions.size)
    }
    
    // ========== softDeleteContractVersion Tests ==========
    
    @Test
    fun `test softDeleteContractVersion marks version as deleted`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create version 1
        val (version, _) = contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        assertFalse(version.isDeleted)
        
        // Soft delete the version
        val (deletedVersion, deletedDAO) = contractRepo.softDeleteContractVersion(version)
        
        assertTrue(deletedVersion.isDeleted)
        assertTrue(deletedDAO.isDeleted)
    }
    
    @Test
    fun `test softDeleteContractVersion updates updatedAt timestamp`() {
        // Create a contract
        val (contract, _) = contractRepo.createContract(
            name = "test_contract",
            description = "Test description",
            latestContentHash = "hash123",
            ownerId = testOwnerId
        )
        
        // Create version 1
        val (version, _) = contractRepo.createContractVersion(
            contractId = contract.id.value,
            version = 1,
            content = "content1",
            createdBy = testCreatedBy,
            contentHash = "hash1"
        )
        
        val originalUpdatedAt = version.updatedAt
        
        // Wait a tiny bit to ensure timestamp difference
        Thread.sleep(10)
        
        // Soft delete the version
        val (deletedVersion, _) = contractRepo.softDeleteContractVersion(version)
        
        assertTrue(deletedVersion.updatedAt > originalUpdatedAt)
    }
}
