# Threadify Archiver Service

## Overview

The **Archiver Service** is a background service that reads events from NATS JetStream and writes them to Postgres in batches. It ensures durable persistence of all thread activity while keeping the main API service fast and responsive.

**Migration Status (2026-01-06):** ✅ Fully migrated from Redis Streams to NATS JetStream

## Architecture

### Core Principle

```
NATS JetStream (Fast, Distributed) → Archiver → Postgres (Durable, Permanent)
```

- **API Service**: Publishes events to NATS JetStream (fast, non-blocking, async)
- **Archiver Service**: Consumes from NATS streams, batches, writes to Postgres
- **Separation**: API doesn't wait for Postgres, archiver handles persistence
- **Reliability**: NATS WorkQueue policy with ACK/NACK for guaranteed delivery

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Archiver Service                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  NATS JetStream Consumers (goroutines)                      │
│  ├── Activity Log Consumer (activity.log)                   │
│  ├── Thread Metadata Consumer (metadata.thread)             │
│  ├── Thread Access Consumer (access.thread)                 │
│  └── Validations Consumer (validations.thread)              │
│                                                              │
│  Batch Processing (NATS native)                             │
│  ├── Batch size: 10 messages                                │
│  ├── Timeout: 5 seconds                                     │
│  ├── Consumer group: archiver-nats-1                        │
│  └── WorkQueue policy (at-least-once delivery)              │
│                                                              │
│  Postgres Writer                                             │
│  ├── Multi-row INSERT for efficiency                        │
│  ├── Batch writes to Postgres                               │
│  ├── ACK on success, NACK on failure                        │
│  └── Performance metrics tracking                            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## How It Works

### 1. Event Flow

```
API Service:
  ↓ (NATS Publish - Async)
NATS JetStream: activity.log, metadata.thread, etc.
  ↓ (Consumer.Messages() iterator)
NATS Consumer: Reads batch of messages
  ↓ (Batch size: 10, Timeout: 5s)
Batch Processing: Accumulates messages
  ↓ (Unmarshal JSON, type conversion)
Postgres Writer: Multi-row INSERT
  ↓ (On success)
ACK: msg.Ack() for each message
  ↓ (On failure)
NACK: msg.Nak() - message redelivered
```

### 2. NATS Batch Processing

NATS consumer batches messages using **either** trigger:

**Size Trigger:**
- Batch size: 10 messages (configurable)

**Time Trigger:**
- Timeout: 5 seconds (configurable)

**Example:**
```
Low traffic: 3 messages in 5 seconds → Flush (time trigger)
High traffic: 10 messages in 2 seconds → Flush (size trigger)
```

### 3. NATS Streams & Subjects

| Stream Name | Subject | Purpose | Retention |
|-------------|---------|---------|-----------|
| activity_log | activity.log | Step events, state changes | 24 hours |
| thread_metadata | metadata.thread | Thread creation, completion | 24 hours |
| thread_access | access.thread | Access grants, permissions | 24 hours |
| thread_validations | validations.thread | Validation results | 24 hours |

**Stream Policy:** WorkQueue (at-least-once delivery)  
**Consumer Group:** archiver-nats-1

### 3. Retry Logic

**Exponential Backoff:**
```
Attempt 1: Immediate
Attempt 2: 1 second delay
Attempt 3: 2 seconds delay
Attempt 4: 4 seconds delay
Attempt 5: 8 seconds delay
Attempt 6: 16 seconds delay
Max: 6 attempts total
```

**On Failure:**
- If all retries fail: Log error, keep events in stream
- Events remain in Valkey (not ACKed)
- Next consumer read will retry same events
- No data loss

### 4. Consumer Groups

**Valkey Consumer Groups** enable horizontal scaling:

```
Single Instance:
Consumer: archiver-1
  ├── Reads from streams:step_events
  ├── Reads from streams:thread_metadata
  ├── Reads from streams:audit_logs
  └── Reads from streams:invitations

Multiple Instances (High Load):
Consumer: archiver-1
  └── Processes entries 1, 3, 5, 7...
Consumer: archiver-2
  └── Processes entries 2, 4, 6, 8...

Valkey distributes load automatically
No duplicate processing (consumer group guarantees)
```

## Configuration

### config.yaml

```yaml
archiver:
  enabled: true
  
  buffers:
    step_events:
      size: 100
      flush_interval_seconds: 5
    thread_metadata:
      size: 50
      flush_interval_seconds: 10
    audit_logs:
      size: 200
      flush_interval_seconds: 30
    invitations:
      size: 50
      flush_interval_seconds: 10
  
  retry:
    max_attempts: 6
    initial_backoff_seconds: 1
    max_backoff_seconds: 16
  
  streams:
    consumer_group: "archiver-group"
    block_timeout_seconds: 5
    batch_size: 100
```

