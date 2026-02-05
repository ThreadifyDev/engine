# Threadify SDK Documentation

Build business process graphs with context—track what happened, validate every step, and trigger context-aware actions.

## Installation

```bash
npm install @threadify/sdk
```

### Module Support

The SDK supports both **ES Modules (ESM)** and **CommonJS (CJS)**:

**ES Modules (recommended):**
```javascript
import { Threadify } from '@threadify/sdk';
```

**CommonJS:**
```javascript
const { Threadify } = require('@threadify/sdk');
// or
const Threadify = require('@threadify/sdk');
```

**Full CommonJS Example:** See [`examples/commonjs-example.js`](examples/commonjs-example.js) for a complete working example.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Core Concepts](#core-concepts) - Connection, Thread, Step, Contract
3. [Common Scenarios](#common-scenarios) - Quick examples to get started
   - [Track a Simple Workflow](#track-a-simple-workflow)
   - [Link to External Systems](#link-to-external-systems)
   - [Handle Failures](#handle-failures-gracefully)
   - [Work with Contracts](#work-with-contracts-predefined-workflows)
   - [Invite Others & Join Existing Threads](#invite-others--join-existing-threads)
4. [Data Retrieval](#data-retrieval)
   - [Connection Methods](#connection-methods)
   - [ArchivedThread Methods](#archivedthread-methods)
   - [ArchivedStep Methods](#archivedstep-methods)
5. [Real-Time Notifications](#real-time-notifications)
   - [Subscribing to Notifications](#subscribing-to-notifications)
   - [Notification Object](#notification-object)
   - [Subscription Patterns](#subscription-patterns)
   - [Flow Control & HPA Support](#flow-control)
6. [Data Structures](#data-structures)
   - [ArchivedThread](#archivedthread-structure)
   - [ArchivedStep](#archivedstep-structure)
   - [ValidationResult](#validationresult-structure)
   - [StepHistory](#stephistory-structure)
7. [Support](#support)

---

## Quick Start

**ES Modules:**
```javascript
import { Threadify } from '@threadify/sdk';

// Connect with your API key
const connection = await Threadify.connect('your-api-key', 'my-service');

// Start tracking a workflow
const thread = await connection.start();

// Record each step with full context
await thread.step('order_placed')
  .addContext({ orderId: 'ORD-12345', amount: 99.99 })
  .success();

await thread.step('payment_processed')
  .addContext({ paymentId: 'PAY-67890' })
  .success();
```

**CommonJS:**
```javascript
const { Threadify } = require('@threadify/sdk');

// Same API as ESM
(async () => {
  const connection = await Threadify.connect('your-api-key', 'my-service');
  const thread = await connection.start();
  
  await thread.step('order_placed')
    .addContext({ orderId: 'ORD-12345', amount: 99.99 })
    .success();
})();
```

---

## Core Concepts

### 1. **Connection**
A WebSocket connection to the Threadify Engine. Manages authentication and message routing.

### 2. **Thread**
A workflow execution instance. Can be contract-based (with validation rules) or non-contract (free-form).

### 3. **Step**
An atomic unit of work within a thread. Steps have:
- **Name**: Identifies the step type
- **Context**: Business data associated with the step
- **Status**: `success`, `failed`, `error`
- **Idempotency**: Automatic deduplication based on name + context

### 4. **Contract**
Optional YAML-defined workflow specifications that enforce validation rules. Contracts define entry points, valid transitions, required fields, and role-based access control.

**Usage:**
- `contractName` - Uses the latest version
- `contractName:version` - Uses a specific version (e.g., `order_fulfillment:v2`)


## Common Scenarios

> **Note:** All examples below work with both ES Modules and CommonJS. Just replace `import` with `require()` and wrap in an async function if needed.

### Track a Simple Workflow

**ES Modules:**
```javascript
const connection = await Threadify.connect('your-api-key');
const thread = await connection.start();

// Each step is automatically validated and tracked
await thread.step('order_received')
  .addContext({ orderId: 'ORD-123', total: 299.99 })
  .success();

await thread.step('inventory_checked')
  .addContext({ inStock: true, warehouse: 'US-EAST' })
  .success();

await thread.step('payment_captured')
  .addContext({ paymentId: 'ch_abc123', amount: 299.99 })
  .success();
```

### Link to External Systems

```javascript
// Connect your workflow to Stripe, Shopify, etc.
await thread.step('process_payment')
  .addContext({ amount: 299.99, currency: 'USD' })
  .addRefs({
    stripe_payment_id: 'pi_abc123',
    shopify_order_id: '12345',
    customer_email: 'customer@example.com'
  })
  .success();

// Now you can trace from Stripe back to your workflow instantly
```

### Handle Failures Gracefully

```javascript
try {
  await processPayment(orderId);
  await thread.step('payment_processed')
    .addContext({ orderId, status: 'success' })
    .success();
} catch (error) {
  // Threadify tracks failures too
  await thread.step('payment_processed')
    .addContext({ orderId })
    .failed({ 
      message: 'Payment gateway timeout',
      errorCode: 'TIMEOUT',
      retries: 2,
      nextRetryAt: new Date(Date.now() + 5000).toISOString()
    });
  
  // You'll get notified automatically if this violates your workflow rules
}
```

### Mark Steps with Messages or Data

Complete steps with either a simple message or rich data object:

```javascript
// Simple string message
await thread.step('order_placed')
  .addContext({ orderId: 'ORD-123' })
  .success('Order placed successfully');

// Rich data object
await thread.step('payment_processed')
  .addContext({ orderId: 'ORD-123' })
  .success({ 
    message: 'Payment processed',
    transactionId: 'txn_abc123',
    amount: 99.99,
    timestamp: new Date().toISOString()
  });

// Error with details
await thread.step('inventory_check')
  .addContext({ warehouseId: 'WH-001' })
  .error({ 
    message: 'Inventory service unavailable',
    service: 'inventory-api',
    statusCode: 503,
    retryable: true
  });

// No message or data
await thread.step('simple_step')
  .success();
```

### Work with Contracts (Predefined Workflows)

Contracts enforce workflow structure and validate your steps automatically.

```javascript
// Start with latest version of contract
const thread = await connection.start('order_fulfillment', 'merchant');

// Or use a specific version
const thread2 = await connection.start('order_fulfillment:v2', 'merchant');

// Contract validates: entry point, required fields, role access
await thread.step('order_placed')
  .addContext({ 
    order_id: 'ORD-123',      // Required by contract
    product_id: 'PROD-456',   // Required by contract
    quantity: '2'             // Required by contract
  })
  .success();

// Contract ensures this is a valid next step for your role
await thread.step('payment_processed')
  .addContext({ 
    payment_id: 'PAY-789',
    amount: '99.99'
  })
  .success();
```

### Invite Others & Join Existing Threads

Collaborate on threads by inviting external partners or joining existing workflows.

**Inviting Others:**
- Thread owners can create invitation tokens for external parties
- Set business role, access level (permission tier), and expiration time
- Share token securely with partner
- Partner uses token to join the thread

**Joining Methods:**

**1. Token-Based Join (External Parties)**
- For external partners/services outside your organization
- Requires an invitation token created by the thread owner
- Token contains thread ID, role, permissions, and expiry
- Secure, time-limited access

**2. Direct Join (Internal Services)**
- For services within the same organization
- Only requires the thread ID and your role
- Authentication via API key
- Faster, simpler for internal collaboration

---

```javascript
// ============================================
// OWNER: Creating an invitation token
// ============================================
const ownerConnection = await Threadify.connect('owner-api-key', 'merchant-service');
const thread = await ownerConnection.start('order_fulfillment', 'merchant');

// Create invitation token for external partner
const invitationResponse = await thread.inviteParty({
  role: 'logistics',           // Business/contract role for the invited user
  accessLevel: 'participant',  // Access level: owner/participant/observer/external (default: external)
  expiresIn: '48h'             // Token expiration (e.g., '24h', '7d')
});

const invitationToken = invitationResponse.token;
console.log('Share this token:', invitationToken);
console.log('Access level:', invitationResponse.accessLevel);
// Token format: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

// ============================================
// INVITED USER: Joining with token
// ============================================
const invitedConnection = await Threadify.connect('invited-api-key', 'logistics-service');

// METHOD 1: Token-Based Join (for external partners/services)
// Use the invitation token shared by the thread owner
const joinedThread = await invitedConnection.join(invitationToken);

// METHOD 2: Direct Join (for internal services in same organization)
// Only requires thread ID and your role (no invitation needed)
const directThread = await invitedConnection.join('thread-uuid-123', 'logistics');

// ============================================
// COLLABORATION: Continue the workflow
// ============================================
await joinedThread.step('shipment_created')
  .addContext({ 
    trackingNumber: 'TRACK-456',
    carrier: 'FedEx'
  })
  .success();

// Check your access details
console.log('Thread ID:', joinedThread.threadId);
console.log('Your role:', joinedThread.role);
console.log('Your access level:', joinedThread.accessLevel);
```

**Access Levels Explained:**

Access levels control what users can do in a thread via the RBAC permission system:

- **`owner`** - Full control: create, read, write, delete, invite others, all notifications
- **`participant`** - Active collaboration: read, write, execute steps, critical notifications
- **`observer`** - Read-only: view thread data, completion notifications only
- **`external`** - Minimal access: view own steps/data only (default for invitations)

The `role` parameter is your business/contract role (e.g., "supplier", "merchant"), while `accessLevel` determines your actual system permissions.

---

## Data Retrieval

Query archived thread data, step history, and validation results.

### Connection Methods

#### `connection.getThread(threadId)`

Get a thread by ID with access to all its data.

```javascript
const thread = await connection.getThread('thread-uuid-123');
// thread.id, thread.status, thread.contractName
```

**Returns:** [`Promise<ArchivedThread>`](#archivedthread-structure)

---

#### `connection.getThreadsByRef(options)`

Find threads by external reference with optional server-side filtering.

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

**Returns:** [`Promise<Array<ArchivedThread>>`](#archivedthread-structure)

---

### ArchivedThread Methods

#### `thread.steps(stepIdentifier, options)`

Get all steps for this thread, optionally filtered.

```javascript
// Get all steps
const allSteps = await thread.steps();

// Filter by step name
const stepsByName = await thread.steps('order_placed');

// Filter by step name and idempotency key
const specificStep = await thread.steps('order_placed:order-123');

// Filter by step name and status
const successfulSteps = await thread.steps('order_placed', { status: 'success' });

// Get first step (single result)
const step = (await thread.steps('order_placed'))[0];
```

**Returns:** [`Promise<Array<ArchivedStep>>`](#archivedstep-structure)

---

#### `thread.validationResults(options)`

Get validation results (contract violations, timeouts, etc.) for this thread.

```javascript
const validations = await thread.validationResults({ limit: 10 });
validations.forEach(v => {
  if (v.hasCriticalViolation) {
    console.log(`❌ ${v.stepName}: ${v.validations[0].message}`);
  }
});
```

**Returns:** [`Promise<Array<ValidationResult>>`](#validationresult-structure)

---

#### `thread.getCompleteData(options)`

Get complete thread data with all nested steps, history, and validations in a single request.

```javascript
const data = await thread.getCompleteData({ stepHistoryLimit: 50, validationLimit: 10 });

console.log(`Thread: ${data.id}`);
console.log(`Status: ${data.status}`);
console.log(`Steps: ${data.steps.length}`);
console.log(`Validations: ${data.validationResults.length}`);

// Access nested data
data.steps.forEach(step => {
  console.log(`${step.stepName}: ${step.status}`);
  step.history.forEach(h => {
    console.log(`  ${h.timestamp}: ${h.status}`);
  });
});
```

**Returns:** [Complete thread object](#complete-thread-data-structure) with nested steps, history, and validations

**Benefits:**
- ✅ Single network request
- ✅ Atomic data snapshot
- ✅ Perfect for dashboards and audit trails

---

### ArchivedStep Methods

#### `step.history(options)`

Get execution history for this step.

```javascript
const steps = await thread.steps('order_placed');
const step = steps[0];
const history = await step.history({ limit: 100 }); // all history
const filtered = await step.history({ limit: 10, activityType: 'step_recorded', startAt: '2026-01-01T00:00:00Z' }); // filtered
```

**Returns:** [`Promise<Array<StepHistory>>`](#stephistory-structure)

**Important:** The `status` field in step history shows the **step execution status** (`success`, `failed`, `error`) that you set when recording the step. Contract violations are tracked separately in `thread.validationResults()` and do **not** change the step status. A step can have `status: 'success'` but still have validation violations.

---

### Complete Thread Audit Trail

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

### Find Threads by Reference

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

### Step-Level Analysis

```javascript
const thread = await connection.getThread('thread-uuid');
const steps = await thread.steps('payment_processing');
const step = steps[0]; // Get first matching step

// Get detailed history
const history = await step.history({ limit: 50 });

console.log(`Payment Processing - ${history.length} attempts`);

const failures = history.filter(h => h.status === 'failed');
console.log(`Failed attempts: ${failures.length}`);

failures.forEach(f => {
  console.log(`  ${f.timestamp}: ${f.error}`);
});
```

### Understanding Step Status vs Violations

```javascript
const thread = await connection.getThread('thread-uuid');

// Step status shows execution outcome (what YOU set)
const steps = await thread.steps('order_placed');
console.log(`Step status: ${steps[0].status}`); // "success", "failed", or "error"

// Validation results show contract violations (what Threadify detected)
const validations = await thread.validationResults();
validations.forEach(v => {
  console.log(`Validation: ${v.overallStatus}`); // "critical", "warning", or "info"
  console.log(`Step was: ${v.stepName}`);
});

// Example: A step can succeed but violate contract rules
// - Step status: "success" (payment processed successfully)
// - Validation: "critical" (invalid transition, missing required field, timeout, etc.)
```

---

## Real-Time Notifications

Threadify provides a push-based notification system for real-time validation alerts. Notifications are delivered via WebSocket with automatic deduplication and flow control.

### Connecting with Notifications

Notifications are enabled automatically when you connect. Use the `maxInFlight` option to control flow (default: 10, max: 100).

---

### Subscribing to Notifications

Subscribe to events using the `.on()` method with event patterns:

**Event Patterns:**
- **`step.success`** - Step executed successfully
- **`step.failed`** - Step execution failed
- **`rule.violated`** - Contract validation violated
- **`rule.passed`** - Contract validation passed
- **`step.*`** - All step execution events
- **`rule.*`** - All validation events
- **`*`** - All events (execution + validation)

```javascript
// Subscribe to step execution events
connection.on('step.success', 'order_placed', (notification) => {
  console.log('Order placed successfully');
  notification.ack(); // IMPORTANT: Must ACK
});

connection.on('step.failed', 'payment_processed', (notification) => {
  console.error('Payment failed:', notification.message);
  notification.ack();
});

// Subscribe to contract validation events
connection.on('rule.violated', 'order_placed', (notification) => {
  console.error('Validation violation:', notification.message);
  notification.ack();
});

connection.on('rule.passed', 'order_placed', (notification) => {
  console.log('Validation passed');
  notification.ack();
});

// Wildcards - subscribe to all events for a step
connection.on('*', 'order_placed', (notification) => {
  console.log('Any event on order_placed:', notification);
  notification.ack();
});

// Contract-specific subscriptions
connection.on('rule.violated', 'product_delivery@order_placed', (notification) => {
  console.log('Product delivery contract violation');
  notification.ack();
});
```

---

### Notification Object

Each notification has the following properties:

```javascript
{
  notificationId: 'uuid',           // Unique notification ID
  threadId: 'uuid',                 // Thread ID
  stepId: 'uuid',                   // Step ID
  stepName: 'order_placed',         // Step name
  ownerId: 'user-123',              // Owner ID
  contractName: 'product_delivery', // Contract name (or empty)
  stepStatus: 'success',            // Step status: success, failed, error
  status: 'violated',               // Validation status: passed, violated
  violationType: 'timeout',         // Type of violation (if any)
  severity: 'critical',             // Severity: info, warning, critical
  message: 'Step timeout exceeded', // Human-readable message
  details: {},                      // Additional details
  timestamp: '2026-01-19T...',      // ISO timestamp
  
  // Methods
  ack()                             // Acknowledge notification
}
```

---

### Notification Methods

#### `notification.ack()`

Acknowledge receipt and processing of the notification. **You must call this** to prevent redelivery.

```javascript
connection.on('rule.violated', 'order_placed', (notification) => {
  // Process the notification
  logToDatabase(notification);
  
  // ACK to confirm processing
  notification.ack();
});
```

**Important:**
- ⚠️ If you don't ACK within 30 seconds, the notification will be redelivered
- ⚠️ After 3 failed deliveries, the notification moves to the Dead Letter Queue
- ✅ ACK is idempotent - safe to call multiple times

---

### Subscription Patterns

**Step execution events:**
```javascript
connection.on('step.success', 'order_placed', handler);
connection.on('step.failed', 'order_placed', handler);
```

**Contract validation events:**
```javascript
connection.on('rule.violated', 'order_placed', handler);
connection.on('rule.passed', 'order_placed', handler);
```

**Wildcard patterns:**
```javascript
connection.on('step.*', 'order_placed', handler);  // All step events
connection.on('rule.*', 'order_placed', handler);  // All validation events
connection.on('*', 'order_placed', handler);       // All events
```

**Contract-specific subscriptions:**
```javascript
connection.on('rule.violated', 'product_delivery@order_placed', handler);
```

**Multiple subscriptions:**
```javascript
connection.on('step.success', 'order_placed', handleSuccess);
connection.on('step.failed', 'order_placed', handleFailure);
connection.on('rule.violated', 'order_placed', handleViolation);
```

---

### Flow Control & HPA Support

**Flow Control:** Set `maxInFlight` to limit pending notifications (prevents overwhelming client)

**HPA-Safe:** Each notification delivered to **exactly one pod** - no duplicate processing, automatic load balancing

**Error Handling:**
```javascript
connection.on('rule.violated', 'order_placed', async (notification) => {
  try {
    await processViolation(notification);
    notification.ack();  // ACK on success
  } catch (error) {
    // Don't ACK - notification redelivered after 30s (max 3 attempts)
  }
});
```

---

## Data Structures

Detailed structure of objects returned by data retrieval methods.

### ArchivedThread Structure

Returned by `getThread()` and `getThreadsByRef()`.

```javascript
{
  id: string,
  contractId: string,
  contractVersion: string,
  contractName: string,
  ownerId: string,
  companyId: string,
  status: 'active' | 'completed' | 'failed',
  lastHash: string,
  refs: { [key: string]: string },  // External references
  startedAt: string,                 // ISO timestamp
  completedAt: string,               // ISO timestamp
  error: string                      // Error message if failed
}
```

---

### ArchivedStep Structure

Returned by `thread.steps()`.

```javascript
{
  threadId: string,
  stepName: string,
  idempotencyKey: string,
  status: 'success' | 'failed' | 'error',
  retryCount: number,
  firstSeenAt: string,    // ISO timestamp
  lastUpdatedAt: string,  // ISO timestamp
  latestStepID: string,   // UUID of latest attempt
  previousStep: string    // Previous step name
}
```

---

### ValidationResult Structure

Returned by `thread.validationResults()`.

```javascript
{
  validationId: string,
  threadId: string,
  stepName: string,
  timestamp: string,      // ISO timestamp
  overallStatus: 'critical' | 'warning' | 'info',
  hasCriticalViolation: boolean,
  criticalCount: number,
  warningCount: number,
  validations: [{
    type: string,        // e.g., 'invalid_transition', 'timeout', 'missing_field'
    message: string,     // Human-readable description
    severity: 'critical' | 'warning' | 'info',
    field: string,       // Affected field (if applicable)
    expected: any,       // Expected value
    actual: any          // Actual value
  }]
}
```

---

### StepHistory Structure

Returned by `step.history()`.

```javascript
{
  attempt: number,        // Retry attempt number
  timestamp: string,      // ISO timestamp
  status: 'success' | 'failed' | 'error',
  context: object,        // Business context data
  duration: number,       // Execution time in ms
  error: string          // Error message if failed
}
```

---

### Complete Thread Data Structure

Returned by `thread.getCompleteData()`.

```javascript
{
  // Thread metadata (same as ArchivedThread)
  id: string,
  contractId: string,
  contractVersion: string,
  contractName: string,
  ownerId: string,
  companyId: string,
  status: 'active' | 'completed' | 'failed',
  lastHash: string,
  refs: { [key: string]: string },
  startedAt: string,
  completedAt: string,
  error: string,
  
  // Nested step data
  steps: [{
    threadId: string,
    stepName: string,
    idempotencyKey: string,
    status: string,
    retryCount: number,
    firstSeenAt: string,
    lastUpdatedAt: string,
    latestStepID: string,
    previousStep: string,
    
    // Nested history for each step
    history: [{
      attempt: number,
      timestamp: string,
      status: string,
      context: object,
      duration: number,
      error: string
    }]
  }],
  
  // Nested validation results
  validationResults: [{
    validationId: string,
    threadId: string,
    stepName: string,
    timestamp: string,
    overallStatus: string,
    hasCriticalViolation: boolean,
    criticalCount: number,
    warningCount: number,
    validations: [{
      type: string,
      message: string,
      severity: string,
      field: string,
      expected: any,
      actual: any
    }]
  }]
}
```

---

## Support

For issues, questions, or contributions:
- Email: [support@threadify.dev](mailto:support@threadify.dev)

---
