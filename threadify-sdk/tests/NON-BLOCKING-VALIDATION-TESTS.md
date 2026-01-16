# Non-Blocking Validation Tests - Complete Test Suite

Comprehensive end-to-end tests for all 7 non-blocking validation types in Threadify. These tests verify that violations are logged asynchronously without blocking step submission.

## Overview

Non-blocking validations run **after** a step is successfully recorded. They detect contract violations and log them as notifications without preventing workflow execution. This allows workflows to continue while maintaining a complete audit trail of violations.

---

## Test Files

### 1. **Invalid Transition Validation** (`e2e-invalid-transition.test.js`)

Tests step transition violations when steps don't follow the contract's defined transition paths.

**Validation Type:** `invalid_transition` (CRITICAL)

**Test Cases:**
- ✅ Skipping required intermediate steps
- ✅ Invalid transition paths (jumping to unrelated steps)
- ✅ Transitions from terminal steps
- ✅ Multiple invalid transitions in sequence
- ✅ Valid transition path (control test)
- ✅ Alternative valid paths (cancellation)

**Example Violation:**
```
order_placed → shipped (INVALID)
Expected: order_placed → payment_validation
```

**Run:**
```bash
node tests/e2e-invalid-transition.test.js
```

---

### 2. **Step Timeout Validation** (`e2e-step-timeout.test.js`)

Tests violations when step execution time exceeds the timeout defined in the contract.

**Validation Type:** `step_timeout_exceeded` (CRITICAL)

**Test Cases:**
- ✅ Short timeout exceeded (30s)
- ✅ Medium timeout exceeded (5m)
- ✅ Long timeout exceeded (2h)
- ✅ Very long timeout exceeded (24h)
- ✅ Edge case: Exactly at timeout
- ✅ Edge case: Just over timeout (1s)
- ✅ Within timeout (control test)
- ✅ Multiple timeout violations in same thread

**Example Violation:**
```
payment_validation took 45s (timeout: 30s)
Violation: Step exceeded timeout by 15s
```

**Run:**
```bash
node tests/e2e-step-timeout.test.js
```

---

### 3. **Max Duration Validation** (`e2e-max-duration.test.js`)

Tests violations when thread duration exceeds the max_duration defined in the contract.

**Validation Type:** `max_duration_exceeded` (CRITICAL)

**Test Cases:**
- ✅ Thread exceeding max duration (72h)
- ✅ Significantly over max duration (120h)
- ✅ Edge case: Exactly at max duration
- ✅ Edge case: Just over max duration (1h)
- ✅ Within max duration (control test)
- ✅ Fresh thread (< 1 minute)
- ✅ Long-running thread approaching limit
- ✅ Multiple steps after violation
- ✅ Completed thread within duration

**Example Violation:**
```
Thread running for 80h (max_duration: 72h)
Violation: Thread exceeded maximum duration by 8h
```

**Run:**
```bash
node tests/e2e-max-duration.test.js
```

---

### 4. **Multiple Terminal States Validation** (`e2e-multiple-terminals.test.js`)

Tests violations when a thread reaches multiple terminal states when not allowed.

**Validation Type:** `multiple_terminal_states` (CRITICAL/CONFIGURABLE)

**Test Cases:**
- ✅ Reaching multiple terminal states (violation)
- ✅ Single terminal state (control test)
- ✅ Alternative terminal path (cancellation)
- ✅ Attempting three terminal states
- ✅ Terminal state after delivery failure
- ✅ Retry after failure then terminal
- ✅ Immediate multiple terminal attempts

**Example Violation:**
```
Thread reached: delivered (terminal 1)
Then reached: order_cancelled (terminal 2)
Violation: Multiple terminal states not allowed
```

**Run:**
```bash
node tests/e2e-multiple-terminals.test.js
```

---

### 5. **Retry Limit Validation** (`e2e-retry-limit.test.js`)

Tests violations when step retry count exceeds max_retries defined in contract transitions.

**Validation Type:** `retry_limit_exceeded` (CRITICAL)

**Test Cases:**
- ✅ Retry limit exceeded (3 retries, attempting 4th)
- ✅ Within retry limit (control test)
- ✅ Exact retry limit (at threshold)
- ✅ Different steps with different retry limits
- ✅ Multiple retry violations in same thread

**Example Violation:**
```
payment_validation retry attempt 4 (max_retries: 3)
Violation: Step exceeded retry limit
```

**Run:**
```bash
node tests/e2e-retry-limit.test.js
```

**Note:** This test already exists in the codebase.

---

### 6. **Missing Optional Fields Validation** (`e2e-missing-optional-fields.test.js`)

