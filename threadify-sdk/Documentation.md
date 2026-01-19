
Build business process graphs with context—track what happened, validate every step, and trigger context-aware actions.

---

## Data Retrieval API

Threadify lets you build business process graphs with context—track what happened, validate every step, and trigger context-aware actions.

This SDK provides a concise, modern GraphQL-based API for accessing all archived thread data, step history, and validations.

### Connection Methods

#### `connection.getThread(threadId)`

Get a thread by ID with access to all its data.

**Parameters:**
- `threadId` (string, required): Thread ID

**Returns:** `Promise<ArchivedThread>`

**Example:**
```javascript
const thread = await connection.getThread('thread-uuid-123');
// thread.id, thread.status, thread.contractName
```

---

#### `connection.getThreadsByRef({ refKey, refValue, ...filters })`

Find threads by external reference with optional server-side filtering.

**Parameters:**
- `refKey` (string, required): Reference key (e.g., "orderId")
- `refValue` (string, required): Reference value (e.g., "ORDER-12345")
- `status` (string, optional): Filter by status ("active", "completed", etc.)
- `startedAfter` (string, optional): ISO timestamp - only threads started after this time
- `startedBefore` (string, optional): ISO timestamp - only threads started before this time
- `limit` (number, optional): Maximum results (default: 50)
- `offset` (number, optional): Pagination offset (default: 0)

**Returns:** `Promise<Array<ArchivedThread>>`

**Example:**
```javascript
const threads = await connection.getThreadsByRef({ refKey: 'orderId', refValue: 'ORDER-12345' });
// With filters: status, time range, pagination
const filtered = await connection.getThreadsByRef({ 
  refKey: 'orderId', 
  refValue: 'ORDER-12345',
  status: 'completed',
  startedAfter: '2026-01-01T00:00:00Z',
  limit: 10
});
```

---

### ArchivedThread Methods

#### `thread.steps(filters)`

Get all steps for this thread, optionally filtered.

**Parameters:**
- `filters` (object, optional):
  - `stepName` (string): Filter by step name
  - `idempotencyKey` (string): Filter by idempotency key

**Returns:** `Promise<Array<ArchivedStep>>`

**Example:**
```javascript
const allSteps = await thread.steps(); // all steps
const stepsByName = await thread.steps({ stepName: 'order_placed' }); // filter by name
const stepsByNameAndIdemp = await thread.steps({ stepName: 'order_placed', idempotencyKey: 'order-123' }); // filter by name and idempKey
```

---

#### `thread.getStep(stepIdentifier)`

Get a specific step by name or "name:idempotencyKey".

**Parameters:**
- `stepIdentifier` (string, required): Step name or "stepName:idempKey"

**Returns:** `Promise<ArchivedStep>`

**Example:**
```javascript
const step = await thread.getStep('order_placed'); // by step name
const stepWithIdemp = await thread.getStep('order_placed:order-123'); // by stepName:idempKey
```

---

#### `thread.validationResults(options)`

Get validation results for this thread.

**Parameters:**
- `options` (object, optional):
  - `limit` (number): Maximum results to return
  - `stepName` (string): Filter by step name
  - `validationType` (string): Filter by validation type

**Returns:** `Promise<Array<ValidationResult>>`

**Example:**
```javascript
const validations = await thread.validationResults({ limit: 10 });
// validations is an array of ValidationResult objects
```

---

#### `thread.getCompleteData(options)` ⭐ **NEW**

Get complete thread picture with all nested data in a **single GraphQL query**. This is the most efficient way to retrieve all thread data.

**Parameters:**
- `options` (object, optional):
  - `stepHistoryLimit` (number): Limit for step history per step (default: 50)
  - `validationLimit` (number): Limit for validation results (default: 10)
  - `stepName` (string): Filter steps by name
  - `idempotencyKey` (string): Filter steps by idempotency key

