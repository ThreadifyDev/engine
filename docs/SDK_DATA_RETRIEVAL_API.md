# SDK Data Retrieval API Design

## Overview

This document defines the SDK API for retrieving thread, step, and validation data from the Threadify Engine. The design prioritizes consistency, permission-aware filtering, and developer experience.

---

## Core Principles

1. **Permission-aware** - All results filtered by access scope (owner/participant/observer)
2. **Idempotency key support** - `stepName:idempKey` format parsing throughout
3. **No duplication** - Use existing event handlers, no watch methods
4. **Focused scope** - No search/analytics in SDK (GraphQL for UI only)
5. **Array consistency** - `getStep()` ALWAYS returns array, even for single result

---

## Complete SDK API

### Connection Level

```javascript
const connection = await Threadify.connect({ 
  apiKey: 'your-api-key',
  serverUrl: 'ws://localhost:8081/threads'
});

const thread = await connection.getThread(threadId);
```

---

### Thread Instance Methods

```javascript
class ThreadInstance {
  // ==================
  // Metadata Methods
  // ==================
  
  getThreadId()
  // Returns: string
  // Example: 'thread-uuid-123'
  
  getStatus()
  // Returns: 'active' | 'completed' | 'failed'
  
  getMetadata()
  // Returns: { contractName, version, createdAt, updatedAt, ... }
  // NOTE: Does NOT include access/permissions data
  
  // ==================
  // Step Methods
  // ==================
  
  async getStep(identifier)
  // Parameters:
  //   identifier: 'stepName' or 'stepName:idempKey'
  // Returns: Step[] (ALWAYS array, even for single result)
  // Returns: [] (empty array if not found)
  // Permission-filtered based on caller's access scope
  
  async getSteps()
  // Returns: Step[] (all recorded steps in thread)
  // Permission-filtered based on caller's access scope
  
  async getAvailableSteps()
  // Returns: ContractStep[] (all steps defined in contract graph)
  // Shows what steps are possible in this workflow
  
  // ==================
  // Validation Methods
  // ==================
  
  async getViolations(identifier?)
  // Parameters:
  //   identifier: Optional 'stepName' or 'stepName:idempKey' filter
  // Returns: Violation[]
  // Permission-filtered based on caller's access scope
  
  // ==================
  // Recording Methods (Existing)
  // ==================
  
  step(stepName)
  // Returns: ThreadStep (for recording new steps)
  
  // ==================
  // Event Handlers (Existing)
  // ==================
  
  onViolation(stepName, handler)
  onCompleted(stepName, handler)
  onFailed(stepName, handler)
}
```

---

## Data Types

### Step Object

```javascript
interface Step {
  stepId: string          // Unique step UUID
  stepName: string        // Step name from contract
  idempKey: string        // Idempotency key
  status: string          // 'success' | 'failed' | 'error'
  context: object         // Business context (permission-filtered)
  timestamp: string       // ISO 8601 timestamp
  retryCount: number      // Number of retry attempts
  
  // History method
  history(options: { start: number, end: number }): Promise<StepHistory[]>
}
```

### ContractStep Object

```javascript
interface ContractStep {
  stepName: string
  isEntryPoint: boolean
  isTerminal: boolean
  requiredFields: string[]
  optionalFields: string[]
  validTransitions: string[]
}
```

### Violation Object

```javascript
interface Violation {
  violationId: string
  stepName: string
  idempKey: string
  type: 'missing_field' | 'invalid_transition' | 'step_timeout' | 'retry_limit' | ...
  severity: 'critical' | 'warning' | 'info'
  message: string
  timestamp: string
  details: object
}
```

### StepHistory Object

```javascript
interface StepHistory {
  attempt: number
  timestamp: string
  status: string
  context: object
  duration: number
  error?: string
}
```

---

## Step Identifier Format

### Format: `stepName` or `stepName:idempKey`

**Rule:** Anywhere a stepName is passed to a function (except step creation), if the content contains a `:` followed by another content, that other content is the idempotency key.

### Parsing Logic

```javascript
function parseStepIdentifier(identifier) {
  const parts = identifier.split(':');
  
  if (parts.length === 1) {
    // Just step name, no idempKey
    return { stepName: parts[0], idempKey: null };
  } else if (parts.length === 2) {
    // stepName:idempKey format
    return { stepName: parts[0], idempKey: parts[1] };
  } else {
    throw new Error('Invalid step identifier format. Use "stepName" or "stepName:idempKey"');
  }
}
```