Tests info-level notifications when optional business context fields are not provided.

**Validation Type:** `missing_optional_field` (INFO)

**Test Cases:**
- ✅ Missing all optional fields
- ✅ Missing some optional fields
- ✅ All optional fields provided (control test)
- ✅ Different steps with different optional fields
- ✅ Multiple steps missing optional fields
- ✅ Step with no optional fields defined
- ✅ Optional fields with empty string values
- ✅ Workflow with mixed optional field usage
- ✅ Cancellation path with missing optional
- ✅ Complete delivery with all optional fields

**Example Notification:**
```
order_placed missing optional fields: [notes, promo_code]
Severity: INFO (non-critical)
```

**Run:**
```bash
node tests/e2e-missing-optional-fields.test.js
```

---

### 7. **Extra Undocumented Fields Validation** (`e2e-extra-fields.test.js`)

Tests info-level notifications when fields not defined in contract business_context are provided.

**Validation Type:** `extra_undocumented_field` (INFO)

**Test Cases:**
- ✅ Single extra undocumented field
- ✅ Multiple extra undocumented fields
- ✅ Only documented fields (control test)
- ✅ Mix of documented and extra fields
- ✅ Different steps with extra fields
- ✅ Extra fields in multiple steps
- ✅ Extra fields with similar names to documented
- ✅ Extra fields in cancellation path
- ✅ Extra fields with special characters
- ✅ Complete workflow with mixed extra fields
- ✅ Extra fields vs missing optional fields (both violations)

**Example Notification:**
```
order_placed has extra fields: [internal_debug_flag, test_mode, trace_id]
Severity: INFO (informational)
```

**Run:**
```bash
node tests/e2e-extra-fields.test.js
```

---

## Validation Severity Levels

| Severity | Description | Action Taken |
|----------|-------------|--------------|
| **CRITICAL** | Blocks workflow progress | Step marked as "violated", thread may fail |
| **MAJOR** | Significant issue | Warning logged, thread continues |
| **MINOR** | Minor concern | Informational only |
| **WARNING** | Potential issue | Logged for review |
| **INFO** | Informational | No action required |

---

## Prerequisites

### 1. Server Running
```bash
cd threadify-go
go run cmd/server/main.go
```

Server should be running on `http://localhost:8081`

### 2. Archiver Running (Optional but Recommended)
```bash
cd threadify-go
go run cmd/archiver/main.go
```

The archiver consumes validation notifications and writes them to Postgres.

### 3. Contract Uploaded
The tests automatically upload the contract from:
```
threadify-go/examples/contract_example.yaml
```

### 4. Dependencies Installed
```bash
cd threadify-sdk
npm install
```

---

## Running the Tests

### Run Individual Test
```bash
node tests/e2e-invalid-transition.test.js
node tests/e2e-step-timeout.test.js
node tests/e2e-max-duration.test.js
node tests/e2e-multiple-terminals.test.js
node tests/e2e-retry-limit.test.js
node tests/e2e-missing-optional-fields.test.js
node tests/e2e-extra-fields.test.js
```

### Run All Non-Blocking Validation Tests
```bash
# Create a test runner script
for test in e2e-invalid-transition e2e-step-timeout e2e-max-duration e2e-multiple-terminals e2e-retry-limit e2e-missing-optional-fields e2e-extra-fields; do
  echo "Running $test..."
  node tests/${test}.test.js
  echo ""
done
```

---

## Expected Output

Each test will output:
- ✅ Test setup (login, contract upload, connection)
- 📋 Test case descriptions
- ℹ️ Step execution details
- ⚠️ Violation indicators
- ✅ Pass/fail status

**Example:**
```
🚀 Starting Invalid Transition Validation Tests

============================================================

✅ Logged in successfully
✅ Contract uploaded successfully
✅ SDK connected

📋 Test 1: Skipping Required Intermediate Steps
   Expected: order_placed → payment_validation → payment_validated
   Attempting: order_placed → payment_validated (SKIP payment_validation)

   Thread: thread-abc123
   ✅ Step: order_placed
   ⚠️  Step: payment_validated (skipped payment_validation)
   Expected: Invalid transition violation logged asynchronously

   ✅ PASS: Invalid transition should be logged

============================================================
✅ All Invalid Transition Tests Completed!

Note: Violations are logged asynchronously.
Check server logs or validation_results table for details.
```

---

## Verifying Violations

### 1. Server Logs
Check the Go server console for validation output:
```
[ASYNC-VALIDATION] Starting validation for thread=thread-123
[CRITICAL-VIOLATION] Found violation: type=invalid_transition
[LUA-SUCCESS] Step state updated, final status: violated
```

