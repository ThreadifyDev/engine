# CASE 10: addRefs - Add External References to Thread

## Overview

The `addRefs` action allows clients to add external system references (e.g., order IDs, customer IDs, invoice IDs) to an existing thread. This is a standalone operation independent of step recording, enabling flexible metadata management throughout a thread's lifecycle.

## Request Format

```json
{
  "action": "addRefs",
  "threadId": "039841c7-79a5-4594-99a2-a9bb00c2df35",
  "refs": {
    "orderId": "ORD-12345",
    "customerId": "CUST-67890",
    "invoiceId": "INV-11111"
  }
}
```

## Response Format

### Success Response
```json
{
  "action": "addRefs",
  "status": "success",
  "message": "Added 3 refs to thread",
  "threadId": "039841c7-79a5-4594-99a2-a9bb00c2df35"
}
```

### Error Response
```json
{
  "action": "addRefs",
  "status": "error",
  "message": "Thread not found"
}
```

## Complete Flow

### Phase 1: Request Validation

```
1. WebSocket Handler receives message
   ├─ Parse JSON: action = "addRefs"
   ├─ Extract threadId and refs map
   └─ Call ThreadService.HandleAddRefs(req, ownerID)

2. ThreadService.HandleAddRefs()
   ├─ Validate threadId is not empty
   ├─ Validate refs map is not empty
   ├─ Verify thread exists: ThreadRepository.Get(threadId)
   ├─ Verify ownership: thread.OwnerID == ownerID
   └─ If validation fails → Return error response
```

### Phase 2: Valkey Write (Hot Storage)

```
3. ThreadRepository.AddRefs(threadId, refs)
   ├─ Get metadata key: "thread:{threadId}:meta"
   ├─ Start Valkey pipeline
   ├─ For each ref (key, value):
   │  └─ HSET thread:{threadId}:meta "refs:{key}" "{value}"
   ├─ EXPIRE thread:{threadId}:meta {TTL}
   └─ Execute pipeline atomically

Result: Refs stored in Valkey hash
  - Key: thread:039841c7-79a5-4594-99a2-a9bb00c2df35:meta
  - Fields:
    * refs:orderId → "ORD-12345"
    * refs:customerId → "CUST-67890"
    * refs:invoiceId → "INV-11111"
```

### Phase 3: NATS Publishing (Async Archival)

```
4. Publish to NATS JetStream (Async Goroutine)
   ├─ For each ref (key, value):
   │  ├─ Create event:
   │  │  {
   │  │    "threadId": "039841c7-79a5-4594-99a2-a9bb00c2df35",
   │  │    "refKey": "orderId",
   │  │    "refValue": "ORD-12345",
   │  │    "action": "ref_added"
   │  │  }
   │  ├─ Publish to subject: "metadata.thread"
   │  └─ Log success/failure
   └─ Context timeout: 5 seconds

Result: 3 messages published to NATS stream "ARCHIVAL"
  - Subject: metadata.thread
  - Consumer group: "archivers" (will consume later)
```

### Phase 4: Response to Client

```
5. Return Success Response
   ├─ Build AddRefsResponse:
   │  ├─ action: "addRefs"
   │  ├─ status: "success"
   │  ├─ message: "Added 3 refs to thread"
   │  └─ threadId: "039841c7-79a5-4594-99a2-a9bb00c2df35"
   └─ Send JSON response via WebSocket
```

### Phase 5: Archival to PostgreSQL (Eventually Consistent)

```
6. Archiver Service (Separate Process)
   ├─ NATS Consumer reads from "metadata.thread"
   ├─ Batch messages (up to batch_size or flush_interval)
   ├─ Filter events where action == "ref_added"
   ├─ Call PostgresWriter.WriteThreadRefs(events)
   │
   └─ For each ref event:
      ├─ Extract: threadId, refKey, refValue
      └─ Execute SQL:
         INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
         VALUES ($1, $2, $3, NOW(), NOW())
         ON CONFLICT (thread_id, ref_key) DO UPDATE SET
           ref_value = EXCLUDED.ref_value,
           updated_at = NOW()

Result: Refs persisted in PostgreSQL
  - Table: thread_refs
  - Rows:
    * (039841c7..., 'orderId', 'ORD-12345', NOW(), NOW())
    * (039841c7..., 'customerId', 'CUST-67890', NOW(), NOW())
    * (039841c7..., 'invoiceId', 'INV-11111', NOW(), NOW())
```

