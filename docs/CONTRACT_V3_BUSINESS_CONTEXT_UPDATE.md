# Contract V3: Business Context Update

## Overview

This document summarizes the updated `business_context` structure in Contract V3, which now supports distinguishing between **required** and **optional** fields.

## Structure Change

### Previous Structure (Flat Array)

```yaml
steps:
  - id: order_placed
    owner: merchant
    business_context:
      - order_id
      - customer_id
      - total_amount
```

### New Structure (Required/Optional)

```yaml
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
```

## Key Benefits

1. **Clear Data Requirements**: Explicitly defines which fields are mandatory vs. optional
2. **Better Validation**: Missing required fields trigger critical errors; missing optional fields are informational only
3. **Improved Data Quality**: Track data completeness separately from data integrity
4. **Flexible Evolution**: Add optional fields without breaking existing integrations

## Validation Behavior

### Required Fields

- **Missing required field** → **MAJOR** violation
- Thread execution **fails immediately**
- All required fields must be present for step to be accepted

```json
{
  "error": "missing_required_field",
  "severity": "major",
  "message": "Step 'order_placed' is missing required business_context field 'total_amount'"
}
```

### Optional Fields

- **Missing optional field** → **INFO** violation
- Thread execution **continues normally**
- Logged for data completeness tracking

```json
{
  "info": "missing_optional_field",
  "severity": "info",
  "message": "Step 'order_placed' is missing optional business_context fields"
}
```

### Undocumented Fields

- **Extra undocumented field** → **INFO** violation
- Field not in required or optional arrays
- May indicate contract needs updating

```json
{
  "info": "extra_undocumented_field",
  "severity": "info",
  "message": "Step 'order_placed' contains undocumented fields"
}
```

## Contract Validation Rules

When uploading a contract, the following validation applies to `business_context`:

1. Must be an object (not a string or array)
2. Can contain `required` and/or `optional` fields
3. Both `required` and `optional` must be arrays of strings
4. At least one of `required` or `optional` should be present if `business_context` is defined

### Valid Examples

**Both required and optional:**

```yaml
business_context:
  required:
    - order_id
    - customer_id
  optional:
    - customer_notes
```

**Only required:**

```yaml
business_context:
  required:
    - order_id
    - customer_id
```

**Only optional:**

```yaml
business_context:
  optional:
    - customer_notes
    - promotional_code
```

### Invalid Examples

**String instead of object:**

```yaml
business_context: "order_id"  # ❌ ERROR
```

**Array instead of object:**

```yaml
business_context:
  - order_id  # ❌ ERROR
  - customer_id
```

**String instead of array:**

```yaml
business_context:
  required: "order_id"  # ❌ ERROR
```

## Migration Guide

### For Existing Contracts

If you have existing contracts with the old flat array structure, migrate as follows:

**Step 1: Identify Critical Fields**

Determine which fields are absolutely necessary for your business process.

**Step 2: Classify Fields**

- **Required**: Fields without which the step cannot proceed
- **Optional**: Fields that enhance data but aren't strictly necessary

**Step 3: Update Structure**

```yaml
# Old
business_context:
  - order_id
  - customer_id
  - total_amount
  - customer_notes

# New
business_context:
  required:
    - order_id
    - customer_id
    - total_amount
  optional:
    - customer_notes
```

**Step 4: Test Validation**

- Verify required fields trigger critical errors when missing
- Verify optional fields only log info violations when missing

### For New Contracts

Always use the new structure from the start:

1. Define required fields first (core business data)
2. Add optional fields for enrichment data
3. Document the purpose of each field in your contract documentation

## Monitoring Recommendations

### Data Integrity Metrics

Track `missing_required_field` major violations (data integrity issues) → Data collection issues, integration problems
- **Alert threshold**: > 1% of step executions
- **Action**: Fix upstream data sources

### Data Completeness Metrics

Track `missing_optional_field` violations:

- **High rate** → Opportunity to improve data richness
- **Alert threshold**: > 50% of step executions (informational)
- **Action**: Encourage optional field submission

### Contract Evolution Metrics

Track `extra_undocumented_field` violations:

- **New fields appearing** → Contract may need updating
- **Alert threshold**: New field types appearing
- **Action**: Review and potentially add to contract

## Quick Reference: Validation Scenarios

| Scenario | Required Fields | Optional Fields | Extra Fields | Result | Severity |
|----------|----------------|-----------------|--------------|--------|----------|
| All fields present | ✅ All | ✅ All | ❌ None | Success | None |
| Only required | ✅ All | ❌ Missing | ❌ None | Success | Info (missing optional) |
| Required + extra | ✅ All | ❌ Missing | ✅ Present | Success | Info (extra undocumented) |
| Missing required | ❌ Missing | Any | Any | **Failure** | **Major** |
| Required + optional + extra | ✅ All | ✅ All | ✅ Present | Success | Info (extra undocumented) |

---

## SDK Examples

### Example Contract Definition

```yaml
steps:
  - id: package_shipped
    owner: logistics_carrier
    business_context:
      required:
        - tracking_number
        - carrier
      optional:
        - estimated_delivery
        - carrier_contact
```

### Scenario 1: All Required Fields Present