### Examples

```javascript
// Get all occurrences of a step
const steps = await thread.getStep('order_placed');
// Returns: [
//   { stepName: 'order_placed', idempKey: 'abc123', ... },
//   { stepName: 'order_placed', idempKey: 'def456', ... }
// ]

// Get specific step instance (still returns array)
const steps = await thread.getStep('order_placed:abc123');
// Returns: [
//   { stepName: 'order_placed', idempKey: 'abc123', ... }
// ]

// Not found
const steps = await thread.getStep('nonexistent_step');
// Returns: []
```

---

## Usage Examples

### Example 1: Get All Occurrences of a Step

```javascript
const orderSteps = await thread.getStep('order_placed');
console.log(`Found ${orderSteps.length} order_placed steps`);

orderSteps.forEach(step => {
  console.log(`- ${step.idempKey}: ${step.status}`);
  console.log(`  Order ID: ${step.context.order_id}`);
});
```

### Example 2: Get Specific Step Instance

```javascript
const [step] = await thread.getStep('order_placed:abc123');

if (step) {
  console.log('Step found:', step.status);
  console.log('Context:', step.context);
  
  // Get history
  const history = await step.history({ start: 0, end: 5 });
  console.log('History:', history);
} else {
  console.log('Step not found');
}
```

### Example 3: Check if Step Exists

```javascript
const steps = await thread.getStep('payment_validation');

if (steps.length > 0) {
  console.log('Payment validation completed');
} else {
  console.log('Payment validation not yet recorded');
}
```

### Example 4: Get All Steps in Thread

```javascript
const allSteps = await thread.getSteps();
console.log(`Thread has ${allSteps.length} recorded steps`);

// Build timeline
allSteps.forEach(step => {
  console.log(`${step.timestamp}: ${step.stepName} (${step.status})`);
});
```

### Example 5: Get Available Steps from Contract

```javascript
const availableSteps = await thread.getAvailableSteps();

console.log('Available steps in workflow:');
availableSteps.forEach(step => {
  const tags = [];
  if (step.isEntryPoint) tags.push('ENTRY');
  if (step.isTerminal) tags.push('TERMINAL');
  
  console.log(`- ${step.stepName} ${tags.join(' ')}`);
  console.log(`  Required: ${step.requiredFields.join(', ')}`);
  console.log(`  Can transition to: ${step.validTransitions.join(', ')}`);
});
```

### Example 6: Get Violations

```javascript
// Get all violations for thread
const allViolations = await thread.getViolations();
console.log(`Thread has ${allViolations.length} violations`);

// Get violations for specific step
const stepViolations = await thread.getViolations('order_placed:abc123');

stepViolations.forEach(violation => {
  console.log(`[${violation.severity}] ${violation.type}: ${violation.message}`);
});
```

### Example 7: Step History

```javascript
const [step] = await thread.getStep('payment_validation:xyz789');

if (step && step.retryCount > 0) {
  // Get full history of retries
  const history = await step.history({ start: 0, end: step.retryCount + 1 });
  
  console.log('Retry history:');
  history.forEach((attempt, idx) => {
    console.log(`Attempt ${idx + 1}: ${attempt.status} (${attempt.duration}ms)`);
    if (attempt.error) {
      console.log(`  Error: ${attempt.error}`);
    }
  });
}
```

---

## Backend HTTP Endpoints

### Thread Endpoints

```
GET  /api/threads/{threadId}
```
- Returns thread metadata (no access/permissions data)
- Response: `{ success: true, thread: { threadId, contractName, status, ... } }`

---

### Step Endpoints

```
GET  /api/threads/{threadId}/steps/{stepIdentifier}
```
- `stepIdentifier`: `"order_placed"` or `"order_placed:abc123"`
- Returns: `{ success: true, steps: Step[] }` (ALWAYS array)
- Permission-filtered based on API key

```
GET  /api/threads/{threadId}/steps
```
- Returns all recorded steps in thread
- Response: `{ success: true, steps: Step[] }`
- Permission-filtered

```
GET  /api/threads/{threadId}/steps/{stepIdentifier}/history?start=0&end=10
```
- Returns paginated step history
- Response: `{ success: true, history: StepHistory[] }`

