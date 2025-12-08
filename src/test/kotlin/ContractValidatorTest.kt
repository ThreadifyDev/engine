package dev.threadify

import dev.threadify.Utilities.ContractValidator
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class ContractValidatorTest {
    
    private val validator = ContractValidator()
    
    // Valid baseline contract for reference
    private val validContract = """
        contract_name: payment_processing_v1
        version: 1
        description: Payment processing with fraud checks
        
        parties:
          - merchant
          - payment_processor
          - bank
        
        steps:
          - id: payment_initiated
            owner: merchant
            business_context:
              amount: number
              currency: string
              customer_id: string
        
          - id: fraud_check
            owner: payment_processor
            depends_on: payment_initiated
            timeout: 2s
            business_context:
              fraud_score: number
              recommendation: string
        
          - id: risk_assessment
            owner: payment_processor
            depends_on: payment_initiated
            timeout: 2s
            business_context:
              risk_score: number
              risk_level: string
        
          - id: bank_authorization
            owner: bank
            depends_on:
              - fraud_check
              - risk_assessment
            timeout: 5s
            business_context:
              authorized: boolean
              authorization_code: string
        
          - id: payment_complete
            owner: merchant
            depends_on: bank_authorization
            business_context:
              transaction_id: string
        
        groups:
          - id: fraud_validation
            steps:
              - fraud_check
              - risk_assessment
            rules:
              all_must_succeed: true
              max_combined_duration: 3s
        
        validation:
          max_duration: 10s
    """.trimIndent()
    
    @Test
    fun `test valid contract passes validation`() {
        val (_, result) = validator.validate(validContract)
        assertTrue(result.isValid, "Valid contract should pass validation")
        assertEquals(0, result.errors.size)
    }
    
    // ========== contract_id validation tests ==========
    
    @Test
    fun `test contract_name with special characters fails`() {
        val yaml = validContract.replace("contract_name: payment_processing_v1", "contract_name: payment-processing-v1")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "contract_name" && it.message.contains("alphanumeric")
        })
    }
    
    @Test
    fun `test contract_name with spaces fails`() {
        val yaml = validContract.replace("contract_name: payment_processing_v1", "contract_name: payment processing v1")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "contract_name" && it.message.contains("alphanumeric")
        })
    }
    
    @Test
    fun `test contract_name with dots fails`() {
        val yaml = validContract.replace("contract_name: payment_processing_v1", "contract_name: payment.processing.v1")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "contract_name" && it.message.contains("alphanumeric")
        })
    }
    
    @Test
    fun `test contract_name with underscores passes`() {
        val yaml = validContract.replace("contract_name: payment_processing_v1", "contract_name: payment_processing_v1_test")
        val (_, result) = validator.validate(yaml)
        
        assertTrue(result.isValid)
    }
    
    // ========== version validation tests ==========
    
    @Test
    fun `test version zero fails`() {
        val yaml = validContract.replace("version: 1", "version: 0")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "version" && it.message.contains("positive integer")
        })
    }
    
    @Test
    fun `test negative version fails`() {
        val yaml = validContract.replace("version: 1", "version: -1")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "version" && it.message.contains("positive integer")
        })
    }
    
    @Test
    fun `test large version number passes`() {
        val yaml = validContract.replace("version: 1", "version: 999")
        val (_, result) = validator.validate(yaml)
        
        assertTrue(result.isValid)
    }
    
    // ========== description validation tests ==========
    
    @Test
    fun `test empty description fails`() {
        val yaml = validContract.replace("description: Payment processing with fraud checks", "description: \"\"")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "description" && it.message.contains("Cannot be empty")
        })
    }
    
    @Test
    fun `test blank description fails`() {
        val yaml = validContract.replace("description: Payment processing with fraud checks", "description: \"   \"")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "description" && it.message.contains("Cannot be empty")
        })
    }
    
    // ========== party validation tests ==========
    
    @Test
    fun `test step owner not in parties fails`() {
        val yaml = validContract.replaceFirst("owner: merchant", "owner: unknown_party")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("owner") && it.message.contains("not a defined party")
        })
    }
    
    @Test
    fun `test unused party fails`() {
        val yaml = validContract.replace(
            "parties:\n  - merchant\n  - payment_processor\n  - bank",
            "parties:\n  - merchant\n  - payment_processor\n  - bank\n  - unused_party"
        )
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "parties.unused_party" && it.message.contains("not assigned to any step")
        })
    }
    
    // ========== depends_on validation tests ==========
    
    @Test
    fun `test depends_on with non-existent step fails`() {
        val yaml = validContract.replaceFirst("depends_on: payment_initiated", "depends_on: non_existent_step")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("depends_on") && it.message.contains("does not exist")
        })
    }
    
    @Test
    fun `test depends_on with multiple non-existent steps fails`() {
        val yaml = validContract.replace(
            "depends_on:\n      - fraud_check\n      - risk_assessment",
            "depends_on:\n      - fraud_check\n      - non_existent_step"
        )
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("depends_on") && it.message.contains("non_existent_step")
        })
    }
    
    @Test
    fun `test depends_on as single string passes`() {
        // Already tested in valid contract, but explicit test
        val (_, result) = validator.validate(validContract)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test depends_on as array passes`() {
        // Already tested in valid contract with bank_authorization step
        val (_, result) = validator.validate(validContract)
        assertTrue(result.isValid)
    }
    
    // ========== step type validation tests ==========
    
    @Test
    fun `test step type managed passes`() {
        val yaml = validContract.replace(
            "- id: payment_initiated\n      owner: merchant",
            "- id: payment_initiated\n      owner: merchant\n      type: managed"
        )
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test step type human_in_loop passes`() {
        val yaml = validContract.replace(
            "- id: payment_initiated\n      owner: merchant",
            "- id: payment_initiated\n      owner: merchant\n      type: human_in_loop"
        )
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test step type external passes`() {
        val yaml = validContract.replace(
            "- id: payment_initiated\n      owner: merchant",
            "- id: payment_initiated\n      owner: merchant\n      type: external"
        )
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test step type defaults to managed when not specified`() {
        // Valid contract doesn't specify type, should default to managed
        val (_, result) = validator.validate(validContract)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test invalid step type fails`() {
        val yaml = validContract.replace(
            "- id: payment_initiated\n      owner: merchant",
            "- id: payment_initiated\n      owner: merchant\n      type: automated"
        )
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("type") && it.message.contains("managed, human_in_loop, or external")
        })
    }
    
    // ========== timeout validation tests ==========
    
    @Test
    fun `test timeout with seconds passes`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 10s")
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test timeout with milliseconds passes`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 2000ms")
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test timeout with microseconds passes`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 2000000us")
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test timeout with minutes passes`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 1m")
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test timeout with decimal passes`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 2.5s")
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test timeout with invalid unit fails`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 2w")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("timeout") && it.message.contains("Invalid duration format")
        })
    }
    
    @Test
    fun `test timeout without unit fails`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: 2")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("timeout") && it.message.contains("Invalid duration format")
        })
    }
    
    @Test
    fun `test timeout with invalid format fails`() {
        val yaml = validContract.replaceFirst("timeout: 2s", "timeout: two seconds")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("timeout") && it.message.contains("Invalid duration format")
        })
    }
    
    // ========== business_context validation tests ==========
    
    @Test
    fun `test business_context with valid types passes`() {
        val (_, result) = validator.validate(validContract)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test business_context with object type passes`() {
        val yaml = validContract.replace(
            "customer_id: string",
            "customer_id: string\n      metadata: object"
        )
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test business_context with array type passes`() {
        val yaml = validContract.replace(
            "customer_id: string",
            "customer_id: string\n      tags: array"
        )
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test business_context with invalid type fails`() {
        val yaml = validContract.replace("amount: number", "amount: integer")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("business_context") && it.message.contains("Invalid type")
        })
    }
    
    @Test
    fun `test business_context with custom type fails`() {
        val yaml = validContract.replace("currency: string", "currency: currency_type")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("business_context") && it.message.contains("Invalid type")
        })
    }
    
    // ========== groups validation tests ==========
    
    @Test
    fun `test group with non-existent step fails`() {
        val yaml = validContract.replace(
            "steps:\n      - fraud_check\n      - risk_assessment",
            "steps:\n      - fraud_check\n      - non_existent_step"
        )
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("groups") && it.message.contains("does not exist")
        })
    }
    
    @Test
    fun `test group with invalid max_combined_duration fails`() {
        val yaml = validContract.replace("max_combined_duration: 3s", "max_combined_duration: 3hours")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field.contains("max_combined_duration") && it.message.contains("Invalid duration format")
        })
    }
    
    @Test
    fun `test contract without groups passes`() {
        val yaml = validContract.replace(
            Regex("groups:.*?validation:", RegexOption.DOT_MATCHES_ALL),
            "validation:"
        )
        val (_, result) = validator.validate(yaml)
        assertTrue(result.isValid)
    }
    
    // ========== validation rules tests ==========
    
    @Test
    fun `test validation max_duration with valid format passes`() {
        val (_, result) = validator.validate(validContract)
        assertTrue(result.isValid)
    }
    
    @Test
    fun `test validation max_duration with invalid format fails`() {
        val yaml = validContract.replace("max_duration: 10s", "max_duration: 10 seconds")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "validation.max_duration" && it.message.contains("Invalid duration format")
        })
    }
    
    @Test
    fun `test validation max_duration without unit fails`() {
        val yaml = validContract.replace("max_duration: 10s", "max_duration: 10")
        val (_, result) = validator.validate(yaml)
        
        assertFalse(result.isValid)
        assertTrue(result.errors.any { 
            it.field == "validation.max_duration" && it.message.contains("Invalid duration format")
        })
    }
    
    // ========== YAML parsing tests ==========
    
    @Test
    fun `test malformed YAML fails`() {
        val yaml = """
            contract_name: test
            version: 1
            description: test
            parties:
              - party1
            steps:
              - id: step1
                owner: party1
            validation
              max_duration: 10s
        """.trimIndent()
        
        val (_, result) = validator.validate(yaml)
        assertFalse(result.isValid)
        assertTrue(result.errors.any { it.field == "yaml" })
    }
    
    @Test
    fun `test missing required field fails`() {
        val yaml = """
            contract_name: test
            version: 1
            parties:
              - party1
            steps:
              - id: step1
                owner: party1
            validation:
              max_duration: 10s
        """.trimIndent()
        
        val (_, result) = validator.validate(yaml)
        assertFalse(result.isValid)
        assertTrue(result.errors.any { it.field == "yaml" })
    }
    
    // ========== Multiple errors test ==========
    
    @Test
    fun `test multiple validation errors are all reported`() {
        val yaml = """
            contract_name: invalid-name-with-dashes
            version: 0
            description: ""
            parties:
              - party1
              - unused_party
            steps:
              - id: step1
                owner: non_existent_party
                depends_on: non_existent_step
                timeout: invalid_timeout
                business_context:
                  field1: invalid_type
            validation:
              max_duration: invalid
        """.trimIndent()
        
        val (_, result) = validator.validate(yaml)
        assertFalse(result.isValid)
        
        // Should have multiple errors
        assertTrue(result.errors.size >= 7, "Expected at least 7 errors, got ${result.errors.size}")
        
        // Verify specific errors exist
        assertTrue(result.errors.any { it.field == "contract_name" })
        assertTrue(result.errors.any { it.field == "version" })
        assertTrue(result.errors.any { it.field == "description" })
        assertTrue(result.errors.any { it.field.contains("owner") })
        assertTrue(result.errors.any { it.field.contains("depends_on") })
        assertTrue(result.errors.any { it.field.contains("timeout") })
        assertTrue(result.errors.any { it.field.contains("business_context") })
        assertTrue(result.errors.any { it.field == "validation.max_duration" })
        assertTrue(result.errors.any { it.field == "parties.unused_party" })
    }
}