## Code Structure

### Files

```
internal/archiver/
├── buffer.go          # Event buffer with size/time triggers
├── buffer_test.go     # Buffer tests
├── writer.go          # Batch writer with retry logic
├── writer_test.go     # Writer tests
├── consumer.go        # Stream consumer (TODO)
├── archiver.go        # Main orchestrator (TODO)
└── README.md          # This file
```

### Key Components

#### EventBuffer

**Purpose**: Thread-safe in-memory buffer for batching events

**Methods:**
- `Add(event)`: Add event to buffer
- `ShouldFlush()`: Check if flush needed (size or time)
- `GetAndClear()`: Get all events and clear buffer
- `Len()`: Current buffer size

**Used by:**
- `consumer.go`: Adds events from stream
- `writer.go`: Reads events for batch write

#### Writer

**Purpose**: Writes batches to Postgres with retry logic

**Methods:**
- `ProcessBatch(ctx, stream, group, events, writeFunc)`: Write batch and ACK

**Features:**
- Exponential backoff retry
- ACK only on success
- Context cancellation support

**Used by:**
- `archiver.go`: Orchestrates batch writes

#### ValkeyClient Interface

**Purpose**: Abstraction for Valkey stream operations

**Methods:**
- `XAck(ctx, stream, group, ids)`: Acknowledge stream entries

**Implemented by:**
- `internal/repository/valkey/client.go`

## Crash Recovery

### Scenario: Archiver Crashes

**State Before Crash:**
```
Buffer: 50 events (not flushed)
Valkey: Events not ACKed
Postgres: No writes yet
```

**On Restart:**
```
1. Consumer rejoins consumer group
2. Valkey returns pending entries (not ACKed)
3. Events re-added to buffer
4. Flush triggered
5. Write to Postgres
6. ACK entries
```

**Result:**
- ✅ No data loss (events stay in Valkey)
- ✅ At-least-once delivery
- ✅ Idempotent writes (upsert in Postgres)

## Monitoring

### Key Metrics

**Stream Health:**
- Stream length (pending entries)
- Consumer lag (time since oldest entry)
- Pending entries per consumer

**Buffer Health:**
- Buffer fill percentage
- Flush frequency
- Average batch size

**Write Performance:**
- Postgres write latency
- Transaction success rate
- Retry count
- Error rate

### Alerts

**Critical:**
- Stream length > 10,000 (falling behind)
- Postgres write failures > 10 in 1 minute
- Consumer lag > 5 minutes

**Warning:**
- Stream length > 5,000
- Retry rate > 5%
- Buffer flush time > 2x expected

## Deployment

### Build

```bash
# Build archiver binary
go build -o bin/archiver ./cmd/archiver

# Build with API in same image
go build -o bin/server ./cmd/server
go build -o bin/archiver ./cmd/archiver
```

### Run

```bash
# Run archiver
./bin/archiver --config=config/config.yaml

# With environment overrides
ARCHIVER_BUFFERS_STEP_EVENTS_SIZE=200 ./bin/archiver
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: threadify-archiver
spec:
  replicas: 2  # Scale as needed
  template:
    spec:
      containers:
      - name: archiver
        image: threadify:latest
        command: ["/app/archiver"]
        volumeMounts:
        - name: config
          mountPath: /app/config
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 1000m
            memory: 1Gi
```

## Benefits

### Performance

✅ **API Speed**: API doesn't wait for Postgres writes
✅ **Batching**: Efficient bulk inserts (100x faster than individual)
✅ **Async**: Background processing doesn't block users

### Reliability

✅ **No Data Loss**: Events stay in Valkey until confirmed written
✅ **Retry Logic**: Automatic retry with backoff
✅ **Crash Recovery**: Resume from where it left off

### Scalability

✅ **Horizontal Scaling**: Add more archiver instances
✅ **Independent**: Scale API and archiver separately
✅ **Load Distribution**: Consumer groups distribute work

### Operational

✅ **Separation of Concerns**: API and persistence decoupled
✅ **Failure Isolation**: Postgres down? API still works
✅ **Observability**: Rich metrics and monitoring

## Trade-offs

### Eventual Consistency

**Implication**: Events in Valkey may not be in Postgres immediately

**Acceptable because:**
- Valkey is durable (AOF/RDB persistence)
- Typical delay: 5-30 seconds
- API reads from Valkey (fast, recent data)
- Postgres for long-term storage and analytics

### Memory Usage

**Implication**: Buffers hold events in memory

**Mitigation:**
- Configurable buffer sizes
- Time-based flush prevents unbounded growth
- Typical memory: 512MB-1GB per instance

### Complexity

**Implication**: Two services instead of one

**Justified by:**
- Better performance (API 10x faster)
- Better reliability (failure isolation)
- Better scalability (independent scaling)

## Future Enhancements

### Planned