### 2. Postgres Database
Query the `validation_results` table:
```sql
SELECT 
    thread_id,
    step_name,
    violation_type,
    severity,
    message,
    created_at
FROM validation_results
WHERE thread_id = 'thread-abc123'
ORDER BY created_at DESC;
```

### 3. Valkey/Redis Streams
Check the validation stream:
```bash
redis-cli XREAD COUNT 10 STREAMS streams:thread_validations 0
```

---

## Test Coverage Summary

| Validation Type | Test File | Test Cases | Severity | Status |
|----------------|-----------|------------|----------|--------|
| Invalid Transition | `e2e-invalid-transition.test.js` | 6 | CRITICAL | ✅ New |
| Step Timeout | `e2e-step-timeout.test.js` | 8 | CRITICAL | ✅ New |
| Max Duration | `e2e-max-duration.test.js` | 9 | CRITICAL | ✅ New |
| Multiple Terminals | `e2e-multiple-terminals.test.js` | 7 | CRITICAL | ✅ New |
| Retry Limit | `e2e-retry-limit.test.js` | 5+ | CRITICAL | ✅ Exists |
| Missing Optional | `e2e-missing-optional-fields.test.js` | 10 | INFO | ✅ New |
| Extra Fields | `e2e-extra-fields.test.js` | 11 | INFO | ✅ New |

**Total Test Cases:** 56+ comprehensive scenarios

---

## Edge Cases Covered

### Timing Edge Cases
- ✅ Exactly at timeout/limit
- ✅ Just over timeout/limit (1s/1h)
- ✅ Significantly over limits
- ✅ Well within limits

### Workflow Edge Cases
- ✅ Multiple violations in same thread
- ✅ Multiple violations in same step
- ✅ Valid paths (control tests)
- ✅ Alternative valid paths
- ✅ Terminal state transitions
- ✅ Retry scenarios

### Data Edge Cases
- ✅ Missing all optional fields
- ✅ Missing some optional fields
- ✅ Empty string values
- ✅ Extra fields with special characters
- ✅ Fields with similar names
- ✅ Mix of violations

---

## Troubleshooting

### Connection Refused
- Ensure server is running on port 8081
- Check `config/config.yaml` for correct port

### Contract Not Found
- Verify `examples/contract_example.yaml` exists
- Check contract structure (entry_points, transitions, etc.)

### No Violations Logged
- Wait 2-3 seconds after step submission (async processing)
- Check server logs for async validation output
- Verify archiver is running (optional but recommended)

### Tests Failing
- Ensure clean database state
- Check Valkey/Redis is running
- Verify contract matches test expectations

---

## Architecture Notes

### Async Validation Flow
```
Step Submission → Blocking Validations → Record Step → Return Success
                                              ↓
                                    Async Goroutine
                                              ↓
                         Non-Blocking Validations
                                              ↓
                         Store in Valkey Stream
                                              ↓
                         Update Step State (Lua)
                                              ↓
                         Archiver → Postgres
```

### Key Implementation Files
- **Validation Logic**: `/internal/service/validation_service.go`
- **Notification Service**: `/internal/service/notification_service.go`
- **Step Event Service**: `/internal/service/step_event.go`
- **Archiver**: `/cmd/archiver/main.go`
- **Models**: `/internal/models/validation.go`

---

## Contributing

When adding new validation tests:

1. **Follow naming convention**: `e2e-{validation-type}.test.js`
2. **Include edge cases**: At least 5-10 test scenarios
3. **Add control tests**: Valid scenarios that should NOT trigger violations
4. **Document expected behavior**: Clear comments on what should happen
5. **Test both success and failure**: Positive and negative cases
6. **Update this README**: Add new test to the summary table

---

## Related Documentation

- **Main Documentation**: `/WhatIsThreadify.md`
- **Contract Examples**: `/threadify-go/examples/`
- **SDK Documentation**: `/threadify-sdk/Documentation.md`
- **Blocking Validation Tests**: `/threadify-sdk/tests/e2e-validation.test.js`

---

## Summary

This comprehensive test suite validates all 7 non-blocking validation types with **56+ test scenarios** covering:
- ✅ Critical violations (invalid transitions, timeouts, limits)
- ✅ Info-level notifications (missing optional, extra fields)
- ✅ Edge cases (exact limits, just over, significantly over)
- ✅ Multiple violations per thread
- ✅ Different workflow paths
- ✅ Control tests (valid scenarios)

All tests are designed to be **independent**, **repeatable**, and **comprehensive** without duplicating existing test coverage.
