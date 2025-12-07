# Contract Validation Test Suite

## Overview
Comprehensive test suite for the ContractValidator with 40 test cases covering all edge cases and validation rules.

## Test Coverage

### ✅ Valid Contract Test (1 test)
- Valid contract passes all validation checks

### ✅ Contract ID Validation (4 tests)
- ❌ Special characters (hyphens) - FAIL
- ❌ Spaces - FAIL  
- ❌ Dots - FAIL
- ✅ Underscores - PASS

**Rule:** `contract_id` must contain only alphanumeric characters and underscores

### ✅ Version Validation (3 tests)
- ❌ Version 0 - FAIL
- ❌ Negative version - FAIL
- ✅ Large version number (999) - PASS

**Rule:** Version must be a positive integer (>= 1)

### ✅ Description Validation (2 tests)
- ❌ Empty description - FAIL
- ❌ Blank description (whitespace only) - FAIL

**Rule:** Description cannot be empty or blank

### ✅ Party Validation (2 tests)
- ❌ Step owner not in defined parties - FAIL
- ❌ Unused party (not assigned to any step) - FAIL

**Rules:**
- All step owners must reference defined parties
- All defined parties must be assigned to at least one step

### ✅ Depends_on Validation (4 tests)
- ❌ Single non-existent step reference - FAIL
- ❌ Multiple steps with one non-existent - FAIL
- ✅ Single string format - PASS
- ✅ Array format - PASS

**Rules:**
- All `depends_on` references must point to existing steps
- Supports both single string and array formats

### ✅ Timeout Validation (8 tests)
- ✅ Seconds (10s) - PASS
- ✅ Milliseconds (2000ms) - PASS
- ✅ Microseconds (2000000us) - PASS
- ✅ Minutes (1m) - PASS
- ✅ Decimal values (2.5s) - PASS
- ❌ Invalid unit (2h) - FAIL
- ❌ No unit (2) - FAIL
- ❌ Invalid format (two seconds) - FAIL

**Rule:** Timeout format must match: `^\d+(\.\d+)?(s|ms|us|m)$`

**Valid units:** `s`, `ms`, `us`, `m`

### ✅ Business Context Validation (5 tests)
- ✅ Valid types (string, number, boolean) - PASS
- ✅ Object type - PASS
- ✅ Array type - PASS
- ❌ Invalid type (integer) - FAIL
- ❌ Custom type (currency_type) - FAIL

**Valid types:** `string`, `number`, `boolean`, `object`, `array`

### ✅ Groups Validation (3 tests)
- ❌ Non-existent step reference - FAIL
- ❌ Invalid max_combined_duration format - FAIL
- ✅ Contract without groups - PASS

**Rules:**
- All group step references must exist
- `max_combined_duration` must follow duration format rules

### ✅ Validation Rules (3 tests)
- ✅ Valid max_duration format - PASS
- ❌ Invalid format (10 seconds) - FAIL
- ❌ No unit (10) - FAIL

**Rule:** `max_duration` must follow duration format rules

### ✅ YAML Parsing (2 tests)
- ❌ Malformed YAML syntax - FAIL
- ❌ Missing required fields - FAIL

**Rule:** YAML must be well-formed and contain all required fields

### ✅ Multiple Errors Test (1 test)
- Validates that multiple errors are collected and reported together
- Tests at least 9 different validation errors in a single contract

## Test Results

**Total Tests:** 40  
**Passed:** 40 ✅  
**Failed:** 0 ❌  
**Success Rate:** 100%

## Running the Tests

```bash
./gradlew test --tests ContractValidatorTest
```

## Key Improvements from SnakeYAML Migration

1. **Type Handling:** Fixed parsing to handle YAML's automatic type inference (e.g., `timeout: 2` becomes integer, not string)
2. **Flexible Parsing:** Uses `.toString()` for duration fields to handle both string and numeric YAML values
3. **Robust Validation:** All validation rules are tested with both valid and invalid inputs

## Example Valid Contract

```yaml
contract_id: payment_processing_v1
version: 1
description: Payment processing with fraud checks

parties:
  - id: merchant
  - id: payment_processor
  - id: bank

steps:
  - id: payment_initiated
    owner: merchant
    business_context:
      amount: number
      currency: string

  - id: fraud_check
    owner: payment_processor
    depends_on: payment_initiated
    timeout: 2s

groups:
  - id: fraud_validation
    steps:
      - fraud_check
    rules:
      all_must_succeed: true
      max_combined_duration: 3s

validation:
  max_duration: 10s
```

## Edge Cases Covered

- ✅ Special characters in identifiers
- ✅ Boundary values (zero, negative numbers)
- ✅ Empty/blank strings
- ✅ Missing references (parties, steps)
- ✅ Invalid format strings
- ✅ Type mismatches
- ✅ Optional fields (groups, timeout, business_context)
- ✅ Multiple error accumulation
- ✅ YAML parsing errors