```
GET  /api/threads/{threadId}/available-steps
```
- Returns contract graph steps
- Response: `{ success: true, steps: ContractStep[] }`

---

### Violation Endpoints

```
GET  /api/threads/{threadId}/violations?step={stepIdentifier}
```
- Optional `step` query parameter for filtering
- Response: `{ success: true, violations: Violation[] }`
- Permission-filtered

---

## Permission-Based Filtering

All methods filter results based on the caller's permission scope:

### Owner Scope
- **Access**: Full access to all steps
- **Context**: Full business context visible
- **Violations**: All violations visible

### Participant Scope
- **Access**: Only steps recorded by this service (matched by serviceName/apiKey)
- **Context**: Full context for own steps only
- **Violations**: Only violations for own steps

### Observer Scope
- **Access**: All steps visible
- **Context**: Sanitized/limited context (sensitive fields removed)
- **Violations**: All violations visible but with sanitized context

### Backend Implementation

```go
func (h *ThreadHandler) filterStepsByPermission(steps []Step, scope string, apiKey string) []Step {
    switch scope {
    case "owner":
        // Return all steps with full context
        return steps
        
    case "participant":
        // Return only steps recorded by this service
        filtered := []Step{}
        for _, step := range steps {
            if step.ServiceName == apiKey {
                filtered = append(filtered, step)
            }
        }
        return filtered
        
    case "observer":
        // Return steps with sanitized context
        sanitized := []Step{}
        for _, step := range steps {
            step.Context = sanitizeContext(step.Context)
            sanitized = append(sanitized, step)
        }
        return sanitized
        
    default:
        // No access
        return []Step{}
    }
}
```

---

## What's NOT in SDK

### ❌ No Watch/Subscribe Methods
Use existing event handlers instead:
```javascript
// ❌ Don't add these
thread.watch(callback)
thread.watchStep(stepName, callback)

// ✅ Use existing handlers
thread.onViolation('order_placed', handler)
thread.onCompleted('order_placed', handler)
```

### ❌ No Search Methods
Search belongs in GraphQL API for UI:
```javascript
// ❌ Not in SDK
connection.searchThreads(query)
connection.findThreads(filters)

// ✅ Only in GraphQL for UI dashboards
```

### ❌ No Analytics/Aggregations
Analytics belong in GraphQL API:
```javascript
// ❌ Not in SDK
connection.getThreadStats()
connection.getStepStats()
connection.getAverageDuration()

// ✅ Only in GraphQL for UI dashboards
```

### ❌ No Access/Permissions Data
getThread does not return access control data:
```javascript
// ❌ Not returned
{ access: [...], permissions: [...] }

// ✅ Only metadata
{ threadId, contractName, status, createdAt, ... }
```

---

## Benefits of This Design

### 1. Array Consistency
- ✅ No type checking needed - always iterate over array
- ✅ Consistent error handling - check `length === 0`
- ✅ Easier to extend - future features work the same way
- ✅ Predictable behavior - no surprises for developers

### 2. Permission Awareness
- ✅ Automatic filtering based on caller's scope
- ✅ No accidental data leakage
- ✅ Same API for all permission levels
- ✅ Backend enforces security

### 3. Idempotency Key Support
- ✅ Flexible querying (all steps or specific instance)
- ✅ Consistent format throughout API
- ✅ Easy to parse and validate
- ✅ Supports retry scenarios

### 4. Focused SDK
- ✅ Only operational methods, no UI-specific features
- ✅ Lightweight and fast
- ✅ Clear separation of concerns
- ✅ Easy to learn and use

---

## Implementation Status

**Status:** NOT YET IMPLEMENTED

**Next Steps:**
1. Discuss data retrieval strategy (Valkey vs Postgres, hot/cold path)
2. Implement backend HTTP endpoints
3. Add SDK methods to Thread.js
4. Add permission filtering logic
5. Write tests

---

## Related Documentation

- [Notification System](./NATS_MESSAGING.md)
- [Two-Phase ACK Architecture](./TWO_PHASE_ACK.md) (to be created)
- [Permission System](./PERMISSIONS.md) (to be created)
- [Contract Validation](./VALIDATION.md)

---

**Last Updated:** 2026-01-09
