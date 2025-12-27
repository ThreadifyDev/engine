# Violation Severity Reference

Quick reference guide for all Threadify contract violations organized by severity level.

## Severity Levels

| Level | Description | Action |
| ----- | ----------- | ------ |
| **Critical** | Violations that break contract integrity | Thread execution fails immediately |
| **Major** | Data or timing constraint violations | Thread execution fails immediately |
| **Minor** | Potential issues that don't break the contract | Logged; execution continues |
| **Info** | Informational tracking and auditing | Logged; no impact on execution |

---

## Critical Violations

### 1. `invalid_transition`

**Description:** Attempting a transition not allowed by contract rules

**Causes:**
- Transitioning to a step not in the `to` field of current step's transitions
- Skipping required intermediate steps

**Example:**
```javascript
// Contract allows: order_placed → payment_validated
// Violation: order_placed → package_shipped
```

---

### 2. `unauthorized_owner`

**Description:** Party executing a step they don't own

**Causes:**
- Wrong party attempting to execute a step
- Missing or incorrect authentication claims

**Example:**
```javascript
// Contract: payment_validated owned by payment_processor
// Violation: merchant attempts to execute payment_validated
```

---

### 3. `contract_not_found`

**Description:** Contract doesn't exist or has been deleted

**Causes:**
- Invalid contract name or version
- Contract was deleted after thread creation
- Typo in contract reference

**Example:**
```javascript
thread.create({
  contract_name: "nonexistent_contract",
  contract_version: 3
})
```

---

## Major Violations

### 1. `missing_required_field`

**Description:** Required business_context field is missing

**Causes:**
- Client didn't provide required field
- Field name mismatch (typo)
- Data transformation error upstream

**Example:**
```javascript
// Contract requires: tracking_number, carrier
// Provided: carrier only
thread.step("package_shipped", {
  "carrier": "DHL"
  // Missing: tracking_number
})
```

**WebSocket Event:**
```json
{
  "event": "thread_violation",
  "thread_id": "thread_123",
  "violation_type": "missing_required_field",
  "severity": "major",
  "step_id": "package_shipped",
  "missing_fields": ["tracking_number"],
  "provided_fields": ["carrier"]
}
```

---

### 2. `max_duration_exceeded`

**Description:** Thread exceeded contract's max_duration

**Causes:**
- Long-running process exceeded time limit
- Stuck in retry loop
- Waiting for external system response

**Example:**
```yaml
# Contract: max_duration: 72h
# Violation: Thread running for 73h
```

---

### 3. `step_timeout_exceeded`

**Description:** Step exceeded its timeout duration

**Causes:**
- Step took longer than allowed
- External API slow to respond
- Processing bottleneck

**Example:**
```yaml
# Contract: payment_validated timeout: 5s
# Violation: Step took 7s
```

---

## Minor Violations

### 1. `multiple_terminal_states`

**Description:** Thread reached multiple terminal states

**Causes:**
- Workflow design allows multiple endings
- Parallel execution paths converging
- Configured as acceptable via `allow_multiple_terminals: true`

**Example:**
```yaml
# Thread reaches both: delivered AND order_cancelled
```

**Note:** Severity can be configured via `multiple_terminals_severity` field

---

### 2. `retry_attempted`

**Description:** Retry transition executed (informational)

**Causes:**
- Automatic retry triggered after failure
- Within max_retries limit

**Example:**
```yaml
# payment_failed → retry → payment_validated
# Retry count: 1 of 3
```

---

## Info Violations

### 1. `missing_optional_field`

**Description:** Optional business_context field is missing

**Causes:**
- Client didn't provide optional field
- Data not available at submission time

**Example:**
```javascript
// Contract optional: estimated_delivery, carrier_contact
// Provided: tracking_number, carrier only
thread.step("package_shipped", {
  "tracking_number": "ABC123",
  "carrier": "DHL"
  // Missing optional: estimated_delivery, carrier_contact
})
```

**Note:** This is purely informational for data completeness tracking

---

### 2. `extra_undocumented_field`

**Description:** Field not in contract's business_context

**Causes:**
- Client sending additional fields
- Contract needs updating to include new field
- Internal/debug fields being sent

**Example:**
```javascript
thread.step("package_shipped", {
  "tracking_number": "ABC123",
  "carrier": "DHL",
  "internal_shipping_id": "XYZ789"  // Not in contract
})
```

**Note:** Extra fields are allowed but logged for contract evolution tracking

---

### 3. `step_completed`

**Description:** Step completed successfully (performance tracking)

**Causes:**
- Normal step completion
- Used for performance monitoring

**Example:**
```yaml
# Step completed in 2h of 24h timeout
# Utilization: 8.3%
```

---

## Violation Summary Table

| Violation Type | Severity | Execution | Common Cause |
| -------------- | -------- | --------- | ------------ |
| `invalid_transition` | Critical | Fails | Wrong workflow path |
| `unauthorized_owner` | Critical | Fails | Wrong party executing |
| `contract_not_found` | Critical | Fails | Invalid contract reference |
| `missing_required_field` | Major | Fails | Missing required data |
| `max_duration_exceeded` | Major | Fails | Thread took too long |
| `step_timeout_exceeded` | Major | Fails | Step took too long |
| `multiple_terminal_states` | Minor | Continues | Multiple endings reached |
| `retry_attempted` | Minor | Continues | Retry triggered |
| `missing_optional_field` | Info | Continues | Optional data not provided |
| `extra_undocumented_field` | Info | Continues | Extra fields sent |
| `step_completed` | Info | Continues | Normal completion |

---

## Monitoring Priorities

### High Priority (Immediate Action)

**Critical Violations:**
- Alert threshold: > 0.1% of requests
- Action: Investigate immediately, may indicate integration issues

**Major Violations:**
- Alert threshold: > 1% of requests
- Action: Review data sources and timeout configurations

### Medium Priority (Review Regularly)

**Minor Violations:**
- Alert threshold: > 10% of threads
- Action: Review workflow design, consider contract updates

### Low Priority (Track Trends)

**Info Violations:**
- Alert threshold: Track trends over time
- Action: Identify data completeness opportunities

---

## WebSocket Event Handling

```javascript
thread.on("violation", (event) => {
  switch (event.severity) {
    case "critical":
    case "major":
      // Execution has failed
      console.error(`Thread failed: ${event.message}`);
      notifyUser(event);
      logError(event);
      break;
      
    case "minor":
      // Execution continues but needs attention
      console.warn(`Thread warning: ${event.message}`);
      logWarning(event);
      break;
      
    case "info":
      // Informational only
      console.info(`Thread info: ${event.message}`);
      trackAnalytics(event);
      break;
  }
});
```

---

## Related Documentation

- [Contract Structure V3 Specification](./CONTRACT_STRUCTURE_V3.md)
- [Business Context Update Guide](./CONTRACT_V3_BUSINESS_CONTEXT_UPDATE.md)
- [Contract Validation Tests](./CONTRACT_VALIDATION_TESTS.md)
