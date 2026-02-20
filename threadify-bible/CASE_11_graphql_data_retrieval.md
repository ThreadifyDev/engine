# CASE 11: GraphQL Data Retrieval

**Handler**: GraphQL Query Resolvers  
**Purpose**: Retrieve archived thread data with cryptographic verification  
**Protocol**: HTTP POST (GraphQL)  
**Authentication**: JWT via `X-API-Key` header  

---

## Overview

GraphQL provides read-only access to archived thread data with:
- Cache-aside pattern (Valkey → PostgreSQL fallback)
- Lazy-loaded cryptographic verification
- Flexible filtering and pagination
- Batch-optimized queries to prevent N+1 problems

---

## Request Flow

### 1. Query: `thread(id: ID!)`

Retrieve a single thread with optional nested data (steps, validations, hash chain status).

**GraphQL Query Example**:
```graphql
query GetThread($id: ID!) {
  thread(id: $id) {
    id
    contractId
    contractName
    status
    lastHash
    startedAt
    completedAt
    
    steps(status: "failed") {
      stepName
      status
      retryCount
      verified
      verificationError
      history(limit: 1) {
        attempt
        status
        timestamp
        duration
        context
      }
    }
    
    hashChainVerified
    hashChainStatus {
      verified
      totalEvents
      lastVerifiedAt
      brokenAt
      error
    }
  }
}
```

---

## Detailed Flow

### Phase 1: Authentication & Authorization

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Extract JWT from X-API-Key header                        │
│ 2. Validate JWT signature and expiration                    │
│ 3. Extract ownerID, companyID, serviceName from claims      │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. Call ThreadRepository.GetThreadWithCache(ctx, threadID)  │
│    - Check Valkey: thread:{id}                              │
│    - If miss: Query PostgreSQL threads table                │
│    - Async write-back to Valkey (7-day TTL)                 │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 5. Access Control Check                                     │
│    - ThreadAccessService.CheckThreadAccess(threadID, ownerID, "read") │
│    - Check if user is owner OR has invitation-based access  │
│    - Cache result in context for child resolvers            │
└─────────────────────────────────────────────────────────────┘
                            ↓
                    ┌───────────────┐
                    │ Access Denied?│
                    └───────┬───────┘
                            │
                ┌───────────┴───────────┐
                │ Yes                   │ No
                ↓                       ↓
        Return Error 403        Continue to Phase 2
```

---

### Phase 2: Nested Field Resolution (Steps)

**Resolver**: `Thread.steps(stepName, idempotencyKey, status)`

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Check if steps were batch-loaded (from threads query)    │
│    - If cached: Apply filters in-memory                     │
│    - If not: Query StepStateRepository.ListSteps()          │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. StepStateRepository.ListSteps(threadID, filters)         │
│    - Check Valkey: KEYS thread:{id}:steps:*                 │
│    - If miss: Query PostgreSQL thread_step_states table     │
│    - Apply filters: stepName, idempotencyKey, status        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. For each step, resolve nested fields:                    │
│    - verified (lazy-loaded)                                 │
│    - verificationError (lazy-loaded)                        │
│    - history (nested query)                                 │
└─────────────────────────────────────────────────────────────┘
```

---

### Phase 3: Cryptographic Verification (Lazy-Loaded)

**Resolver**: `StepStateInfo.verified` and `StepStateInfo.verificationError`

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Check if verification was requested (field in query)     │
│    - If not requested: Skip (lazy-loaded)                   │
│    - If requested: Proceed with verification                │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Load step history from PostgreSQL                        │
│    - Query thread_activities table                          │
│    - Filter by threadID, stepName, idempotencyKey           │
│    - Order by timestamp ASC                                 │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. Verify HMAC hash chain                                   │
│    - Load hash chain secret for version (from config)       │
│    - Compute HMAC-SHA256 for each event                     │
│    - Verify: HMAC(prevHash + eventData) == event.hash       │
│    - If mismatch: Set verified=false, verificationError     │
└─────────────────────────────────────────────────────────────┘
                            ↓
                    ┌───────────────┐
                    │ Verified?     │
                    └───────┬───────┘
                            │
                ┌───────────┴───────────┐
                │ Yes                   │ No
                ↓                       ↓
        verified: true          verified: false
        verificationError: null verificationError: "Hash mismatch at event X"
