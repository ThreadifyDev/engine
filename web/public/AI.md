# Threadify SDK - AI Assistant Guide

## Language-Specific Syntax Guides

For implementation syntax in your language, see:
- **JavaScript/TypeScript**: [AI-javascript.md](https://threadify.dev/AI-javascript.md)
- **Python**: [AI-python.md](https://threadify.dev/AI-python.md) (Coming Soon)
- **Go**: [AI-go.md](https://threadify.dev/AI-go.md)

---

## What is Threadify?

**Every customer request tells a story. Turn it into intelligence.**

Threadify turns customer requests into live execution graphs. Support answers "what happened?" in seconds. Operations validates business logic in real-time. AI agents act with complete context.

**Core Components:**
- **Thread** - One customer request flowing through your system
- **Step** - One action in the workflow
- **Contract** - YAML validation rules enforced at runtime

---

## Core Concepts

### 1. Connecting to Threadify

**What it does:** Establishes WebSocket connection to Threadify Engine

**Parameters:**
- `apiKey` (required) - Your API key
- `serviceName` (optional) - Identifier for your service
- `options` (optional) - Configuration object
  - `wsUrl` - WebSocket URL (default: wss://eng.threadify.dev/threads)
  - `graphqlUrl` - GraphQL endpoint URL
  - `debug` - Enable debug logging (boolean)

**Returns:** Connection instance

---

### 2. Starting a Thread

**What it does:** Creates a new execution thread

**Variants:**
1. **No contract** - Simple workflow tracking
2. **With service name** - Specify which service is starting
3. **With contract** - Contract-based workflow with validation

**Parameters:**
- `contractName` (optional) - Name of contract to use
- `serviceName` (optional) - Service identifier

**Returns:** Thread instance

---

### 3. Recording a Step

**What it does:** Records one action in the workflow

**Required:**
- `stepName` - Descriptive name (use snake_case)

**Step Methods (fluent API):**
- `addContext(data)` - Add business context (key-value pairs)
- `idempotencyKey(key)` - Set custom idempotency key
- `subStep(name, data, status)` - Add sub-step
- `success(messageOrData)` - Mark step as successful
- `failed(messageOrData)` - Mark step as failed (business logic)
- `error(messageOrData)` - Mark step as errored (system error)

**Thread Methods (called on thread object):**
- `thread.addRefs(refs)` - Add external system references
- `thread.linkThread(threadId, relationship)` - Link to another thread

**Context Rules:**
- All values converted to strings automatically
- Keep context flat (no nested objects)
- Use descriptive keys (e.g., `customer_id` not `cid`)

---

### 4. Step Status

Every step must have one of three statuses:

| Status | When to Use | Example |
|--------|-------------|---------|
| `success` | Step completed as expected | Payment processed |
| `failed` | Business logic failure | Payment declined |
| `error` | System/technical error | Payment gateway timeout |

---

### 5. Idempotency

**What it does:** Prevents duplicate step execution

**How it works:**
- **Auto-generated (default)**: SDK creates hash from `stepName + context`
- **Manual**: Call `.idempotencyKey(key)` to set custom key
- Server rejects duplicates with same key

**When to use manual keys:**
- External system IDs (e.g., payment transaction ID)
- User-initiated retries (e.g., "Retry Payment" button)
- Cross-service deduplication

**When auto-generated works:**
- Context uniquely identifies the operation
- No external identifier available
- Simple retry scenarios

---

### 6. External References

**What it does:** Links thread to external systems

**Usage:** Call `thread.addRefs(refs)` on the thread object

**Parameters:**
- `refs` - Key-value pairs (e.g., `{stripe_payment_id: 'pi_123'}`)

**Use cases:**
- Link to payment provider IDs
- Link to order management system
- Link to CRM records

---

### 7. Thread Linking

**What it does:** Creates parent-child relationships between threads

**Parameters:**
- `threadId` - Thread ID to link to
- `relationship` - Type of relationship (default: 'parent')

**Use cases:**
- Multi-stage workflows
- Saga patterns
- Distributed transactions

---

### 8. Notifications (Event Subscriptions)

**What it does:** Subscribe to real-time events

**Event Types:**
- `step.success` - Step completed successfully
- `step.failed` - Step failed
- `step.error` - Step errored
- `rule.violated` - Contract validation failed
- `rule.passed` - Contract validation passed

**Parameters:**
- `event` - Event pattern (e.g., 'step.success')
- `stepName` - Step to monitor
- `handler` - Callback function

**Handler receives:**
- `notification` object with context, severity, etc.
- Must call `notification.ack()` to acknowledge

---

### 10. Joining Threads

**What it does:** Join an existing thread

**Variants:**
1. **With invitation token** - External parties
2. **Direct join** - Internal services with threadId + role

**Parameters:**
- `token` - JWT invitation token
- OR
- `threadId` - Thread ID to join
- `role` - Role for access control

---

### 11. Contracts (YAML)

**What it does:** Defines business rules enforced at runtime

**Structure:**
```yaml
name: order_flow
entry_points: [validate_cart]
terminal_steps: [order_complete, order_failed]
transitions:
  validate_cart: [check_inventory]
  check_inventory: [charge_payment]
  charge_payment: [send_confirmation]
```

**Prevents:**
- Race conditions
- Invalid state transitions
- Missing required steps

---

## Common Patterns

### Error Handling Pattern
1. Wrap step in try/catch
2. On success: call `.success()`
3. On business failure: call `.failed()`
4. On system error: call `.error()`

### Integration Pattern
1. Connect once, reuse connection
2. Start thread per request
3. Record steps as actions execute
4. Add context for queryability

### Microservices Pattern
1. Service A starts thread
2. Service A records its steps
3. Service A passes `thread.id` to Service B
4. Service B joins thread
5. Service B records its steps

---

---

## MCP Integration (Model Context Protocol)

**What it does:** Enables AI assistants and automation tools to query Threadify execution graphs

**Available via:** Native MCP server at `/threadify-go/.mcp/server.json`

### MCP Tools

| Tool | Purpose | Example Query |
|------|---------|---------------|
| `get_thread` | Retrieve complete thread execution | "Show me thread abc-123 with all steps" |
| `search_threads` | Find threads with filters | "Find all failed checkout threads from last hour" |
| `verify_thread_integrity` | Validate cryptographic hash chain | "Verify integrity of thread abc-123" |
| `contract_graph` | Get workflow structure | "Show me the checkout contract graph" |
| `resolve_actors` | Convert UUIDs to names | "Who executed payment_authorized in thread abc-123?" |
| `graphql_query` | Custom precise queries | Execute any GraphQL query |

### MCP Use Cases

✅ **Debugging Silent Failures**
- "Why did payment succeed but order never shipped?"
- MCP reveals which step failed silently after payment

✅ **Compliance Verification**
- "Prove credit checks always run before loan approval"
- MCP validates hash chains and execution order

✅ **Performance Analysis**
- "What's the bottleneck in our checkout flow?"
- MCP shows average step durations

### MCP Configuration

**Claude Desktop:**
```json
{
  "mcpServers": {
    "threadify": {
      "command": "npx",
      "args": ["-y", "@threadify/mcp-server"],
      "env": {
        "THREADIFY_API_KEY": "your-api-key",
        "THREADIFY_URL": "https://mcp.threadify.dev"
      }
    }
  }
}
```

**Cline (VS Code):**
Add to MCP settings with same configuration structure.

**Custom Integration:**
Use any MCP SDK to connect and call tools programmatically.

### MCP Resources

- `graphql://schema` - Complete GraphQL schema for advanced queries

**Learn more:** https://docs.threadify.dev/core-concepts/mcp-integration

---

## When to Use Threadify

✅ **Good fit:**
- Multi-step business processes
- Distributed workflows across services
- Need audit trail for compliance
- Complex error handling/retries
- Support teams need visibility

❌ **Not ideal:**
- Simple CRUD operations
- Single-step actions
- High-frequency events (>10k/sec per thread)

---

## Documentation Links
- Full docs: https://docs.threadify.dev
- Quickstart: https://docs.threadify.dev/quickstart
- Core Concepts: https://docs.threadify.dev/core-concepts/overview
- API Reference: https://docs.threadify.dev/api-reference/overview
- MCP Integration: https://docs.threadify.dev/core-concepts/mcp-integration