- [ ] Dead Letter Queue (DLQ) for persistent failures
- [ ] Metrics endpoint (Prometheus)
- [ ] Health check endpoint
- [ ] Graceful shutdown (flush buffers first)
- [ ] Compression for large batches
- [ ] Priority queues (critical events first)

### Possible

- [ ] Multi-region replication
- [ ] Cold storage archival (S3)
- [ ] Real-time analytics stream
- [ ] Event replay capability

## Final Architecture Decisions

### **Data Flow:**

```
API Service writes (Pipeline - Atomic):
├─ List: thread:{threadID}:activity (for validation, handlers, queries)
│  └─ TTL: 7 days
└─ Stream: streams:step_events (for archival)
   └─ MAXLEN: ~100,000 entries (auto-trim)

Archiver Service:
└─ Reads from Stream → Writes to Postgres → ACKs
```

### **Key Design Choices:**

1. **Single Valkey Instance** (start simple, split later if needed)
2. **Pipeline for Atomic Writes** (both List + Stream succeed or fail together)
3. **MAXLEN for Auto-Trimming** (keeps last 100K entries, ~50MB)
4. **List for Operational Data** (validation, handlers, immediate access)
5. **Stream for Archival** (durable queue, consumer groups, at-least-once delivery)

### **Why This Works:**

- ✅ **Immediate validation**: List has data instantly
- ✅ **Reliable archival**: Stream guarantees delivery
- ✅ **Atomic writes**: Both succeed or both fail (no inconsistency)
- ✅ **Auto-cleanup**: MAXLEN prevents unbounded growth
- ✅ **Simple**: One Valkey instance, proven patterns

## Summary

The Archiver Service is a critical component that:

1. **Decouples** API from Postgres for performance
2. **Batches** events for efficient writes
3. **Retries** failed writes with backoff
4. **Scales** horizontally with consumer groups
5. **Recovers** from crashes without data loss

It maintains Threadify's core principle: **an immutable, append-only event stream that represents the universal truth of business service execution**, while ensuring that truth is durably persisted to Postgres.

---

## NATS Migration (2026-01-06)

### Migration Summary

Successfully migrated from Redis Streams to NATS JetStream for all archival operations.

### What Changed

**Before (Redis Streams):**
- Partitioned streams: `streams:activity_log:0-9`
- Manual partition logic using FNV hash
- Redis consumer groups with XREADGROUP
- Complex worker pool management

**After (NATS JetStream):**
- Unified streams: `activity.log`, `metadata.thread`, etc.
- NATS handles distribution automatically
- JetStream consumer with WorkQueue policy
- Simplified consumer code

### Benefits

✅ **Better Delivery Guarantees** - WorkQueue policy with explicit ACK/NACK  
✅ **Unified Message Bus** - Single system for all async messaging  
✅ **No Partitioning Complexity** - NATS handles distribution  
✅ **Built-in Monitoring** - JetStream provides metrics  
✅ **Simpler Code** - Removed 200+ lines of partition logic  

### Performance Metrics

Current measured throughput:

| Stream | Batch Size | Processing Time | Throughput |
|--------|-----------|----------------|------------|
| activity.log | 1-3 | 16-85 ms | 15-62 msg/s |
| metadata.thread | 1 | 100 ms | 10 msg/s |
| access.thread | 1 | 96 ms | 10 msg/s |
| validations.thread | 1 | 46 ms | 22 msg/s |

**End-to-end latency:** 50-110ms (event → database)  
**Bottleneck:** Postgres writes (~80% of time)  
**NATS overhead:** Negligible (~5ms)  

### Code Changes

**Files Modified:**
- `internal/archiver/nats_consumer.go` (NEW - 296 lines)
- `internal/repository/valkey/activity.go` (9 XAdd → NATS PublishAsync)
- `internal/repository/nats/archival_publisher.go` (Added success logging)
- `cmd/archiver/main.go` (Removed Redis stream consumers)
- `internal/repository/valkey/lua/validate_and_update_step_state.lua` (Removed XADD)

**Files Removed/Deprecated:**
- Redis stream consumer loops (archiver main.go)
- Partition logic (activity repository)
- Activity worker pool (no longer used)

### Monitoring

**Success Logs:**
```
✅ [NATS-ARCHIVAL] Published to activity.log (size: 296 bytes)
⏱️  [NATS-PERF] Processed 3 activity_log messages in 85ms (35.23 msg/s)
```

**Key Metrics:**
- NATS publish success rate (should be 100%)
- Archiver processing time (should be < 500ms)
- Postgres write success rate (should be 100%)
- NATS queue depth (should be < 100)

### Related Documentation

- [NATS Performance Metrics](./NATS_PERFORMANCE_METRICS.md)
- [Architecture Corrections](./ARCHITECTURE_CORRECTIONS.md)
