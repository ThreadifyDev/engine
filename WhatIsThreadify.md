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