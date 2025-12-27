# Contract Structure V3 Specification

## Overview

This document defines the structure and validation rules for Threadify contracts version 3. Contracts define high-level business processes with multiple parties, steps, and state transitions.

## Contract Schema

### Top-Level Fields

```yaml
contract_name: string       # Unique identifier for the contract
version: integer           # Contract schema version (currently 3)
description: string        # Human-readable description of the business process
parties: array            # List of participating parties
steps: array              # Ordered list of process steps
transitions: array        # Valid state transitions between steps
terminal_steps: array     # Steps that mark process completion
validation: object        # Contract-level validation rules
```

## Example Contract: Product Delivery

```yaml
contract_name: product_delivery
version: 3
description: High-level business process for product delivery

parties:
  - merchant
  - payment_processor
  - warehouse_manager
  - logistics_carrier

steps:
  - id: order_placed
    owner: merchant
    timeout: 30s
    business_context:
      required:
        - order_id
        - customer_id
        - total_amount
      optional:
        - customer_notes
    
  - id: payment_validated
    owner: payment_processor
    timeout: 5s
    business_context:
      required:
        - transaction_id
      optional:
        - validation_method
        - fraud_score
  
  - id: payment_failed
    owner: payment_processor
    business_context:
      required:
        - failure_reason
  
  - id: fulfillment_ready
    owner: warehouse_manager
    timeout: 10s
    business_context:
      required:
        - warehouse_id
  
  - id: package_shipped
    owner: logistics_carrier
    timeout: 24h
    business_context:
      required:
        - tracking_number
        - carrier
  
  - id: delivered
    owner: logistics_carrier
    business_context:
      required:
        - delivery_timestamp
      optional:
        - signature_captured
  
  - id: order_cancelled
    owner: merchant
    business_context:
      required:
        - cancellation_reason

transitions:
  - from: order_placed
    to: payment_validated
    
  - from: payment_validated
    to: 
      - fulfillment_ready
      - payment_failed
    
  - from: payment_failed
    to: order_cancelled
    can_retry: true
    max_retries: 3
    retry_leads_to: payment_validated
    
  - from: fulfillment_ready
    to: 
      - package_shipped
      - order_cancelled
    
  - from: package_shipped
    to:
      - delivered
      - order_cancelled
    can_retry: true
    max_retries: 1

terminal_steps:
  - delivered
  - order_cancelled

validation:
  max_duration: 72h
  allow_multiple_terminals: true
  multiple_terminals_severity: minor
```

---

## Field Definitions

### Parties

**Type:** `array of strings`

**Description:** List of all parties participating in the contract. Each party represents a distinct actor or role in the business process.

**Example:**

```yaml
parties:
  - merchant
  - payment_processor
  - warehouse_manager
  - logistics_carrier
```

---

### Steps

**Type:** `array of Step objects`

**Description:** Defines all possible states in the business process workflow.

#### Step Object

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `id` | string | Yes | Unique identifier for the step |
| `owner` | string | Yes | Party responsible for this step (must be in `parties`) |
| `timeout` | duration | No | Maximum time allowed for this step (e.g., `30s`, `5m`, `24h`) |
| `business_context` | object | No | Business data fields relevant to this step |
| `business_context.required` | array of strings | No | Required fields that must be present in step data |
| `business_context.optional` | array of strings | No | Optional fields that may be present in step data |

**Example:**

```yaml
steps:
  - id: order_placed
    owner: merchant
    timeout: 30s
    business_context:
      required:
        - order_id
        - customer_id
        - total_amount
      optional:
        - customer_notes
        - promotional_code
```

---

### Transitions

**Type:** `array of Transition objects`

**Description:** Defines valid state transitions between steps, including retry logic.

#### Transition Object

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `from` | string | Yes | Source step ID (must exist in `steps`) |
| `to` | string or array of strings | Yes | Target step ID(s) (must exist in `steps`) |
| `can_retry` | boolean | No | Whether this transition supports retry logic |
| `max_retries` | integer | No | Maximum number of retry attempts (requires `can_retry: true`) |
| `retry_leads_to` | string | No | Step to transition to on successful retry (must exist in `steps`) |

