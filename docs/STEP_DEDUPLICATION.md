# Step Deduplication with Idempotency Keys

## Overview

Threadify implements step deduplication using idempotency keys while maintaining its core principle: **an immutable, append-only event stream that represents the universal truth of business service execution**.

## Core Architecture

### Thread Object
- **Purpose**: Metadata container
- **Contains**: ID, owner, contract, status, deduplication index
- **NOT**: The source of truth for execution history

### Step Events (Activity Queue)
- **Purpose**: Immutable audit trail
- **Storage**: Valkey list (append-only)
- **Contains**: Complete execution history with cryptographic hashing
- **Principle**: Every action is recorded, nothing is modified

### Steps Map (Deduplication Index)
- **Purpose**: Fast duplicate detection (O(1) lookup)
- **Storage**: In-memory + Valkey (part of Thread object)
- **Contains**: Latest state per idempotency key
- **Key Format**: `stepName:idempotencyKey`

## How It Works

### 1. Idempotency Key Generation

**SDK (Automatic):**
```javascript
// Hash generated from stepName + context
await thread.step('payment')
  .addContext({ orderId: 'PO-123', amount: '100' })
  .stop('success');
// idempotencyKey = hash('payment' + '{"amount":"100","orderId":"PO-123"}')
```

**SDK (Manual Override):**
```javascript
await thread.step('payment')
  .addContext({ orderId: 'PO-123' })
  .idempotencyKey('stripe-txn-xyz789')
  .stop('success');
```

### 2. Deduplication Logic

```
Incoming: stepName='payment', idempotencyKey='abc123', status='success'

1. Check index: thread.Steps['payment:abc123']
   
   A. Not found?
      → Append event to activity queue
      → Add to index: Steps['payment:abc123'] = {stepId, status, ...}
      → Response: success
   
   B. Found with status='failed'?
      → Append NEW event to activity queue (retry)
      → Update index: Steps['payment:abc123'] = {new stepId, status='success', retryCount++}
      → Response: success (retry succeeded)
   
   C. Found with status='success'?
      → DO NOT append event
      → Response: error (duplicate detected)
```

### 3. Example: Failed Step Retry

**First Attempt (Fails):**
```
Event appended to queue:
{
  stepId: 'uuid-1',
  stepName: 'payment',
  status: 'failed',
  context: {orderId: 'PO-123', amount: '100'},
  timestamp: '2025-01-15T10:00:00Z',
  hash: 'hash-1',
  prevHash: 'hash-0'
}

Index updated:
Steps['payment:abc123'] = {
  stepId: 'uuid-1',
  status: 'failed',
  idempotencyKey: 'abc123',
  retryCount: 0
}
```

**Retry (Same Context):**
```
Event appended to queue:
{
  stepId: 'uuid-2',  // NEW EVENT
  stepName: 'payment',
  status: 'success',
  context: {orderId: 'PO-123', amount: '100'},
  timestamp: '2025-01-15T10:05:00Z',
  hash: 'hash-2',
  prevHash: 'hash-1'  // Links to previous
}

Index updated:
Steps['payment:abc123'] = {
  stepId: 'uuid-2',  // Points to latest
  status: 'success',
  idempotencyKey: 'abc123',
  retryCount: 1
}
```

**Result:**
- ✅ Two events in activity queue (complete audit trail)
- ✅ Cryptographic chain maintained
- ✅ Index points to latest successful attempt
- ✅ Full retry history preserved

### 4. Example: Duplicate Prevention

**First Payment:**
```
Event appended:
{stepId: 'uuid-1', stepName: 'payment', status: 'success', ...}

Index:
Steps['payment:abc123'] = {stepId: 'uuid-1', status: 'success'}
```

**Duplicate Attempt (Same Context):**
```
Check index: Steps['payment:abc123'] exists with status='success'
→ REJECT: No event appended
→ Response: {status: 'error', isDuplicate: true}
→ SDK: Logs warning, doesn't throw
```

**Result:**
- ✅ Duplicate prevented
- ✅ No pollution of event stream
- ✅ User notified via warning

### 5. Example: Multiple Payments (Different Context)

**First Payment:**
```
Context: {orderId: 'PO-123'}
IdempotencyKey: 'abc123'
Event appended, Index: Steps['payment:abc123']
```

**Second Payment (Different Order):**
```
Context: {orderId: 'PO-456'}
IdempotencyKey: 'xyz789'  // Different hash!
Event appended, Index: Steps['payment:xyz789']
```

**Result:**
- ✅ Two separate payment events
- ✅ Two separate index entries
- ✅ Both tracked independently

## Key Benefits

### 1. Immutable Event Stream
- Every action is recorded
- Complete audit trail preserved
- Cryptographic integrity maintained
- Pattern analysis enabled

### 2. Real-Time Violation Detection
- Contract rules checked on each event
- Duplicate detection before event append
- Role-based access control enforced
- Permission validation per step

### 3. Universal Truth Graph
- Single source of truth for execution
- Cross-system workflow visibility
- Human + agent + system actions tracked
- Customer request lifecycle captured

### 4. Performance
- O(1) duplicate detection (hash lookup)
- Three-tier caching (memory → Valkey → Postgres)
- Async event processing
- Batched writes

## Implementation Details

### SDK (JavaScript)

**ThreadStep.js:**
- `idempotencyKey(key)`: Manual override
- `_generateIdempotencyKey()`: FNV-1a hash algorithm
- Automatic inclusion in event payload
- Graceful duplicate error handling

### Backend (Go)

**Models:**
- `RecordEventRequest.IdempotencyKey`: Client-provided key
- `Thread.Steps`: Deduplication index map
- `StepState`: Index entry structure

**HandleRecordEvent:**
1. Check `thread.Steps[stepName:idempKey]`
2. If exists + success → reject
3. If exists + failed → allow (append new event)
4. Append event to activity queue (always immutable)
5. Update index (in-memory tracking)
6. Save thread to cache + Valkey

## Storage Layout

### Valkey Keys

```
# Thread metadata + deduplication index
thread:{threadID} → JSON {
  id, owner, contract, status,
  steps: {
    "payment:abc123": {stepId, status, idempotencyKey, retryCount},
    "payment:xyz789": {stepId, status, idempotencyKey, retryCount}
  }
}

# Immutable activity queue (append-only)
thread:{threadID}:activity → List [
  {stepId: uuid-1, stepName: payment, status: failed, hash: ...},
  {stepId: uuid-2, stepName: payment, status: success, hash: ...},
  {stepId: uuid-3, stepName: notification, status: success, hash: ...}
]
```

## Philosophy

> **Threadify is not a dashboard that looks for patterns in your data. It's a universal thread that holds all truth about how you executed a business service on a singular basis.**

The deduplication index enables:
- ✅ Fast duplicate prevention
- ✅ Efficient retry handling
- ✅ Real-time violation detection

While preserving:
- ✅ Complete execution history
- ✅ Immutable event stream
- ✅ Cryptographic integrity
- ✅ Cross-system visibility

The `Steps` map is a **performance optimization** and **safety mechanism**, not a replacement for the event stream. The activity queue remains the single source of truth.