## Database Impact

### Valkey (Immediate - Hot Storage)
```
Key: thread:039841c7-79a5-4594-99a2-a9bb00c2df35:meta
Type: Hash
Fields:
  refs:orderId → "ORD-12345"
  refs:customerId → "CUST-67890"
  refs:invoiceId → "INV-11111"
TTL: 7 days (configurable)
```

### NATS JetStream (Async - Reliable Queue)
```
Stream: ARCHIVAL
Subject: metadata.thread
Messages: 3 (one per ref)
Consumer Group: archivers
Retention: Until ACK'd by archiver
```

### PostgreSQL (Eventually Consistent - Cold Storage)
```sql
-- Table: thread_refs
-- Constraint: UNIQUE(thread_id, ref_key)

SELECT * FROM thread_refs WHERE thread_id = '039841c7-79a5-4594-99a2-a9bb00c2df35';

 thread_id                              | ref_key      | ref_value    | created_at | updated_at
----------------------------------------+--------------+--------------+------------+------------
 039841c7-79a5-4594-99a2-a9bb00c2df35  | customerId   | CUST-67890   | ...        | ...
 039841c7-79a5-4594-99a2-a9bb00c2df35  | invoiceId    | INV-11111    | ...        | ...
 039841c7-79a5-4594-99a2-a9bb00c2df35  | orderId      | ORD-12345    | ...        | ...
```

## Error Handling

### 1. Thread ID Missing
```json
{
  "action": "addRefs",
  "status": "error",
  "message": "Thread ID is required"
}
```

### 2. Empty Refs Map
```json
{
  "action": "addRefs",
  "status": "error",
  "message": "At least one ref is required"
}
```

### 3. Thread Not Found
```json
{
  "action": "addRefs",
  "status": "error",
  "message": "Thread not found"
}
```

### 4. Access Denied
```json
{
  "action": "addRefs",
  "status": "error",
  "message": "Access denied"
}
```

### 5. Valkey Write Failure
```json
{
  "action": "addRefs",
  "status": "error",
  "message": "Failed to store refs: {error details}"
}
```

## Code Path

### Handler Entry Point
```go
// File: internal/handlers/thread.go
case "addRefs":
    var req models.AddRefsRequest
    json.Unmarshal(msgBytes, &req)
    response = h.threadService.HandleAddRefs(&req, session.ownerID)
```

### Service Layer
```go
// File: internal/service/thread.go
func (s *ThreadService) HandleAddRefs(req *models.AddRefsRequest, ownerID string) *models.AddRefsResponse {
    // 1. Validate request
    // 2. Verify thread exists and user has access
    // 3. Store refs in Valkey
    // 4. Publish to NATS (async)
    // 5. Return response
}
```

### Repository Layer
```go
// File: internal/repository/valkey/thread.go
func (r *ThreadRepository) AddRefs(ctx context.Context, threadID string, refs map[string]string) error {
    metaKey := r.getThreadMetaKey(threadID)
    pipe := r.valkey.Pipeline()
    
    for key, value := range refs {
        pipe.HSet(ctx, metaKey, "refs:"+key, value)
    }
    pipe.Expire(ctx, metaKey, time.Duration(r.ttl)*time.Second)
    
    _, err := pipe.Exec(ctx)
    return err
}
```

### Archiver Layer
```go
// File: internal/archiver/postgres_writer.go
func (w *PostgresWriter) WriteThreadRefs(ctx context.Context, events []StreamEvent) error {
    query := `
        INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
        ON CONFLICT (thread_id, ref_key) DO UPDATE SET
            ref_value = EXCLUDED.ref_value,
            updated_at = NOW()
    `
    
    for _, event := range events {
        _, err := w.db.Pool.Exec(ctx, query,
            event.Data["threadId"],
            event.Data["refKey"],
            event.Data["refValue"],
        )
        if err != nil {
            return err
        }
    }
    return nil
}
```

## SDK Usage

### JavaScript/Node.js SDK
```javascript
import Threadify from 'threadify-sdk';

// Connect and start thread
const connection = await Threadify.connect(API_KEY, 'service-name');
const thread = await connection.start('product_delivery', 'merchant');

// Add refs to thread
await thread.addRefs({
    orderId: 'ORD-12345',
    customerId: 'CUST-67890',
    invoiceId: 'INV-11111'
});

// Add more refs later
await thread.addRefs({
    shipmentId: 'SHIP-99999',
    trackingNumber: 'TRK-ABCDE'
});
```