```javascript
// ✅ Valid - has all required fields
thread.step("package_shipped", {
    "tracking_number": "ABC123",
    "carrier": "DHL"
})
```

**Result**: ✅ Success, no violations

---

### Scenario 2: Required + Optional + Extra Fields

```javascript
// ✅ Valid - has required + optional + extra fields
thread.step("package_shipped", {
    "tracking_number": "ABC123",
    "carrier": "DHL",
    "estimated_delivery": "2025-01-15",        // Optional field
    "internal_shipping_id": "XYZ789"           // Not in contract, but allowed
})
```

**Result**: ✅ Success, info violation logged for `extra_undocumented_field`

**Violation Details:**

```json
{
  "info": "extra_undocumented_field",
  "severity": "info",
  "message": "Step 'package_shipped' contains undocumented fields",
  "step_id": "package_shipped",
  "extra_fields": ["internal_shipping_id"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

### Scenario 3: Missing Required Field

```javascript
// ❌ Critical Violation - missing required field
thread.step("package_shipped", {
    "carrier": "DHL"
    // Missing "tracking_number" (required)
})
```

**Result**: ❌ Major error, thread execution fails immediately

**Error Response:**

```json
{
  "error": "missing_required_field",
  "severity": "major",
  "message": "Step 'package_shipped' is missing required business_context field 'tracking_number'",
  "step_id": "package_shipped",
  "missing_required_fields": ["tracking_number"],
  "provided_fields": ["carrier"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

### Scenario 4: Optional Fields Omitted

```javascript
// ✅ Valid - optional fields can be omitted
thread.step("package_shipped", {
    "tracking_number": "ABC123",
    "carrier": "DHL"
    // "estimated_delivery" not provided - that's fine
    // "carrier_contact" not provided - that's fine
})
```

**Result**: ✅ Success, info violation logged for missing optional fields

**Violation Details:**

```json
{
  "info": "missing_optional_field",
  "severity": "info",
  "message": "Step 'package_shipped' is missing optional business_context fields",
  "step_id": "package_shipped",
  "missing_optional_fields": ["estimated_delivery", "carrier_contact"],
  "provided_fields": ["tracking_number", "carrier"],
  "thread_id": "thread_abc123",
  "timestamp": "2025-12-27T16:39:00Z"
}
```

---

### Scenario 5: All Fields Present

```javascript
// ✅ Valid - complete data submission
thread.step("package_shipped", {
    "tracking_number": "ABC123",
    "carrier": "DHL",
    "estimated_delivery": "2025-01-15",
    "carrier_contact": "+1-800-SHIP-IT"
})
```

**Result**: ✅ Success, no violations

---

## API Examples (REST)

### Submitting Step Data with All Fields

```json
POST /api/v3/threads/thread_abc123/steps

{
  "step_id": "package_shipped",
  "party": "logistics_carrier",
  "data": {
    "tracking_number": "ABC123",
    "carrier": "DHL",
    "estimated_delivery": "2025-01-15",
    "carrier_contact": "+1-800-SHIP-IT"
  }
}
```

**Result**: ✅ Success, no violations

### Submitting Step Data with Only Required Fields

```json
POST /api/v3/threads/thread_abc123/steps

{
  "step_id": "package_shipped",
  "party": "logistics_carrier",
  "data": {
    "tracking_number": "ABC123",
    "carrier": "DHL"
  }
}
```

**Result**: ✅ Success, info violation logged for missing optional fields

### Submitting Step Data Missing Required Field

```json
POST /api/v3/threads/thread_abc123/steps

{
  "step_id": "package_shipped",
  "party": "logistics_carrier",
  "data": {
    "carrier": "DHL"
    // Missing: tracking_number
  }
}
```

**Result**: ❌ Critical error, thread execution fails

## Best Practices

### 1. Start Conservative

When in doubt, mark fields as required. You can always relax to optional later without breaking existing integrations.

### 2. Document Field Purpose

Add comments in your contract explaining why each field is required or optional:

```yaml
business_context:
  required:
    - order_id          # Unique identifier for order tracking
    - customer_id       # Required for customer lookup
    - total_amount      # Needed for payment processing
  optional:
    - customer_notes    # Enhances customer service
    - promotional_code  # For discount tracking only
```

### 3. Review Periodically

Regularly review your optional field usage:

- If an optional field is always present → Consider making it required
- If a required field is rarely used → Consider making it optional

### 4. Version Carefully

When changing field requirements:

- **Optional → Required**: Create a new contract version
- **Required → Optional**: Can be done in-place with caution
- **Adding new optional**: Safe to do in-place

## Related Documentation

- [Contract Structure V3 Specification](./CONTRACT_STRUCTURE_V3.md)
- [Contract Validation Tests](./CONTRACT_VALIDATION_TESTS.md)
- [Claims Caching](./CLAIMS_CACHING.md)

## Summary

The new `business_context` structure provides:

- ✅ Clear distinction between required and optional data
- ✅ Better error handling and validation
- ✅ Improved data quality tracking
- ✅ Flexible contract evolution
- ✅ Enhanced monitoring capabilities

This change enables more robust contract enforcement while maintaining flexibility for data enrichment.
