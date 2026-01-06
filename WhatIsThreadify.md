# Threadify - Product Summary (Iteration 2)
Core Concept
The thread IS the workflow.
Threadify is threading infrastructure for distributed systems. A thread represents a complete business process - every step, participant, error, and decision from start to finish.
Like:

GitHub for code → commit history tracks what changed
Threadify for workflows → thread history tracks what happened


The Problem We Solve
When distributed workflows fail, engineering teams spend 2-4 hours correlating logs across multiple services. Current solutions don't work:

Observability tools (DataDog, Honeycomb) focus on technical metrics, not business logic
Workflow engines (Temporal) require rebuilding workflows with their primitives
Logs/APM require manual correlation across services

The pain is acute: Happens during incidents, customer-facing outages, debugging sessions.

The Solution
Threading infrastructure where business processes exist as first-class objects (threads).
A Thread Contains:
Thread: checkout-flow-12345

[10:23:41] inventory-svc → "reserved_items" ✓
[10:23:43] payment-svc → "payment_failed" ✗
  └─ error: "Insufficient funds"
[10:25:12] payment-svc → "payment_retry_succeeded" ✓
[10:25:15] fulfillment-svc → "order_picked" ✓
[10:26:01] shipping-api → "label_created" ✓
[14:32:18] support-svc → "customer_contacted_support"
  └─ agent: "[email protected]"
[14:35:00] support-svc → "escalated_to_manager"

## New Features & API Changes (Iteration 2)

### Enhanced API Design
**Simplified Connection:**
```javascript
// Connect without service parameter - role defined per thread
const thread = await Threadify.connect('api-key');
```

**Role-Based Thread Creation:**
```javascript
// New parameter order: contract_name, role, { refs }
await thread.start('payment_flow', 'payment_processor', {
  refs: {
    customer_id: '12345',
    stripeId: 'ch_abc123',
    nhsId: 'NHS-789'
  }
});
```

**Step-Level External References:**
```javascript
// Method 1: Chain with addRefs()
await thread.step("payment")
  .context({ amount: "$49.99" })
  .addRefs({ stripe_payment_id: paymentIntent.id });

// Method 2: In step creation
await thread.step("loan_application", {
  external_refs: { "application_id": "app_12345" }
});
```

**Flexible Join Methods:**
```javascript
// Token-based join (external parties)
await thread.join(jwt_token);

// Direct join (internal services)  
await thread.join(threadId, 'payment_gateway');
```

### Security & Permission System

**Three-Tier Permission Caching:**
- In-memory cache (mutex-protected) → ~0.1ms lookup
- Valkey cache → ~2-5ms lookup
- Write-through caching for durability

**Permission-Based Access Control:**
- Thread owners have implicit full access
- Invited users get explicit permissions: `read`, `write`
- Permission checks on every `recordThreadEvent` call
- Cross-company collaboration enabled (no company restrictions)

**Role-Based Step Validation (Contract Workflows):**
- Contract steps can require specific roles (e.g., `buyer`, `seller`)
- User role validated before executing contract steps
- Only applies to threads with contracts
- Example: Only `buyer` role can execute `create_order` step

**Server-Level Validation:**
- All permissions validated server-side (no client trust)
- Role-based access control with contract validation
- Invitation permissions enforced (invite permission required)

### Non-Blocking Contract Validation

**Two-Phase Validation System:**
Threadify uses a two-phase validation approach to ensure data integrity without blocking workflow execution:

**Phase 1: Blocking Validations** (Executed before step is recorded)
- Authentication & authorization checks
- Required field validation
- Thread existence & access control
- Idempotency checks (duplicate prevention)
- Step exists in contract
- Entry point validation (first step must be an entry point)
- Role validation (user has correct role for step)
- Required business context fields present
- Thread completion status (cannot add steps to completed threads)

**Phase 2: Non-Blocking Validations** (Executed asynchronously after step is recorded)
- **Invalid Transition Detection** (Critical) - Validates step follows contract's defined transitions
- **Step Timeout Exceeded** (Critical) - Checks if step duration exceeded timeout
- **Max Duration Exceeded** (Critical) - Validates thread hasn't exceeded max_duration
- **Multiple Terminal States** (Critical/Configurable) - Detects if thread reached multiple terminal states
- **Retry Limit Exceeded** (Critical) - Checks if step retry count exceeded max_retries
- **Missing Optional Fields** (Info) - Tracks missing optional business context fields
- **Extra Undocumented Fields** (Info) - Identifies fields not defined in contract