**Example:**

```yaml
transitions:
  - from: payment_failed
    to: order_cancelled
    can_retry: true
    max_retries: 3
    retry_leads_to: payment_validated
```

---

### Terminal Steps

**Type:** `array of strings`

**Description:** Steps that represent the end of a contract execution. Once a terminal step is reached, no further transitions are allowed.

**Example:**

```yaml
terminal_steps:
  - delivered
  - order_cancelled
```

---

### Validation

**Type:** `object`

**Description:** Contract-level validation and constraint rules.

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `max_duration` | duration | No | Maximum allowed duration for contract execution |
| `allow_multiple_terminals` | boolean | No | Whether a contract can reach multiple terminal states |
| `multiple_terminals_severity` | string | No | Severity level if multiple terminals reached: `minor`, `warning`, `error` |

**Example:**

```yaml
validation:
  max_duration: 72h
  allow_multiple_terminals: true
  multiple_terminals_severity: minor
```

---

## Contract Validation Rules

When a contract is uploaded, Threadify performs the following validations:

### 1. Terminal Steps Must Exist in Steps

All steps listed in `terminal_steps` must be defined in the `steps` array.

**❌ Invalid:**

```yaml
steps:
  - id: delivered
    owner: logistics_carrier

terminal_steps:
  - completed  # ERROR: "completed" not defined in steps
```

**✅ Valid:**

```yaml
steps:
  - id: delivered
    owner: logistics_carrier

terminal_steps:
  - delivered  # OK: "delivered" is defined
```

---

### 2. Terminal Steps Must Be Reachable

All terminal steps must appear in at least one transition's `to` field, ensuring they can be reached during contract execution.

**❌ Invalid:**

```yaml
steps:
  - id: mysterious_end
    owner: merchant

transitions:
  - from: order_placed
    to: payment_validated

terminal_steps:
  - mysterious_end  # ERROR: Never appears in any transition's "to" field
```

**✅ Valid:**

```yaml
steps:
  - id: delivered
    owner: logistics_carrier

transitions:
  - from: package_shipped
    to: delivered  # OK: "delivered" is reachable

terminal_steps:
  - delivered
```

---

### 3. All Steps Referenced in Transitions Must Be Defined

Both `from` and `to` fields in transitions must reference steps that exist in the `steps` array.

**❌ Invalid:**

```yaml
steps:
  - id: order_placed
    owner: merchant

transitions:
  - from: order_placed
    to: undefined_step  # ERROR: "undefined_step" not in steps
```

**✅ Valid:**

```yaml
steps:
  - id: order_placed
    owner: merchant
  - id: payment_validated
    owner: payment_processor

transitions:
  - from: order_placed
    to: payment_validated  # OK: Both steps are defined
```

---

### 4. All Parties Referenced in Step Owners Must Be Defined

The `owner` field in each step must reference a party listed in the `parties` array.

**❌ Invalid:**

```yaml
parties:
  - merchant
  - payment_processor

steps:
  - id: order_placed
    owner: unknown_party  # ERROR: "unknown_party" not in parties
```

**✅ Valid:**

```yaml
parties:
  - merchant
  - payment_processor

steps:
  - id: order_placed
    owner: merchant  # OK: "merchant" is defined in parties
```

---

### 5. Retry Transitions Must Reference Valid Steps

When `can_retry: true` is set, the `retry_leads_to` field must reference a step that exists in the `steps` array.

**❌ Invalid:**

```yaml
steps:
  - id: payment_failed
    owner: payment_processor

transitions:
  - from: payment_failed
    to: order_cancelled
    can_retry: true
    retry_leads_to: nonexistent_step  # ERROR: "nonexistent_step" not in steps
```

**✅ Valid:**