```

---

### Phase 4: Step History Resolution

**Root Query**: `stepHistory(threadId, stepName, idempotencyKey, ...)`

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Query PostgreSQL thread_activities table                 │
│    - Filter: threadID, stepName, idempotencyKey             │
│    - Optional filters: startAt, endAt, activityType, actor  │
│    - Pagination: limit (default 100), offset (default 0)    │
│    - Order by: timestamp DESC                               │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Transform to StepHistory objects                         │
│    - Extract: attempt, timestamp, status, context, duration │
│    - Parse context JSON                                     │
│    - Return array of StepHistory                            │
└─────────────────────────────────────────────────────────────┘
```

**Note**: Step history is NOT cached - always queries PostgreSQL for audit trail integrity.

---

## Database Operations

### Valkey (Hot Cache)

**Read Operations**:
1. `GET thread:{threadID}` - Thread metadata
2. `HGETALL thread:{threadID}:meta` - Thread metadata (alternative format)
3. `KEYS thread:{threadID}:steps:*` - List all steps
4. `HGETALL thread:{threadID}:steps:{stepName}:{idempKey}` - Step state

**Write Operations** (Async write-back on cache miss):
5. `SETEX thread:{threadID} 604800 {json}` - Cache thread (7 days)
6. `HSET thread:{threadID}:steps:{stepName}:{idempKey} {fields}` - Cache step state

---

### PostgreSQL (Cold Storage)

**Read Operations**:
1. `SELECT * FROM threads WHERE id = $1` - Thread metadata
2. `SELECT * FROM thread_step_states WHERE thread_id = $1 [AND filters]` - Step states
3. `SELECT * FROM thread_activities WHERE thread_id = $1 AND step_name = $2 [AND filters] ORDER BY timestamp` - Step history
4. `SELECT * FROM validation_results WHERE thread_id = $1 [AND filters]` - Validation results

**No Write Operations** - GraphQL is read-only

---

## GraphQL Schema

### Core Types

```graphql
type Thread {
  id: ID!
  contractId: String
  contractName: String
  contractVersion: Int
  ownerId: String!
  companyId: String!
  status: String!
  # HMAC hash of the last activity event in the thread's hash chain
  lastHash: String
  refs: JSON
  startedAt: String
  completedAt: String
  error: String
  
  # Nested queries
  steps(stepName: String, idempotencyKey: String, status: String): [StepStateInfo!]!
  validationResults(options: ValidationQueryOptions): [ValidationResultInfo!]!
  threadChain(maxDepth: Int = 3): [Thread!]!
  
  # Lazy-loaded verification
  hashChainVerified: Boolean
  hashChainStatus: HashChainStatus
}

type StepStateInfo {
  threadId: String!
  stepName: String!
  idempotencyKey: String!
  status: String!
  retryCount: Int!
  firstSeenAt: String!
  lastUpdatedAt: String!
  latestStepID: String!
  previousStep: String
  
  # Lazy-loaded cryptographic verification
  verified: Boolean
  verificationError: String
  
  # Nested query for step execution history
  history(limit: Int = 100, offset: Int = 0, startAt: String, endAt: String, activityType: String, actor: String): [StepHistory!]!
}

type StepHistory {
  attempt: Int!
  timestamp: String!
  status: String!
  context: String!  # JSON string
  duration: Int!
  error: String
}

type HashChainStatus {
  verified: Boolean!
  lastVerifiedAt: String!
  totalEvents: Int!
  brokenAt: String
  error: String
}
```

---

## Query Patterns

### Pattern 1: Simple Thread Retrieval