**Validation Flow:**
```
Step Submission → Blocking Validations → Record Step → Return Success
                                              ↓
                                    Async Goroutine (Non-Blocking)
                                              ↓
                         Check Transitions, Timeouts, Limits
                                              ↓
                         Store Notifications in Stream
                                              ↓
                    Update Thread.Violated (if critical failure)
```

**Thread Violation Tracking:**
```go
type Thread struct {
    CurrentSteps []string          // Array of current steps (supports parallel execution)
    Violated     *ThreadViolation  // Tracks failed steps and violations
    // ... other fields
}

type ThreadViolation struct {
    FailedSteps map[string]StepViolation // key: stepID
    ViolatedAt  time.Time
}

type StepViolation struct {
    StepID        string
    StepName      string
    OwnerID       string                 // Who published the failed step
    ViolationType ViolationType          // e.g., "invalid_transition"
    Severity      ViolationSeverity      // "critical", "major", "minor", "warning", "info"
    Message       string
    Details       map[string]interface{}
    ViolatedAt    time.Time
}
```

## New Features & API Changes (Iteration 3)

### Contract Preview Endpoint
**Instant Workflow Visualization:**
```bash
# Preview contract without persisting
curl -X POST http://localhost:8080/v1/contracts/preview \
  -H "Content-Type: application/x-yaml" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d 'contract.yaml'
```

**Response:**
```json
{
  "valid": true,
  "mermaid": "```mermaid\nflowchart TD\n    %% Contract: payment_flow\n    %% Node styles\n    classDef entry fill:#e1f5fe,stroke:#01579b,stroke-width:2px\n    classDef terminal fill:#f3e5f5,stroke:#4a148c,stroke-width:2px\n    classDef step fill:#e8f5e8,stroke:#2e7d32,stroke-width:2px\n    classDef group fill:#fff3e0,stroke:#e65100,stroke-width:2px\n\n    %% Swim lanes by party\n    subgraph merchant\n        node_order_placed[\"order_placed\\n⏱️ 5m\\n(merchant)\"]\n    end\n\n    subgraph payment_processor\n        node_payment_validation[\"payment_validation\\n⏱️ 10m\\n(payment_processor)\"]\n    end\n\n    %% Transitions\n    node_order_placed --> node_payment_validation | retry: 3 |\n\n    %% Node classifications\n    class node_order_placed entry\n    class node_payment_validation step\n```",
  "errors": []
}
```

**Key Features:**
- **Live Validation** - Validates YAML against parser and supported properties
- **Visual Debugging** - Generates Mermaid flowchart with swim lanes by party
- **Timeout Visualization** - Shows ⏱️ timeout indicators on each step
- **Retry Configuration** - Displays retry limits on transition arrows
- **No Persistence** - Pure in-memory validation and conversion
- **Authentication Required** - Protected endpoint with JWT auth

**Enhanced Mermaid Visualization:**
- **Party Swim Lanes** - Steps grouped by owner (merchant, payment_processor, etc.)
- **Parallel Group Subgraphs** - Visual hierarchy for parallel execution
- **Timeout Annotations** - Prominent timeout display for bottleneck identification
- **Entry/Terminal Styling** - Color-coded nodes (blue entry, purple terminal, green steps, orange groups)

**Notification Stream:**
All validation results (success and failures) are stored in a Valkey stream for audit and monitoring:
```
streams:validation_notifications = [
  {
    notificationId: "notif-123",
    threadId: "thread-456",
    stepId: "step-789",
    stepName: "payment_validated",
    ownerId: "user-123",
    status: "failed",
    violationType: "invalid_transition",
    severity: "critical",
    message: "Invalid transition from 'order_placed' to 'package_shipped'",
    fromStep: "order_placed",
    toStep: "package_shipped",
    timestamp: "2025-12-30T02:00:00Z"
  }
]
```

**Atomic Updates with Lua Scripts:**
To prevent race conditions when multiple steps validate simultaneously, thread status updates use atomic Lua scripts:

```lua
-- Executed atomically in Valkey
local currentStatus = redis.call('HGET', threadKey, 'status')
-- Status priority: failed (3) > completed (2) > active (1)
if newPriority >= currentPriority then
    redis.call('HSET', threadKey, 'status', newStatus)
