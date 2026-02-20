# Workflow Contracts

## Overview
Contracts define the structure and rules for workflows in Threadify. They specify entry points, allowed transitions, terminal states, timeouts, and business context requirements.

## Contract Structure

```yaml
contract_name: payment_flow
version: 3
description: Complete payment processing workflow

# Entry points - where threads can start
entry_points:
  - order_placed

# Parties involved in the workflow
parties:
  - merchant
  - payment_processor
  - warehouse_manager

# Steps in the workflow
steps:
  - id: order_placed
    owner: merchant
    type: managed
    timeout: 5m
    business_context:
      required:
        - order_id
        - customer_id
        - total_amount
      optional:
        - notes
        - promo_code

  - id: payment_validation
    owner: payment_processor
    type: external
    timeout: 30s
    business_context:
      required:
        - payment_method
        - amount

  - id: payment_validated
    owner: payment_processor
    type: managed

  - id: order_cancelled
    owner: merchant
    type: managed
    business_context:
      required:
        - cancellation_reason

# Transitions define allowed step sequences
transitions:
  - from: order_placed
    to: [payment_validation]
    max_retries: 3
  
  - from: payment_validation
    to: [payment_validated, order_cancelled]
    max_retries: 2

# Terminal steps - where threads end
terminal_steps:
  - payment_validated
  - order_cancelled

# Thread-level settings
max_duration: 24h
allow_multiple_terminals: false
```

## Step Types

### `managed`
- Fully controlled by the service
- Can be retried automatically
- Subject to timeout validation

### `external`
- Depends on external systems
- May have longer timeouts
- Retry logic handled externally

### `human_in_loop`
- Requires human intervention
- Typically longer timeouts
- Manual retry/resolution

## Business Context

### Required Fields
Fields that MUST be present for the step to be valid. Missing required fields will block step execution.

```yaml
business_context:
  required:
    - order_id
    - customer_id
```

### Optional Fields
Fields that are recommended but not mandatory. Missing optional fields generate info-level violations but don't block execution.

```yaml
business_context:
  optional:
    - notes
    - promo_code
    - estimated_delivery
```

## Transitions

### Basic Transition
```yaml
transitions:
  - from: order_placed
    to: [payment_validation]
```

### With Retry Limits
```yaml
transitions:
  - from: payment_validation
    to: [payment_validated, order_cancelled]
    max_retries: 3
```

### Multiple Allowed Next Steps
```yaml
transitions:
  - from: payment_validation
    to: 
      - payment_validated  # Success path
      - order_cancelled    # Failure path
```

## Validation Rules

### Entry Point Validation
- First step in a thread MUST be an entry point
- Blocks execution if violated
- Severity: Critical

### Transition Validation
- Steps must follow allowed transition paths
- Invalid transitions generate critical violations
- Checked atomically in Lua script

### Terminal Step Validation
- Thread completes when a terminal step succeeds
- Multiple terminal steps can be reached if `allow_multiple_terminals: true`
- Default: only one terminal step allowed

### Timeout Validation
- Steps must complete within specified timeout
- Generates critical violation if exceeded
- Timeout starts when step is first recorded

### Retry Limit Validation
- Steps cannot be retried more than `max_retries` times
- Tracked per transition
- Generates critical violation when exceeded

### Business Context Validation
- Required fields: Blocking validation
- Optional fields: Non-blocking (info severity)

## Contract Versioning

### Version Field
```yaml
version: 3
```

Contracts use semantic versioning. Breaking changes require a new version number.

### Backward Compatibility
- Version 3 supports both old (flat array) and new (required/optional) business context formats
- Older contracts are automatically migrated on load

## Thread Lifecycle with Contracts

```
1. Thread Created
   ↓
2. First Step Validated (must be entry point)
   ↓
3. Subsequent Steps Validated (transitions, timeouts, retries)
   ↓
4. Terminal Step Reached
   ↓
5. Thread Marked Complete
```

## Non-Contract Workflows

Threads can be created without contracts:
- No validation rules enforced
- Steps can be in any order
- No timeout or retry limits
- Useful for ad-hoc workflows or logging

## Best Practices

1. **Define clear entry points** - Make it obvious where workflows start
2. **Use meaningful step names** - Reflect business actions, not technical operations
3. **Set realistic timeouts** - Account for external system latency
4. **Limit retry attempts** - Prevent infinite loops
5. **Document business context** - Explain what each field represents
6. **Version contracts carefully** - Breaking changes affect all active threads
7. **Test transitions** - Ensure all paths are valid and reachable

## Examples

### Simple Linear Workflow
```yaml
contract_name: simple_order
entry_points: [order_created]
steps:
  - id: order_created
  - id: order_processed
  - id: order_shipped
transitions:
  - from: order_created
    to: [order_processed]
  - from: order_processed
    to: [order_shipped]
terminal_steps: [order_shipped]
```

### Branching Workflow
```yaml
contract_name: payment_with_retry
entry_points: [payment_initiated]
steps:
  - id: payment_initiated
  - id: payment_processing
  - id: payment_success
  - id: payment_failed
transitions:
  - from: payment_initiated
    to: [payment_processing]
  - from: payment_processing
    to: [payment_success, payment_failed]
    max_retries: 3
terminal_steps: [payment_success, payment_failed]
```

### Multi-Party Workflow
```yaml
contract_name: marketplace_order
parties:
  - buyer
  - seller
  - payment_processor
  - logistics
steps:
  - id: order_placed
    owner: buyer
  - id: order_confirmed
    owner: seller
  - id: payment_collected
    owner: payment_processor
  - id: item_shipped
    owner: logistics
```

## Contract Storage

Contracts are stored in:
- **Development**: YAML files in `/examples/` directory
- **Production**: Database with versioning support
- **Runtime**: Cached in memory for performance

## Contract Updates

### Safe Updates
- Adding new optional fields
- Adding new terminal steps (if `allow_multiple_terminals: true`)
- Increasing timeouts
- Increasing retry limits

### Breaking Updates (require new version)
- Removing steps
- Changing required fields
- Removing transitions
- Decreasing timeouts or retry limits
- Changing entry points

## Troubleshooting

### "Invalid entry point" error
- First step is not in `entry_points` list
- Solution: Start thread with a valid entry point

### "Invalid transition" violation
- Step executed out of order
- Solution: Follow allowed transition paths in contract

### "Retry limit exceeded" violation
- Step retried too many times
- Solution: Fix underlying issue or increase `max_retries`

### "Step timeout exceeded" violation
- Step took longer than allowed
- Solution: Optimize step execution or increase timeout

### "Multiple terminal states" violation
- Thread reached multiple terminal steps
- Solution: Set `allow_multiple_terminals: true` or fix workflow logic