**Returns:** `Promise<Object>` with structure:
```javascript
{
  id, contractId, contractVersion, contractName,
  ownerId, companyId, status, lastHash, refs,
  startedAt, completedAt, error,
  steps: [{
    threadId, stepName, idempotencyKey, status,
    retryCount, firstSeenAt, lastUpdatedAt,
    latestStepID, previousStep,
    history: [{ attempt, timestamp, status, context, duration, error }]
  }],
  validationResults: [{
    validationId, threadId, stepId, stepName,
    idempotencyKey, timestamp, overallStatus,
    hasCriticalViolation, criticalCount, warningCount,
    validations: [{ type, message, field, expected, actual, rule }]
  }]
}
```

**Example:**
```javascript
const completeData = await thread.getCompleteData({ stepHistoryLimit: 50, validationLimit: 10 });
// completeData.steps, completeData.validationResults, etc.
```

**Benefits:**
- ✅ Single network request (much faster)
- ✅ Atomic data snapshot
- ✅ Reduced server load
- ✅ Perfect for dashboards and audit trails

---

### ArchivedStep Methods

#### `step.history(options)`

Get execution history for this step.

**Parameters:**
- `options` (object, optional):
  - `limit` (number): Maximum records (default: 100)
  - `offset` (number): Pagination offset (default: 0)
  - `startAt` (string): ISO timestamp to filter from
  - `endAt` (string): ISO timestamp to filter to
  - `activityType` (string): Filter by activity type
  - `actor` (string): Filter by actor

**Returns:** `Promise<Array<StepHistory>>`

**Example:**
```javascript
const step = await thread.getStep('order_placed');
const history = await step.history({ limit: 100 }); // all history
const filtered = await step.history({ limit: 10, activityType: 'step_recorded', startAt: '2026-01-01T00:00:00Z' }); // filtered
```

---

## Data Retrieval Examples

### Example: Complete Thread Audit Trail

```javascript
import { Threadify } from 'threadify-sdk';

const connection = await Threadify.connect('api-key', 'audit-service');

// Get complete thread picture in one query
const thread = await connection.getThread('thread-uuid');
const completeData = await thread.getCompleteData({
  stepHistoryLimit: 100,
  validationLimit: 50
});

// Generate audit report
console.log('=== Thread Audit Report ===');
console.log(`Thread ID: ${completeData.id}`);
console.log(`Contract: ${completeData.contractName} v${completeData.contractVersion}`);
console.log(`Status: ${completeData.status}`);
console.log(`Duration: ${new Date(completeData.completedAt) - new Date(completeData.startedAt)}ms`);

console.log('\n=== Steps ===');
completeData.steps.forEach(step => {
  console.log(`\n${step.stepName}:${step.idempotencyKey}`);
  console.log(`  Status: ${step.status}`);
  console.log(`  Retries: ${step.retryCount}`);
  console.log(`  History:`);
  step.history.forEach(h => {
    console.log(`    ${h.timestamp}: ${h.status} (${h.duration}ms)`);
  });
});

console.log('\n=== Validations ===');
completeData.validationResults.forEach(val => {
  if (val.hasCriticalViolation) {
    console.log(`❌ ${val.stepName}: ${val.criticalCount} critical issues`);
  }
});
```

### Example: Find Threads by Reference

```javascript
// Find all threads for a specific order
const threads = await connection.getThreadsByRef({
  refKey: 'orderId',
  refValue: 'ORDER-12345'
});

console.log(`Found ${threads.length} threads for order ORDER-12345`);

for (const thread of threads) {
  const data = await thread.getCompleteData();
  console.log(`Thread ${data.id}: ${data.status}`);
  console.log(`  Steps: ${data.steps.length}`);
  console.log(`  Started: ${data.startedAt}`);
}
```

### Example: Step-Level Analysis

```javascript
const thread = await connection.getThread('thread-uuid');
const step = await thread.getStep('payment_processing');

// Get detailed history
const history = await step.history({ limit: 50 });

console.log(`Payment Processing - ${history.length} attempts`);

const failures = history.filter(h => h.status === 'failed');
console.log(`Failed attempts: ${failures.length}`);

failures.forEach(f => {
  console.log(`  ${f.timestamp}: ${f.error}`);
});
```

---

## Support

For issues, questions, or contributions:
- GitHub: [ThreadifyEngine Repository]
- Documentation: This file
- Examples: See `/tests/e2e-validation.test.js`
- Examples: See `/tests/e2e-data-retrieval.test.js`

---