end
```

**Benefits:**
- **Fast Response Times:** Steps are recorded immediately without waiting for complex validations
- **Complete Audit Trail:** All violations tracked with full context
- **Flexible Severity:** Different violation types have configurable severity levels
- **Parallel Execution Support:** CurrentSteps array enables multiple concurrent steps
- **No Blocking:** Critical validations run asynchronously, allowing workflow to continue
- **Race-Free Updates:** Lua scripts ensure atomic status transitions (failed > completed > active)

### Step Deduplication with Idempotency Keys

**Automatic Idempotency:**
- SDK automatically generates idempotency keys from `stepName + context` (FNV-1a hash)
- Prevents accidental duplicate submissions (network retries, double-clicks)
- Duplicate successful steps are rejected with clear error message

**Manual Override:**
```javascript
// Use external transaction ID as idempotency key
await thread.step('payment')
  .addContext({ orderId: 'PO-123', amount: '100' })
  .idempotencyKey('stripe-txn-xyz789')
  .stop('success');
```

**Retry Handling:**
- Failed steps can be retried (same idempotency key → updates existing)
- Successful steps are immutable (duplicate rejected)
- Each attempt creates new event in activity queue (complete audit trail)
- Deduplication index tracks latest state per idempotency key

**Storage:**
- All steps stored with key format: `stepName:idempotencyKey`
- Event stream remains immutable (append-only)
- Index map for O(1) duplicate detection

### Archiver Service - Durable Persistence

**Architecture:**
```
API Service → Valkey Streams → Archiver Service → Postgres
  (fast)      (in-memory)       (batching)      (durable)
```

**Purpose:**
- Decouples API from Postgres for performance
- Batches events for efficient writes (100x faster)
- Ensures durable persistence without blocking users

**How It Works:**
1. API writes events to Valkey streams (non-blocking, <1ms)
2. Archiver reads from streams in batches
3. Accumulates events in local buffers
4. Flushes to Postgres on size OR time trigger
5. ACKs stream entries only after successful write

**Flush Triggers:**
- **Size**: 100 events (step events), 50 (threads), 200 (audit logs)
- **Time**: 5-30 seconds depending on queue type
- Whichever comes first

**Reliability:**
- Exponential backoff retry (6 attempts: 1s, 2s, 4s, 8s, 16s)
- Events stay in Valkey until confirmed written
- Crash recovery: Resume from pending entries
- No data loss (at-least-once delivery)

**Scaling:**
- Consumer groups enable horizontal scaling
- Multiple archiver instances share load
- Independent scaling from API service

**Benefits:**
- ✅ API 10x faster (no Postgres wait)
- ✅ Efficient batching (bulk inserts)
- ✅ Failure isolation (Postgres down? API still works)
- ✅ Complete audit trail preserved

**Multi-Service Architecture:**
- Single API key supports multiple services
- Service-specific session isolation
- Cross-service permission boundaries

### Storage & Performance
**Redis Storage Pattern:**
```go
// Enhanced Thread metadata with refs
thread:thread123 = {
  threadId: "thread123",
  contractId: "payment_flow",
  contractName: "payment_flow",
  refs: {
    stripeId: "ch_123",
    customerId: "12345"
  },
  createdAt: "2025-01-01T12:00:00Z",
  updatedAt: "2025-01-01T12:05:00Z"
}

// Role mapping hash (fast lookups)
thread:thread123:roles = {
  "payment_processor": "api-key-123",
  "fraud_reviewer": "api-key-456",
  "auditor": ""
}

// User permissions hash
thread:thread123:permissions = {
  "api-key-123": ["read", "write", "approve"],
  "api-key-456": ["read", "review"]
}

// Thread event queue for activity tracking
thread:thread123:queue = [
  {
    stepName: "payment",
    userId: "api-key-123",
    role: "payment_processor",
    refs: {stripe_payment_id: "pi_456"},
    context: {amount: "$49.99"},
    timestamp: "2025-01-01T12:01:00Z"
  }
]