```graphql
query GetThread($id: ID!) {
  thread(id: $id) {
    id
    status
    startedAt
    completedAt
  }
}
```

**Performance**: O(1) - Single Valkey GET or PostgreSQL SELECT

---

### Pattern 2: Thread with Steps (No Verification)

```graphql
query GetThreadWithSteps($id: ID!) {
  thread(id: $id) {
    id
    status
    steps {
      stepName
      status
      retryCount
      lastUpdatedAt
    }
  }
}
```

**Performance**: O(n) where n = number of steps
- Thread: O(1) Valkey or PostgreSQL
- Steps: O(n) Valkey KEYS + HGETALL or PostgreSQL SELECT

---

### Pattern 3: Thread with Verified Steps and History

```graphql
query GetThreadWithVerification($id: ID!) {
  thread(id: $id) {
    id
    status
    steps {
      stepName
      status
      verified          # Triggers verification
      verificationError # Triggers verification
      history(limit: 1) {
        attempt
        status
        timestamp
      }
    }
  }
}
```

**Performance**: O(n*m) where n = steps, m = events per step
- Thread: O(1)
- Steps: O(n)
- Verification per step: O(m) PostgreSQL query + HMAC computation

**Warning**: Expensive query - use sparingly or with filters

---

### Pattern 4: Filtered Steps with Status

```graphql
query GetFailedSteps($id: ID!) {
  thread(id: $id) {
    steps(status: "failed") {
      stepName
      retryCount
      history(limit: 5) {
        attempt
        status
        error
        timestamp
      }
    }
  }
}
```

**Performance**: O(n*m) but filtered
- Only failed steps are processed
- History limited to 5 records per step

---

### Pattern 5: Complete Thread Data (Bulk Query)

```graphql
query GetCompleteThread($id: ID!) {
  thread(id: $id) {
    id
    contractName
    status
    lastHash
    
    steps(stepName: "payment_processed") {
      stepName
      status
      retryCount
      verified
      history(limit: 10) {
        attempt
        status
        context
        duration
      }
    }
    
    validationResults(options: {limit: 10}) {
      overallStatus
      hasCriticalViolation
      validations {
        type
        message
        field
      }
    }
    
    hashChainVerified
    hashChainStatus {
      verified
      totalEvents
      error
    }
  }
}
```

**Performance**: O(n*m + v) where n = steps, m = events, v = validations
- Single HTTP request
- Multiple nested queries
- Use for dashboards, not real-time polling

---

## SDK Integration

### JavaScript SDK Usage

```javascript
import Threadify from 'threadify-sdk';

// Connect with GraphQL support
const connection = await Threadify.connect(
  'api-key-123',
  'my-service',
  {
    wsUrl: 'ws://localhost:8081/threads',
    graphqlUrl: 'http://localhost:8081/graphql'
  }
);

// Get archived thread
const thread = await connection.getThread('thread-id-123');

// Get all steps with verification
const steps = await thread.steps();
for (const step of steps) {
  console.log(`Step: ${step.stepName}`);
  console.log(`Verified: ${step.verified}`);
  console.log(`Last Execution:`, step.lastExecution);
}

// Get specific step with full history
const paymentStep = await thread.getStep('payment_processed');
const history = await paymentStep.history({ limit: 10 });

// Filter steps by status
const failedSteps = await thread.steps({ status: 'failed' });

// Get complete thread data (efficient bulk query)
const completeData = await thread.getCompleteData({
  stepName: 'payment_processed',
  status: 'failed',
  stepHistoryLimit: 5,
  validationLimit: 10
});
```

---

## Performance Characteristics

### Cache Hit (Hot Path)

| Operation | Latency | Database |
|-----------|---------|----------|
| Get thread | 1-2ms | Valkey |
| Get steps (10) | 5-10ms | Valkey |
| Total | 6-12ms | Valkey only |

### Cache Miss (Cold Path)