### SDK Implementation
```javascript
// File: threadify-sdk/src/Thread.js
class ThreadInstance {
    async addRefs(refs) {
        if (!refs || typeof refs !== 'object' || Object.keys(refs).length === 0) {
            throw new Error('Refs must be a non-empty object');
        }

        return new Promise((resolve, reject) => {
            this._onceResponse((message) => {
                if (message.action === 'addRefs') {
                    if (message.status === 'success') {
                        this.refs = { ...this.refs, ...refs };
                        resolve(message);
                    } else {
                        reject(new Error(message.message || 'Failed to add refs'));
                    }
                }
            });

            this._send({
                action: 'addRefs',
                threadId: this.threadId,
                refs
            });
        });
    }
}
```

## Use Cases

### 1. Order Processing
```javascript
const thread = await connection.start('order_fulfillment', 'merchant');

// Link to external systems
await thread.addRefs({
    orderId: 'ORD-12345',
    customerId: 'CUST-67890',
    paymentId: 'PAY-11111'
});

// Later, add shipping info
await thread.addRefs({
    shipmentId: 'SHIP-99999',
    trackingNumber: 'TRK-ABCDE'
});
```

### 2. Multi-System Integration
```javascript
// Link thread to multiple external systems
await thread.addRefs({
    salesforceOpportunityId: 'SF-OPP-123',
    stripePaymentIntent: 'pi_abc123',
    hubspotDealId: 'DEAL-456',
    slackThreadTs: '1234567890.123456'
});
```

### 3. Incremental Reference Addition
```javascript
// Start with minimal refs
const thread = await connection.start('product_delivery', 'merchant');
await thread.addRefs({ orderId: 'ORD-12345' });

// Add customer info when available
await thread.addRefs({ customerId: 'CUST-67890' });

// Add payment info after processing
await thread.addRefs({ invoiceId: 'INV-11111' });
```

## Performance Characteristics

### Latency
- **Valkey Write**: < 5ms (synchronous)
- **NATS Publish**: < 10ms (async, non-blocking)
- **PostgreSQL Write**: 100ms - 5s (eventual consistency via archiver)
- **Total Client Response Time**: < 15ms

### Throughput
- **Concurrent Requests**: Limited by Valkey pipeline throughput
- **NATS Publishing**: Async, does not block client response
- **Archiver Batching**: Configurable batch size and flush interval

### Scalability
- **Horizontal**: Multiple server instances can handle addRefs
- **Vertical**: Valkey pipeline supports atomic multi-key writes
- **Archival**: NATS consumer groups enable multiple archiver instances

## Comparison with Legacy Approach

### Old Way (Deprecated)
```javascript
// Refs embedded in step recording
await thread.step('order_placed').success({
    context: { amount: '100.00' },
    refs: { orderId: 'ORD-12345' }  // ❌ Deprecated
});
```

### New Way (Recommended)
```javascript
// Refs as standalone operation
await thread.addRefs({ orderId: 'ORD-12345' });  // ✅ Preferred

await thread.step('order_placed').success({
    context: { amount: '100.00' }
});
```

### Benefits of New Approach
1. **Separation of Concerns**: Refs are thread-level metadata, not step-level
2. **Flexibility**: Add refs at any time, not just during step recording
3. **Cleaner API**: Dedicated method for refs management
4. **Better Semantics**: Refs represent external system links, not step data

## Related Cases

- **CASE_2_startThread.md**: Initial thread creation with refs
- **CASE_3_recordThreadEvent.md**: Deprecated refs field in step recording
- **ThreadifyArchiver.md**: NATS to PostgreSQL archival flow

## Monitoring & Observability

### Logs to Watch
```
✅ [NATS-ARCHIVAL] Published ref {refKey} for thread {threadId}
❌ ERROR: Failed to publish ref {refKey} to NATS: {error}
🔗 Writing {count} thread refs to Postgres
✅ SUCCESS: Successfully wrote {count} thread refs to Postgres
```

### Metrics to Track
- `addRefs_requests_total` - Total addRefs requests
- `addRefs_errors_total` - Failed addRefs requests
- `refs_valkey_write_duration_ms` - Valkey write latency
- `refs_nats_publish_duration_ms` - NATS publish latency
- `refs_archived_total` - Refs successfully archived to PostgreSQL

---

*Last Updated: January 12, 2026*