// Session connection state
session:session789 = {
  api_key: "api-key-123",
  service: "payment-service",
  created_at: "2025-01-01T12:00:00Z"
}
```

**Performance Optimizations:**
- **Redis Pipelines:** All multi-hash operations use pipelines for efficiency
- **Direct Hash Lookups:** Role and permission queries without parsing full thread objects
- **TTL-Based Cleanup:** Automatic cleanup with configurable retention periods
- **Immediate Cleanup Option:** Clean up completed threads immediately to free memory

**Configuration (config.yaml):**
```yaml
redis:
  ttl:
    thread: 24h        # Thread metadata
    roles: 24h         # Role hash
    permissions: 24h   # Permission hash
    events: 72h        # Event queue (longer for audit)
    session: 1h        # Session timeout
```

### Server Implementation (Go)
**WebSocket Message Handlers:**
```go
// Enhanced message structures (already implemented)
type StartThreadRequest struct {
    Action       string            `json:"action"`
    ContractName string            `json:"contractName"`
    Role         string            `json:"role"`
    Refs         map[string]string `json:"refs"`
}

type JoinThreadRequest struct {
    Action      string `json:"action"`
    ThreadToken string `json:"threadToken,omitempty"` // Token-based
    ThreadID    string `json:"threadId,omitempty"`    // Direct join
    Role        string `json:"role,omitempty"`        // Direct join
}
```

**Enhanced ThreadRepository Implementation:**
```go
// Role management with hash operations (fast lookups)
func (r *ThreadRepository) AssignRole(ctx context.Context, threadID, role, userID string) error
func (r *ThreadRepository) GetUserRole(ctx context.Context, threadID, userID string) (string, error)
func (r *ThreadRepository) GetAllRoles(ctx context.Context, threadID string) (map[string]string, error)

// Permission management with hash operations
func (r *ThreadRepository) SetUserPermissions(ctx context.Context, threadID, userID string, permissions []string) error
func (r *ThreadRepository) GetUserPermissions(ctx context.Context, threadID, userID string) ([]string, error)

// Event queue operations for activity tracking
func (r *ThreadRepository) AddThreadEvent(ctx context.Context, threadID string, event models.ThreadEvent) error
func (r *ThreadRepository) GetThreadEvents(ctx context.Context, threadID string, start, stop int64) ([]models.ThreadEvent, error)

// Pipeline operations for atomic multi-hash operations
func (r *ThreadRepository) CreateThreadWithSetup(ctx context.Context, thread *models.Thread, creatorID string, creatorRole string, creatorPerms []string) error
func (r *ThreadRepository) CompleteThread(ctx context.Context, threadID string, immediateCleanup bool) error
```

**Enhanced Thread Model:**
```go
type Thread struct {
    ID              string                 `json:"id"`
    ContractID      *string                `json:"contractId,omitempty"`
    ContractVersion *int                   `json:"contractVersion,omitempty"`
    ContractName    string                 `json:"contractName,omitempty"`  // NEW
    Refs            map[string]string      `json:"refs,omitempty"`         // NEW
    OwnerID         string                 `json:"ownerId"`
    Status          ThreadStatus           `json:"status"`
    // ... rest of existing fields
}

