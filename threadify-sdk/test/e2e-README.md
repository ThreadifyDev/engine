# E2E Validation Tests (Using Threadify SDK)

End-to-end tests for validating thread step submission blocking validations using the official Threadify SDK.

## What's Tested

1. **Entry Point Validation** - Thread must start with an entry point step
2. **Step Exists** - Step must be defined in contract
3. **Required Business Context** - All required context fields must be provided
4. **Idempotency** - Duplicate successful steps are rejected
5. **Role Validation** - User must have required role for step
6. **Access Control** - User must have write permission to thread

## Prerequisites

1. **Server Running**
   ```bash
   cd ../threadify-go
   go run cmd/server/main.go
   ```

2. **Contract File**
   - Ensure `../threadify-go/examples/contract_example.yaml` exists
   - Contract should have:
     - `entry_points: [order_placed]`
     - Steps with required business context
     - Transitions defined

## Running the Tests

```bash
cd threadify-sdk/test
node e2e-validation.test.js
```

The test uses the default test API key (`api-key-123`). You can override it with:
```bash
export API_KEY="your-api-key-here"
node e2e-validation.test.js
```

**Note:** The test uses the SDK's existing dependencies. Install `ws` if needed: `npm install ws`

## Expected Output

```
🚀 Starting E2E Validation Tests

✅ Logged in successfully
✅ Contract uploaded successfully
✅ WebSocket connected
✅ WebSocket authenticated

📋 Test 1: Entry Point Validation
  Thread created: thread-xxx
  ✅ PASS: Non-entry-point step rejected
     Error: Thread must start with one of the entry points: [order_placed]
  ✅ PASS: Entry point step accepted

📋 Test 2: Step Exists in Contract
  ✅ PASS: Non-existent step rejected
     Error: Step 'nonexistent_step' not found in contract

📋 Test 3: Required Business Context
  ✅ PASS: Missing required context rejected
     Error: required context field 'order_id' is missing
  ✅ PASS: Valid context accepted

📋 Test 4: Idempotency Validation
  ✅ First submission successful
  ✅ PASS: Duplicate step rejected
     Error: Step with this signature already successful
  ✅ PASS: Different idempotency key accepted

📋 Test 5: Completed Thread Validation
  ⚠️  Note: This test requires manually completing a thread

✅ All tests completed!
```

## Test Details

### Test 1: Entry Point Validation
- Creates a new thread
- Attempts to submit `shipped` step (not an entry point) → Should FAIL
- Submits `order_placed` step (valid entry point) → Should PASS

### Test 2: Step Exists in Contract
- Creates a new thread
- Attempts to submit `nonexistent_step` → Should FAIL with "not found in contract"

### Test 3: Required Business Context
- Creates a new thread
- Submits `order_placed` without required fields → Should FAIL
- Submits `order_placed` with all required fields → Should PASS

### Test 4: Idempotency Validation
- Creates a new thread
- Submits step with idempotency key `duplicate-key` → Should PASS
- Submits same step with same key → Should FAIL (duplicate)
- Submits same step with different key → Should PASS

### Test 5: Completed Thread Validation
- Manual test (requires completing a thread first)
- Attempting to add steps to completed thread → Should FAIL

## Troubleshooting

**Connection Refused:**
- Ensure server is running on port 8081
- Check server logs for errors

**Contract Not Found:**
- Verify `examples/contract_example.yaml` exists
- Check contract has correct structure (entry_points, transitions, etc.)

**Authentication Failed:**
- Server may require different credentials
- Check server configuration

**WebSocket Timeout:**
- Increase timeout in test file (currently 5000ms)
- Check server WebSocket endpoint is accessible
