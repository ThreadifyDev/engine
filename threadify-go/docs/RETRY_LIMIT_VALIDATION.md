# Retry Limit Validation Implementation

## Overview
Implemented the retry limit exceeded validation as a non-blocking critical validation that tracks when a step has been retried more times than allowed by the contract configuration.

## Implementation Details

### 1. Validation Function
**Location**: `/threadify-go/internal/service/thread_validation.go`

**Function**: `checkRetryLimit()`
- Checks if a step has exceeded its configured retry limit
- Uses the step state hash to retrieve the current retry count
- Compares against `max_retries` defined in contract transitions
- Returns a critical violation if limit is exceeded

**Key Logic**:
```go
func (s *ThreadService) checkRetryLimit(
    ctx context.Context,
    threadID string,
    stepName string,
    idempotencyKey string,
    graph *models.ContractGraph,
) *models.ValidationViolation
```

1. Finds the transition configuration for the current step
2. Checks if `can_retry: true` and `max_retries` is set
3. Retrieves retry count from step state hash: `thread:{threadID}:steps:{stepName}:{idempKey}`
4. Compares retry count against max_retries
5. Returns violation if exceeded

### 2. Retry Tracking Mechanism
**Location**: `/threadify-go/internal/service/lua/update_step_state.lua`

The Lua script automatically tracks retries:
- When a step hash already exists, it increments `retryCount`
- First attempt sets `retryCount: 0`
- Each subsequent attempt increments the counter

**Step State Hash Fields**:
- `status`: Current step status
- `retryCount`: Number of times step has been retried
- `firstSeenAt`: Timestamp of first attempt
- `lastUpdatedAt`: Timestamp of last update
- `latestStepID`: Most recent step ID
- `previousStep`: Previous step in the workflow

### 3. Contract Configuration
**Location**: `/threadify-go/examples/contract_example.yaml`

**Example Configuration**:
```yaml
transitions:
  - from: payment_validation
    to:
      - payment_validated
      - order_cancelled
    can_retry: true
    max_retries: 3
    
  - from: delivery_failed
    to:
      - shipped
    can_retry: true
    max_retries: 2
```

**Fields**:
- `can_retry`: Boolean indicating if retries are allowed
- `max_retries`: Maximum number of retry attempts allowed

### 4. Validation Integration
**Location**: `/threadify-go/internal/service/thread_validation.go` (lines 524-544)

The validation is executed as part of the async non-blocking validations:
- Runs after step is successfully recorded
- Generates a **Critical** severity notification if violated
- Does NOT block step submission
- Results stored in validation streams

### 5. Violation Details
**ViolationType**: `retry_limit_exceeded`
**Severity**: `critical`

**Notification Fields**:
- `message`: "Step '{name}' exceeded retry limit of {max} (current: {count} retries)"
- `details`:
  - `step_name`: Name of the step
  - `retry_count`: Current retry count
  - `max_retries`: Maximum allowed retries

## Testing

### Test File
**Location**: `/threadify-sdk/test/e2e-retry-limit.test.js`

### Test Cases

#### 1. Retry Limit Exceeded
- Creates a thread with `payment_validation` step
- Retries the step 5 times (exceeds max_retries=3)
- Verifies violation is recorded after 3rd retry
- Uses fixed idempotency key to track same step

#### 2. Retry Within Limit
- Creates a thread with `delivery_failed` -> `shipped` retry flow
- Retries 2 times (within max_retries=2)
- Verifies no violation is generated
- Confirms successful retry tracking

### Running Tests
```bash
# Start the server
make start

# Run retry limit tests
cd threadify-sdk
node test/e2e-retry-limit.test.js
```

## How It Works

### Retry Flow
1. **First Attempt**: Step recorded with `retryCount: 0`
2. **Retry Attempt**: Same step with same idempotency key
   - Lua script detects existing step hash
   - Increments `retryCount`
   - Updates step state
3. **Async Validation**: 
   - Checks retry count against max_retries
   - Generates violation if exceeded
   - Stores notification in stream

### Idempotency Key Importance
- Retries MUST use the same idempotency key
- Different idempotency keys = different step instances
- Retry count is tracked per unique `stepName:idempKey` combination

## Data Storage

### Step State Hash
**Key**: `thread:{threadID}:steps:{stepName}:{idempKey}`
```
status: "completed"
retryCount: "3"
firstSeenAt: "2024-01-01T10:00:00Z"
lastUpdatedAt: "2024-01-01T10:05:00Z"
latestStepID: "step-uuid-123"
previousStep: "order_placed:idemp-1"
```

### Validation Notification Stream
**Stream**: `streams:validation_notifications`
```json
{
  "notificationId": "notif-uuid",
  "threadId": "thread-123",
  "stepId": "step-456",
  "stepName": "payment_validation",
  "violationType": "retry_limit_exceeded",
  "severity": "critical",
  "message": "Step 'payment_validation' exceeded retry limit of 3 (current: 4 retries)",
  "details": {
    "step_name": "payment_validation",
    "retry_count": 4,
    "max_retries": 3
  }
}
```

## Benefits

1. **Non-Blocking**: Steps are recorded even if retry limit exceeded
2. **Visibility**: Violations tracked in validation streams
3. **Configurable**: Per-transition retry limits
4. **Accurate**: Uses step state hash for precise tracking
5. **Real-time**: Async validation provides immediate feedback

## Future Enhancements

1. **Retry Backoff**: Add exponential backoff configuration
2. **Retry Windows**: Time-based retry limits (e.g., max 3 retries per hour)
3. **Retry Policies**: Different policies for different failure types
4. **Retry Analytics**: Dashboard showing retry patterns and trends
5. **Auto-Escalation**: Automatically escalate after N retries

## Related Files

- `/threadify-go/internal/service/thread_validation.go` - Validation logic
- `/threadify-go/internal/service/lua/update_step_state.lua` - Retry tracking
- `/threadify-go/internal/models/contract_graph.go` - Contract structures
- `/threadify-go/internal/models/validation.go` - Validation types
- `/threadify-go/examples/contract_example.yaml` - Example configuration
- `/threadify-sdk/test/e2e-retry-limit.test.js` - E2E tests