| Operation | Latency | Database |
|-----------|---------|----------|
| Get thread | 10-20ms | PostgreSQL |
| Get steps (10) | 20-40ms | PostgreSQL |
| Write-back | 5-10ms (async) | Valkey |
| Total | 30-60ms | PostgreSQL + Valkey |

### Verification (Expensive)

| Operation | Latency | Database |
|-----------|---------|----------|
| Load history (100 events) | 20-50ms | PostgreSQL |
| HMAC computation (100 events) | 5-10ms | CPU |
| Total per step | 25-60ms | PostgreSQL + CPU |

**Recommendation**: Only request `verified` field when needed (e.g., audit, investigation)

---

## Security Considerations

### 1. Access Control

- Every query checks `CheckThreadAccess(threadID, ownerID, "read")`
- Supports ownership-based and invitation-based access
- Access check result cached in context for nested resolvers

### 2. Data Leakage Prevention

- No debug logging of user IDs or thread IDs
- Error messages are user-friendly, not exposing internals
- Confidential fields (e.g., internal hashes) not exposed unless needed

### 3. Rate Limiting

- GraphQL queries count against API key rate limits
- Complex queries (with verification) consume more quota
- Consider implementing query complexity analysis

### 4. Cryptographic Verification

- HMAC-SHA256 with versioned secrets
- Supports key rotation (v1, v2, etc.)
- Lazy-loaded to avoid performance impact
- Detects tampering in archived data

---

## Error Handling

### Common Errors

**1. Thread Not Found**
```json
{
  "errors": [{
    "message": "thread not found: thread-id-123",
    "path": ["thread"]
  }]
}
```

**2. Access Denied**
```json
{
  "errors": [{
    "message": "access denied: you don't have permission to view this thread",
    "path": ["thread"]
  }]
}
```

**3. Authentication Required**
```json
{
  "errors": [{
    "message": "authentication required: invalid or missing API key",
    "path": ["thread"]
  }]
}
```

**4. Verification Failed**
```json
{
  "data": {
    "thread": {
      "steps": [{
        "verified": false,
        "verificationError": "Hash mismatch at event 42: expected abc123, got def456"
      }]
    }
  }
}
```

---

## Monitoring & Observability

### Key Metrics

1. **Cache Hit Rate**: `valkey_cache_hits / (valkey_cache_hits + valkey_cache_misses)`
2. **Query Latency**: p50, p95, p99 for each query type
3. **Verification Requests**: Count of queries requesting `verified` field
4. **Access Denied Rate**: Failed authorization attempts

### Logging

- Query type and complexity (field count)
- Cache hit/miss for thread and steps
- PostgreSQL fallback triggers
- Verification requests and results
- Access control decisions (without user IDs)

---

## Best Practices

### For Clients

1. **Use Filters**: Always filter steps by `stepName` or `status` when possible
2. **Limit History**: Use `limit` parameter for step history (default: 100)
3. **Lazy Verification**: Only request `verified` field when investigating issues
4. **Batch Queries**: Use `getCompleteData()` for bulk fetching instead of multiple queries
5. **Pagination**: Use `offset` for large result sets

### For Backend

1. **Cache Warming**: Pre-populate Valkey for active threads
2. **Index Optimization**: Ensure PostgreSQL indexes on `thread_id`, `step_name`, `timestamp`
3. **Query Complexity**: Consider implementing query cost analysis
4. **Batch Loading**: Use DataLoader pattern to prevent N+1 queries
5. **Monitoring**: Track slow queries and cache miss rates

---

## Related Documentation

- **[CASE_3_recordThreadEvent.md](./CASE_3_recordThreadEvent.md)** - How step data is written
- **[ThreadifyArchiver.md](./ThreadifyArchiver.md)** - How data is archived to PostgreSQL
- **[SDK Documentation](../threadify-sdk/Documentation.md)** - Client SDK usage

---

*Last Updated: January 16, 2026 - Added GraphQL data retrieval with cryptographic verification*