type ThreadEvent struct {
    ThreadID  string            `json:"threadId"`
    StepName  string            `json:"stepName"`
    Action    string            `json:"action"`
    UserID    string            `json:"userId"`
    Role      string            `json:"role"`
    Refs      map[string]string `json:"refs,omitempty"`
    Context   map[string]string `json:"context,omitempty"`
    Status    string            `json:"status"`
    Timestamp time.Time         `json:"timestamp"`
}
```

**Key Architecture Improvements:**
- **No RefsStorage:** Enhanced existing ThreadRepository instead of creating duplicate storage
- **Hash-Based Roles:** Fast role lookups with `thread:thread123:roles` hash
- **Hash-Based Permissions:** User permissions in `thread:thread123:permissions` hash  
- **Pipeline Operations:** Atomic multi-hash operations for thread setup/teardown
- **TTL Configuration:** Configurable cleanup periods for different data types
- **Event Queues:** Per-thread activity tracking with `thread:thread123:queue`

### Contract Integration
**Contract-Free Threading:**
- Threads can be created without contracts
- Optional contract validation when specified
- Backward compatibility maintained

**Role Validation:**
- Roles validated against contract parties
- Only contract-defined roles allowed
- Server validates role exists in contract YAML

Key Properties:

One source of truth - Complete workflow history in one thread
Cross-boundary - Internal services + external APIs + human actions
Cryptographic audit trail - Compliance-grade proof of what happened
Operational lookup - "Show me order #12345" not "analyze trends"
Role-based security - Server-enforced permissions with contract validation
Multi-service support - Single API key, isolated service sessions


What Makes It Different
Not Observability (DataDog/Honeycomb):

They instrument code, you correlate traces
Threadify: Process exists as thread, observability is inherent

Not Workflow Orchestration (Temporal):

They execute workflows with durable primitives
Threadify: Participants advance threads, coordination is collaborative

Not Integration Platform (Paragon/Merge):

They connect external APIs
Threadify: Thread internal processes (with or without external systems)

The Positioning:
"Threading infrastructure for business processes" - not orchestration, not pure observability, but workflow coordination + observability unified.

Technical Architecture (What We Know)
Core Components:
Backend: Go with Redis + PostgreSQL

Redis streams for event sourcing
PostgreSQL for persistent storage
Hybrid approach: Redis for real-time, Postgres for durability

SDK Communication: WebSocket-based

Real-time updates
Bidirectional communication

Thread Management:
python# Create thread
thread = threadify.create("checkout", order_id="123")

# Participants post steps
thread.step("inventory_checked", service="inventory-svc", data={...})
thread.step("payment_processed", service="payment-svc", data={...})
thread.step("fraud_review_approved", user="[email protected]", data={...})
State Machines: YAML contracts for validation

Define expected workflow states
Validate step transitions
Ensure data contracts

Cryptographic Audit Trails: Zero-knowledge architecture

Threadify sees encrypted data
Compliance-grade proofs


Use Cases (Developer-First)
Primary:

Incident debugging - "What happened to this failed checkout?"
Compliance/audit - "Prove who approved this transaction"
Cross-team coordination - "What's the status of this onboarding?"

Example Workflows:

E-commerce checkout (inventory → payment → fulfillment → shipping)
SaaS user onboarding (signup → verification → provisioning → activation)
Loan origination (application → credit check → underwriting → approval)
Support escalations (ticket → L1 → L2 → manager → resolution)

Long-running workflows:
Threadify handles workflows spanning minutes, hours, days, or weeks:

Manual approval steps (waits for human action)
Multi-day processes (loan underwriting, employee onboarding)
Any workflow where state persists and steps happen asynchronously


Target Customer (Iteration 1)
ICP: Engineering/Platform teams at companies with:

3-10+ microservices
Multi-step business processes that span services
Compliance requirements (fintech, healthcare, regulated industries)
High-value transactions where debugging costs exceed subscription

Verticals:

E-commerce
Fintech
SaaS companies
Logistics/supply chain
Healthcare

NOT for:

Startups with 2-3 services (premature)
Companies without distributed systems


Differentiation & Moats
Technical:

Cryptographic audit trails - compliance moat
Thread as first-class primitive - category creation
Cross-boundary threading - internal + external + human

Product:

Doesn't replace existing systems - connective layer, not replacement
Progressive adoption - instrument incrementally, don't rebuild
Operational focus - instance-level ("show order #12345") not aggregate analytics

Strategic:

Developer-first PLG - fast adoption, viral growth
SaaS delivery - zero ops overhead vs self-hosted Temporal
Platform potential - others can integrate for context (future)


Competitors & Positioning
No Direct Competitors:

Inngest - durable functions, not workflow threading
Temporal - workflow orchestration, requires rebuild
WorkOS Audit Logs - just logging, no coordination
Process Mining (Celonis) - analytics, not operational lookup

We Compete With:
Temporal + DataDog - companies using both for orchestration + observability
Our advantage: Unified primitive (thread) vs two separate tools

Go-to-Market Strategy
Phase 1: Developer PLG

Build SDK (Python, Go, JavaScript)
Developer-friendly docs
Self-serve signup
Instant value: thread 1 workflow, see results
Community/content: "How we reduced MTTR by 60%"

Growth Levers:

Technical blog content
GitHub presence
Developer community
Word of mouth in engineering teams

Success Metrics:

Time to first thread
Developer activation rate
Threads created per customer
NPS among engineering teams


What We're NOT Building (Iteration 1)
❌ Analytics dashboards (users can build via GraphQL later)
❌ Natural language conditions (add if customers ask)
❌ Non-technical user UI (future: Zapier plugin + business UI)
❌ 100+ integrations (start with SDK, Zapier/webhooks)
❌ Marketplace/partner program (platform features for later)

The MVP Scope
Core:

Thread creation/management API
SDK (Python first)
Basic web UI (search threads, view timeline)
State machine validation (YAML contracts)
Cryptographic audit logging

Nice-to-have:

Event handlers (on_step, basic webhooks)
Zapier integration
Simple search/filtering

Validate:

Can developers thread a workflow in < 1 hour?
Does viewing thread save debugging time?
Would they pay $200-500/month for this?


Key Strategic Decisions Made
✅ Developer-first, not business-user-first
✅ Threading infrastructure, not orchestration or observability
✅ Thread as primitive - workflow exists as thread
✅ SaaS delivery - not self-hosted
✅ API-first architecture - platform-ready from day 1
✅ Start as product, architect for platform
✅ Progressive adoption - instrument existing code, don't rebuild

Brand & Messaging
Tagline:
"Threading infrastructure for distributed systems"
Value Props:

"See everything that happened in any workflow - across your services, external APIs, and teams - in one place"
"Stop losing hours correlating logs across services"
"One source of truth for every business transaction"

Brand Verb: "Threadify your workflows"
Domain: threadify.dev

---

## Architecture Updates (January 2026)

### Partitioned Stream Architecture & Unified Activity Log

**Problem Solved:**
- Eliminated per-thread stream discovery overhead
- Implemented partitioned global streams for scalable archival
- Dual-write pattern for both queries and archival
- Consolidated all event types into unified activity log

**Current Data Architecture:**

**1. Dual-Stream Pattern (Per-Thread + Partitioned)**

**Per-Thread Streams** (for queries & real-time):
- `thread:{id}:activity` - Valkey Stream (XADD)
- Used for: Thread-specific queries, real-time UI updates
- Event types: `step_recorded`, `thread_created`, `invitation_created`, `invitation_used`
- Access pattern: XRANGE for queries, XREADGROUP for real-time consumers

**Partitioned Global Streams** (for archival):
- `streams:activity_log:{0-9}` - 10 partitioned streams
- Used for: Reliable archival to Postgres
- Partition selection: FNV hash of thread_id % 10
- Access pattern: XREADGROUP with consumer groups

**Dual-Write Implementation:**
- All events written to BOTH streams atomically using pipelines
- Per-thread stream: Fast queries without partition lookup
- Partitioned stream: Scalable archival with worker pool

**2. Step State Cache**
- `thread:{id}:steps:{name}:{idempotency_key}` - Valkey Hash
- O(1) lookups for idempotency checks, retry validation
- Fields: status, retryCount, id (step UUID), firstSeenAt, lastUpdatedAt, previousStep
- Does NOT store context (lean approach)

**3. Postgres Archive Tables**
- `activity_log` - Consolidated audit trail for ALL events
  - Columns: id, thread_id, step_id, type, payload (JSONB), hash, created_at
  - Indexed: thread_id, step_id, type, created_at
- `thread_step_state` - Normalized step state snapshots
- `thread_access` - Access grant/revoke audit trail
- `threads` - Thread metadata

**Key Features:**

**Partitioned Worker Pool:**
- 10 partitions, 3 workers per archiver instance (configurable)
- Each worker iterates through all partitions with 100ms block timeout
- Consumer group: "archivers" (supports multiple archiver instances)
- Batch writes: 200 events or 5s flush interval
- XACK after successful Postgres write

**Idempotency Keys:**
- User-provided OR auto-generated from context hash (FNV-1a)
- Composite step_id format: `"{step_name}:{idempotency_key}"`
- Enables automatic retry tracking without explicit keys
- Duplicate successful steps rejected, failed steps can retry

**Event Types Archived:**
- ✅ `step_recorded` - Step execution events
- ✅ `thread_created` - Thread initialization
- ✅ `invitation_created` - Invitation token generation
- ✅ `invitation_used` - Thread join via invitation
- ⏳ `thread_completed` - Thread completion (planned)
- ⏳ `thread_failed` - Thread failure (planned)
- ⏳ `validation_warning` - Non-blocking validation warnings (planned)
- ⏳ `access_revoked` - Permission revocation (planned)

**Reliable Archival:**
- Consumer groups with ACK mechanism
- Exponential backoff retry logic (6 attempts)
- Guaranteed at-least-once delivery to Postgres
- Stream trimming after archival (max 10,000 entries per partition)

**Hot/Cold Data Pattern:**
- Valkey streams for active threads (fast queries, <5ms)
- Postgres for historical data (permanent storage, compliance)
- Automatic TTL management per thread (configurable)

**Benefits:**
- ✅ Scalable archival - no per-thread stream discovery needed
- ✅ Fast queries - direct per-thread stream access
- ✅ Horizontal scaling - multiple archiver instances via consumer groups
- ✅ Atomic dual-write - pipeline ensures consistency
- ✅ Partition-based load distribution - FNV hash for even distribution
- ✅ Immutable audit trail with cryptographic hashing
- ✅ Real-time capable - multiple consumers via consumer groups
- ✅ Performance - O(1) operational queries, efficient batch writes

### Recent Bug Fixes & Improvements (January 2026)

**1. Stream ID Conflict Resolution**
- **Problem**: Archiver's `thread_step_state` stream was failing with "ERR Invalid stream ID specified as stream command argument"
- **Root Cause**: The stream data contained a field named `"id"` (step UUID), which conflicted with Redis stream entry IDs during XACK operations
- **Solution**: Renamed the field from `"id"` to `"step_id"` in both:
  - Stream writer: `/internal/repository/valkey/activity.go` (line 378)
  - Archiver reader: `/internal/archiver/postgres_writer.go` (lines 337, 381)
- **Impact**: Archiver now successfully acknowledges processed events without ID conflicts

**2. JoinThread Response Enhancement**
- **Problem**: `joinThread` WebSocket response was missing the `permissions` field, forcing clients to make additional API calls
- **Solution**: Added `Permissions: strings.Join(permissions, ",")` to `JoinThreadResponse` in `/internal/service/thread.go` (line 742)
- **Benefits**:
  - Clients immediately know their access level after joining
  - Enables client-side authorization decisions (show/hide UI elements)
  - Consistent with `inviteParty` response format
  - Better UX - no round-trip needed for permission checks

**3. Thread Access JSONB Migration**
- **Problem**: Archiver was failing with "ERROR: operator does not exist: character varying = jsonb" when writing to `thread_access` table
- **Root Cause**: The `roles` column was defined as `VARCHAR(100)` but archiver was casting to `::jsonb`
- **Solution**: 
  - Created migration `002_create_and_alter_thread_access.sql` to convert `roles` column to JSONB
  - Updated `InitSchema` in `/internal/database/postgres.go` to create table with JSONB from start
  - Added GIN index on `roles` column for efficient JSONB queries
- **Benefits**:
  - Proper data type for JSON arrays
  - Enables JSONB operators: `WHERE roles @> '["merchant"]'::jsonb`
  - Better performance with GIN indexing
  - Type safety and validation at database level

**4. Empty Timestamp Handling**
- **Problem**: Archiver failing with "ERROR: invalid input syntax for type timestamp" when `timestamp` field was empty
- **Solution**: Added fallback in `/internal/archiver/postgres_writer.go` (lines 289-292):
  ```go
  if timestamp == "" {
      timestamp = time.Now().Format(time.RFC3339)
  }
  ```
- **Impact**: Graceful handling of missing timestamps with current time fallback

**Technical Debt Addressed:**
- ✅ Stream field naming conflicts resolved
- ✅ Database schema aligned with application code
- ✅ API responses now complete and consistent
- ✅ Archiver reliability improved with proper error handling