```yaml
steps:
  - id: payment_failed
    owner: payment_processor
  - id: payment_validated
    owner: payment_processor

transitions:
  - from: payment_failed
    to: order_cancelled
    can_retry: true
    max_retries: 3
    retry_leads_to: payment_validated  # OK: "payment_validated" is defined
```

---

### 6. Business Context Structure Must Be Valid

If `business_context` is provided, it must be an object with optional `required` and/or `optional` arrays containing strings.

**❌ Invalid:**

```yaml
steps:
  - id: order_placed
    owner: merchant
    business_context: "invalid_string"  # ERROR: Must be an object
```

```yaml
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required: "not_an_array"  # ERROR: Must be an array
```

**✅ Valid:**

```yaml
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id
        - customer_id
      optional:
        - customer_notes
```

```yaml
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id  # OK: Only required fields specified
```

```yaml
steps:
  - id: order_placed
    owner: merchant
    business_context:
      optional:
        - customer_notes  # OK: Only optional fields specified
```

---

## Validation Error Messages

When validation fails, Threadify returns structured error messages:

```json
{
  "valid": false,
  "errors": [
    {
      "type": "terminal_step_not_defined",
      "message": "Terminal step 'completed' is not defined in steps array",
      "field": "terminal_steps[0]",
      "value": "completed"
    },
    {
      "type": "step_not_reachable",
      "message": "Terminal step 'mysterious_end' is not reachable from any transition",
      "field": "terminal_steps[1]",
      "value": "mysterious_end"
    }
  ]
}
```

---

## Workflow Validation (Runtime)

During thread execution, Threadify performs real-time validation to ensure the workflow adheres to the contract. Violations are categorized by severity level.

### Violation Severity Levels

| Level | Description | Action |
| ----- | ----------- | ------ |
| **Critical** | Violations that break contract integrity and must halt execution | Thread execution fails immediately |
| **Major** | Violations that indicate data or timing issues requiring attention | Thread execution fails immediately |
| **Minor** | Violations that indicate potential issues but don't break the contract | Logged and tracked; execution continues |
| **Info** | Informational notices for tracking and auditing purposes | Logged only; no impact on execution |

---

### Critical Violations

Critical violations immediately halt thread execution and return an error response. These represent fundamental contract integrity issues.

#### 1. Invalid Transition

**Description:** Attempting to transition to a step that is not allowed by the contract's transition rules.

**Example:**

```yaml
# Contract defines:
transitions:
  - from: order_placed
    to: payment_validated

# Violation: Attempting to go from order_placed to package_shipped
```

**Error Response:**

```json
{
  "error": "invalid_transition",
  "severity": "critical",
  "message": "Invalid transition from 'order_placed' to 'package_shipped'",
  "current_step": "order_placed",
  "attempted_step": "package_shipped",
  "allowed_steps": ["payment_validated"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

#### 2. Unauthorized Owner

**Description:** A party attempts to execute a step they don't own according to the contract.

**Example:**

```yaml
# Contract defines:
steps:
  - id: payment_validated
    owner: payment_processor

# Violation: merchant attempts to execute payment_validated
```

**Error Response:**

```json
{
  "error": "unauthorized_owner",
  "severity": "critical",
  "message": "Party 'merchant' is not authorized to execute step 'payment_validated'",
  "step_id": "payment_validated",
  "expected_owner": "payment_processor",
  "attempted_by": "merchant",
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

#### 3. Contract Not Found

**Description:** Attempting to execute a thread with a contract that doesn't exist or has been deleted.

**Example:**

```javascript
thread.create({
  contract_name: "nonexistent_contract",
  contract_version: 3
})
```

**Error Response:**

```json
{
  "error": "contract_not_found",
  "severity": "critical",
  "message": "Contract 'nonexistent_contract' version 3 not found",
  "contract_name": "nonexistent_contract",
  "contract_version": 3,
  "timestamp": "2025-12-27T16:39:00Z"
}
```

**Behavior:**

- Thread creation fails immediately
- Contract must exist and be validated before thread execution

---

### Major Violations

Major violations immediately halt thread execution and return an error response. These represent data integrity or timing constraint violations.

#### 1. Missing Required Field

**Description:** A step's required `business_context` field is missing from the step data.

**Example:**

```yaml
# Contract defines:
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id
        - customer_id
        - total_amount

# Step data provided:
{
  "step_id": "order_placed",
  "data": {
    "order_id": "ORD-123",
    "customer_id": "CUST-456"
    // Missing required field: total_amount
  }
}
```

**Error Response:**

```json
{
  "error": "missing_required_field",
  "severity": "major",
  "message": "Step 'order_placed' is missing required business_context field 'total_amount'",
  "step_id": "order_placed",
  "missing_required_fields": ["total_amount"],
  "provided_fields": ["order_id", "customer_id"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

**Behavior:**

- Thread execution fails immediately
- All required fields must be present for step to be accepted
- Optional fields can be omitted without error

---

#### 2. Max Duration Exceeded

**Description:** The entire thread execution has exceeded the contract's `max_duration`.

**Example:**

```yaml
# Contract defines:
validation:
  max_duration: 72h

# Violation: Thread has been running for 73 hours
```

**Error Response:**

```json
{
  "error": "max_duration_exceeded",
  "severity": "major",
  "message": "Thread execution exceeded maximum duration of 72h",
  "max_duration": "72h",
  "elapsed": "73h",
  "thread_id": "thread_abc123",
  "started_at": "2025-12-24T16:39:00Z",
  "timestamp": "2025-12-27T17:39:00Z"
}
```

**Behavior:**

- Thread is marked as `failed` with reason `max_duration_exceeded`
- Monitoring begins when thread is created
- No further transitions allowed

---

#### 3. Step Timeout Exceeded

**Description:** A step has exceeded its maximum allowed duration as defined in the contract.

**Example:**

```yaml
# Contract defines:
steps:
  - id: order_placed
    owner: merchant
    timeout: 30s

# Violation: Step took 45 seconds to complete
```

**Error Response:**

```json
{
  "error": "step_timeout_exceeded",
  "severity": "major",
  "message": "Step 'order_placed' exceeded timeout of 30s",
  "step_id": "order_placed",
  "timeout": "30s",
  "elapsed": "45s",
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

**Behavior:**

- Thread is marked as `failed` with reason `timeout`
- Timeout monitoring begins when step is entered
- Can trigger automatic retry if configured in transitions

---

#### 4. Terminal Step Violation

**Description:** Attempting to transition from a terminal step (terminal steps should have no outgoing transitions).

**Example:**

```yaml
# Contract defines:
terminal_steps:
  - delivered

# Violation: Attempting to transition from delivered to another step
```

**Error Response:**

```json
{
  "error": "terminal_step_violation",
  "severity": "critical",
  "message": "Cannot transition from terminal step 'delivered'",
  "step_id": "delivered",
  "attempted_transition_to": "refund_initiated",
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

#### 5. Max Retries Exceeded

**Description:** A retry transition has exceeded its maximum retry count.

**Example:**

```yaml
# Contract defines:
transitions:
  - from: payment_failed
    to: order_cancelled
    can_retry: true
    max_retries: 3
    retry_leads_to: payment_validated

# Violation: 4th retry attempt
```

**Error Response:**

```json
{
  "error": "max_retries_exceeded",
  "severity": "critical",
  "message": "Maximum retries (3) exceeded for transition from 'payment_failed'",
  "step_id": "payment_failed",
  "max_retries": 3,
  "retry_count": 4,
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

### Minor Violations

Minor violations are logged and tracked but do not halt execution. They indicate potential issues that should be reviewed.

#### 1. Multiple Terminal States

**Description:** A thread has reached multiple terminal states, which may indicate a workflow design issue.

**Example:**

```yaml
# Contract defines:
terminal_steps:
  - delivered
  - order_cancelled

validation:
  allow_multiple_terminals: true
  multiple_terminals_severity: minor

# Thread reaches both delivered and order_cancelled
```

**Warning Response:**

```json
{
  "warning": "multiple_terminal_states",
  "severity": "minor",
  "message": "Thread has reached multiple terminal states",
  "terminal_states_reached": ["delivered", "order_cancelled"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

**Behavior:**

- Execution continues
- Warning is logged to thread history
- Can be configured to escalate to `warning` or `error` severity via `multiple_terminals_severity`

---

#### 2. Retry Attempted

**Description:** A retry transition is being executed (informational tracking).

**Example:**

```yaml
# Contract defines:
transitions:
  - from: payment_failed
    to: order_cancelled
    can_retry: true
    max_retries: 3
    retry_leads_to: payment_validated

# Retry is triggered
```

**Warning Response:**

```json
{
  "warning": "retry_attempted",
  "severity": "minor",
  "message": "Retry transition executed from 'payment_failed' to 'payment_validated'",
  "from_step": "payment_failed",
  "to_step": "payment_validated",
  "retry_count": 1,
  "max_retries": 3,
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

### Info Violations

Info-level violations are purely for tracking and auditing. They have no impact on execution.

#### 1. Missing Optional Field

**Description:** A step's optional `business_context` field is missing from the step data. This is informational only since optional fields are not required.

**Example:**

```yaml
# Contract defines:
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id
        - customer_id
        - total_amount
      optional:
        - customer_notes
        - promotional_code

# Step data provided:
{
  "step_id": "order_placed",
  "data": {
    "order_id": "ORD-123",
    "customer_id": "CUST-456",
    "total_amount": 99.99
    // Missing optional fields: customer_notes, promotional_code
  }
}
```

**Info Response:**

```json
{
  "info": "missing_optional_field",
  "severity": "info",
  "message": "Step 'order_placed' is missing optional business_context fields",
  "step_id": "order_placed",
  "missing_optional_fields": ["customer_notes", "promotional_code"],
  "provided_fields": ["order_id", "customer_id", "total_amount"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

**Behavior:**

- Execution continues normally
- Info logged for data completeness tracking
- Can be used to identify opportunities for richer data collection

---

#### 2. Extra Undocumented Field

**Description:** Step data includes fields not defined in the contract's `business_context` (neither required nor optional).

**Example:**

```yaml
# Contract defines:
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id
        - customer_id
      optional:
        - customer_notes

# Step data provided:
{
  "step_id": "order_placed",
  "data": {
    "order_id": "ORD-123",
    "customer_id": "CUST-456",
    "extra_field": "unexpected_value"  // Not in required or optional
  }
}
```

**Info Response:**

```json
{
  "info": "extra_undocumented_field",
  "severity": "info",
  "message": "Step 'order_placed' contains undocumented fields",
  "step_id": "order_placed",
  "extra_fields": ["extra_field"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

#### 3. Step Completed Under Timeout

**Description:** Informational tracking of step completion times relative to timeout.

**Example:**

```yaml
# Contract defines:
steps:
  - id: package_shipped
    owner: logistics_carrier
    timeout: 24h

# Step completed in 2 hours
```

**Info Response:**

```json
{
  "info": "step_completed",
  "severity": "info",
  "message": "Step 'package_shipped' completed successfully",
  "step_id": "package_shipped",
  "timeout": "24h",
  "elapsed": "2h",
  "utilization": "8.3%",
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

## Validation Response Format

All validation responses during thread execution follow a consistent structure:

```json
{
  "validation_result": "success" | "warning" | "failure",
  "violations": [
    {
      "type": "string",
      "severity": "critical" | "minor" | "info",
      "message": "string",
      "step_id": "string",
      "thread_id": "string",
      "timestamp": "ISO8601",
      "details": {}
    }
  ],
  "thread_status": "active" | "failed" | "completed",
  "current_step": "string"
}
```

### Validation Result Types

- **success**: No violations detected, execution continues
- **warning**: Minor or info violations detected, execution continues
- **failure**: Critical or major violation detected, execution halted

---

## WebSocket Events

Threadify emits real-time WebSocket events for all violations during thread execution. Clients can subscribe to these events for immediate notification.

### Event: `thread_violation`

Emitted whenever a violation occurs during thread execution.

#### Event Structure

```json
{
  "event": "thread_violation",
  "thread_id": "string",
  "violation_type": "string",
  "severity": "critical" | "major" | "minor" | "info",
  "step_id": "string",
  "message": "string",
  "timestamp": "ISO8601",
  "details": {}
}
```

### Example Events

#### Missing Required Field (Major)

```json
{
  "event": "thread_violation",
  "thread_id": "thread_123",
  "violation_type": "missing_required_field",
  "severity": "major",
  "step_id": "package_shipped",
  "message": "Step 'package_shipped' is missing required business_context field 'tracking_number'",
  "missing_fields": ["tracking_number"],
  "provided_fields": ["carrier", "internal_shipping_id"],
  "timestamp": "2025-12-27T16:39:00Z"
}
```

#### Invalid Transition (Critical)

```json
{
  "event": "thread_violation",
  "thread_id": "thread_123",
  "violation_type": "invalid_transition",
  "severity": "critical",
  "step_id": "order_placed",
  "message": "Invalid transition from 'order_placed' to 'package_shipped'",
  "current_step": "order_placed",
  "attempted_step": "package_shipped",
  "allowed_steps": ["payment_validated"],
  "timestamp": "2025-12-27T16:39:00Z"
}
```

#### Step Timeout Exceeded (Major)

```json
{
  "event": "thread_violation",
  "thread_id": "thread_123",
  "violation_type": "step_timeout_exceeded",
  "severity": "major",
  "step_id": "payment_validated",
  "message": "Step 'payment_validated' exceeded timeout of 5s",
  "timeout": "5s",
  "elapsed": "7s",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

#### Multiple Terminal States (Minor)

```json
{
  "event": "thread_violation",
  "thread_id": "thread_123",
  "violation_type": "multiple_terminal_states",
  "severity": "minor",
  "message": "Thread has reached multiple terminal states",
  "terminal_states_reached": ["delivered", "order_cancelled"],
  "timestamp": "2025-12-27T16:39:00Z"
}
```

#### Missing Optional Field (Info - Optional)

```json
{
  "event": "thread_violation",
  "thread_id": "thread_123",
  "violation_type": "missing_optional_field",
  "severity": "info",
  "step_id": "package_shipped",
  "message": "Step 'package_shipped' is missing optional business_context fields",
  "missing_optional_fields": ["estimated_delivery", "carrier_contact"],
  "provided_fields": ["tracking_number", "carrier"],
  "timestamp": "2025-12-27T16:39:00Z"
}
```

### Subscribing to Violation Events

```javascript
// SDK example
const thread = await client.thread("thread_123");

thread.on("violation", (event) => {
  console.log(`Violation: ${event.severity} - ${event.message}`);
  
  if (event.severity === "critical" || event.severity === "major") {
    // Handle execution-blocking violations
    alert(`Thread failed: ${event.message}`);
  } else if (event.severity === "minor") {
    // Log warnings
    console.warn(`Warning: ${event.message}`);
  } else {
    // Track info violations for analytics
    analytics.track("thread_info_violation", event);
  }
});
```

---

## Monitoring and Alerting

### Recommended Monitoring

1. **Critical Violation Rate**
   - Track frequency of critical violations by type
   - Alert on unusual spikes in invalid transitions or wrong owner errors

2. **Timeout Patterns**
   - Monitor which steps frequently timeout
   - Identify bottlenecks in the workflow

3. **Retry Success Rate**
   - Track how often retries succeed vs. exhaust max_retries
   - Optimize retry strategies based on data

4. **Data Quality Metrics**
   - Track missing_required_field critical violations (data integrity issues)
   - Track missing_optional_field info violations (data completeness opportunities)
   - Monitor extra_undocumented_field violations (potential contract updates needed)
   - Improve data collection processes

### Audit Trail

All violations (critical, minor, and info) are stored in the thread's audit trail:

```json
{
  "thread_id": "thread_abc123",
  "contract_name": "product_delivery",
  "contract_version": 3,
  "audit_trail": [
    {
      "timestamp": "2025-12-27T16:39:00Z",
      "event_type": "step_completed",
      "step_id": "order_placed",
      "party": "merchant",
      "violations": []
    },
    {
      "timestamp": "2025-12-27T16:39:30Z",
      "event_type": "validation_warning",
      "step_id": "payment_validated",
      "party": "payment_processor",
      "violations": [
        {
          "type": "missing_optional_field",
          "severity": "info",
          "details": {"missing_optional_fields": ["fraud_score", "risk_level"]}
        }
      ]
    }
  ]
}
```

---

## Duration Format

Timeouts and durations use the following format:

| Unit | Suffix | Example |
| ---- | ------ | ------- |
| Seconds | `s` | `30s`, `45s` |
| Minutes | `m` | `5m`, `15m` |
| Hours | `h` | `2h`, `24h` |
| Days | `d` | `3d`, `7d` |

**Examples:**

- `timeout: 30s` - 30 seconds
- `timeout: 5m` - 5 minutes
- `timeout: 24h` - 24 hours
- `max_duration: 72h` - 72 hours (3 days)

---

## Implementation Notes

### For Backend Implementation

1. **Contract Upload Endpoint:** `/api/v3/contracts`

   - Accepts YAML or JSON contract definitions
   - Performs all validation rules before storage
   - Returns validation errors with specific field references

2. **Validation Order:**

   - Parse contract structure
   - Validate required fields
   - Validate party references
   - Validate step definitions
   - Validate transition references
   - Validate terminal step existence and reachability
   - Validate retry logic references

3. **Storage:**
   - Store validated contracts in database
   - Index by `contract_name` and `version`
   - Support contract versioning for evolution

### For SDK Implementation

1. **Contract Builder API:**

   ```javascript
   const contract = new ContractBuilder('product_delivery')
     .version(3)
     .description('High-level business process for product delivery')
     .addParty('merchant')
     .addParty('payment_processor')
     .addStep({
       id: 'order_placed',
       owner: 'merchant',
       timeout: '30s',
       businessContext: ['order_id', 'customer_id', 'total_amount']
     })
     .addTransition({
       from: 'order_placed',
       to: 'payment_validated'
     })
     .addTerminalStep('delivered')
     .build();
   ```

2. **Client-Side Validation:**

   - Provide validation before upload
   - Return user-friendly error messages
   - Support contract visualization

---

## Migration from Previous Versions

### From V2 to V3

Key changes:

- Added `parties` array (previously implicit)
- Added `owner` field to steps (previously optional)
- Added `business_context` to steps with `required` and `optional` fields (new feature)
- Enhanced retry logic with `retry_leads_to` field
- Added contract-level `validation` object
- Introduced runtime validation with severity levels (critical, minor, info)

Migration steps:

1. Extract all unique owners from steps into `parties` array
2. Ensure all steps have explicit `owner` field
3. Add `business_context` objects to relevant steps with `required` and `optional` arrays
4. Classify existing business context fields as either required or optional
5. Update retry transitions to use new `retry_leads_to` field
6. Add `validation` object with appropriate constraints
7. Review and configure violation severity levels for your use case

---

## Future Considerations

Potential enhancements for V4:

- Conditional transitions based on business context
- Parallel step execution
- Sub-contracts and composition
- Dynamic party assignment
- Event-driven triggers
- SLA monitoring and alerting

---

## Related Documentation

- [Claims Caching](./CLAIMS_CACHING.md)
- [Contract Validation Tests](./CONTRACT_VALIDATION_TESTS.md)
- [WebSocket Architecture](../threadify-go/docs/WEBSOCKET_ARCHITECTURE.md)
- [System Architecture](../threadify-go/docs/COMPLETE_SYSTEM_ARCHITECTURE.md)
