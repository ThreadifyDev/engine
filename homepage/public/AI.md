# Threadify SDK - AI Assistant Guide

## Language-Specific Syntax Guides

For implementation syntax in your language, see:
- **JavaScript/TypeScript**: [AI-javascript.md](https://threadify.dev/AI-javascript.md)
- **Python**: [AI-python.md](https://threadify.dev/AI-python.md)
- **Go**: [AI-go.md](https://threadify.dev/AI-go.md)

---

## What is Threadify?

**A shared referee for work across services and AI agents.**

Threadify records the steps of existing workflows, checks them against explicit rules, and keeps the evidence behind each result. Teams can observe violations or have their application await permission before an action. The services and agents still execute the work.

**Core Components:**
- **Thread** - One workflow execution, with its recorded steps and optional contract
- **Step** - One action in the workflow

**Optional (Advanced):**
- **Contract** - Readable Gherkin-style rules for step order, approvals, and content, including regex patterns and comparisons with earlier steps. Contracts are optional; start with basic instrumentation when rules are not requested.

---

## Core Concepts

### 1. Connecting to Threadify

**What it does:** Establishes WebSocket connection to Threadify Engine

**Parameters:**
- `apiKey` (required) - Your API key
- `serviceName` (optional) - Identifier for your service
- `options` (optional) - Configuration object
  - `wsUrl` - WebSocket URL (default: wss://your-threadify-engine.example/threads)
  - `graphqlUrl` - GraphQL endpoint URL
  - `debug` - Enable debug logging (boolean)

**Returns:** Connection instance

---

### 2. Starting a Thread

**What it does:** Creates a new execution thread

**Variants:**
1. **With label** - (Recommended) Give the thread a human-readable name (e.g., "Checkout-cust-123")
2. **With a contract** - Bind the workflow rules for this execution
3. **With tags** - Immutable labels for filtering (e.g., `["production", "v2.1"]`)

**Parameters:**
- `label` (optional, recommended) - A descriptive name for the thread
- `contractName` (optional) - Contract name and version when rules are needed
- `serviceName` (optional) - Service identifier, supplied through the SDK options
- `tags` (optional) - Immutable labels for filtering (e.g., `["production", "v2.1"]`)

> Use basic tracking for observation. Bind a contract when the goal includes checking workflow rules or requesting permission before an action.

**Returns:** Thread instance

**Tags:**
- Immutable string labels attached at thread creation
- Used for categorization, filtering, and organizing threads
- Set via SDK `start()` or OTel span attributes (if the codebase already uses OpenTelemetry)
- Queryable via GraphQL `threads(tags: ["production"])`

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

**What it does:** Deduplicates reports submitted with the same idempotency key. It does not prevent your application from performing an external action twice.

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
- `refs` - Key-value pairs (e.g., `{payment_id: 'pi_123'}`)

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

**Event Types (Basic):**
- `step.success` - Step completed successfully
- `step.failed` - Step failed
- `step.error` - Step errored

**Event Types (Contract-only — Advanced):**
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

### 9. Retrieving Thread Data

**What it does:** Query thread execution data for analysis

**Important:** `getThread()` returns a **read-only** thread object. To add steps or modify a thread, you must use `join()`.

**Use cases:**
- Analyze completed threads
- Debug execution flow
- Generate reports
- Display thread history

**To modify a thread:** Use `join()` to get write access and record new steps.

---

### 10. Joining Threads

**What it does:** Join an existing thread to add steps or modify it

**Variants:**
1. **With invitation token** - External or cross-company parties. Access level was set at invite time via `inviteParty`.
2. **Direct join** - Internal services with threadId. Always defaults to `participant` access level.

**Parameters:**
- `token` - JWT invitation token (access level comes from the invitation)
- OR
- `threadId` - Thread ID to join (direct join, defaults to participant)
- `role` - Role for direct join only (e.g., "supplier", "merchant")

**Returns:** Thread instance with write access (can record steps)

---

### 11. Inviting Parties

**What it does:** Create an invitation token for another party to join a thread

**Access Levels:**
- `Threadify.FOR_EXTERNAL` (default) - External party with limited access
- `Threadify.FOR_OBSERVER` - Read-only observer access
- `Threadify.FOR_PARTICIPANT` - Active participant access

**Parameters:**
- `role` (required) - Business/contract role (e.g., "supplier", "merchant")
- `accessLevel` (optional) - Use enum constants. Defaults to `FOR_EXTERNAL`.
- `expiresIn` (optional) - Token expiry duration (default: "24h")

**Returns:** Invitation token and metadata

**Example:**

```javascript
const invite = await thread.inviteParty({
  role: 'supplier',
  accessLevel: Threadify.FOR_OBSERVER
});
```

---

### 11. Contracts — When Rules Are Requested

Use contracts when the user requests workflow rules or validation. Basic tracking
works without a contract. Gherkin is the primary authoring format; YAML remains
available temporarily during migration.

```gherkin
Feature: refund_review
Rule: Record approval
  When step "approval" is submitted
  Then owner must be "reviewer"
  And this step is an entry point
  And content "payment_reference" must match regex "^PAY-[0-9]{8}$"

Rule: Issue a refund
  When step "refund_issued" is submitted
  Then owner must be "payments"
  And step "approval" must have succeeded
  And content "payment_reference" must equal approval.payment_reference
  And content "amount" must be a number greater than 0
  And content "currency" must be one of "GBP", "USD"
  And this step is terminal
```

**What is checked:**
- Required steps, ownership, ordering, and timing.
- Submitted details: numbers, allowed values, formats, and values from the same
  thread's latest validated successful occurrence of another step.
- Regex patterns use Go's RE2-style syntax. Use `^` and `$` for a whole-field
  match. Escape backslashes in quoted Gherkin strings; lookaround and
  backreferences are unsupported. Invalid patterns fail contract validation.

**Observation and permission are different:**
- Content checks happen during submission, before the event is recorded.
- Flow checks run asynchronously. A recording acknowledgement is not a passed
  validation result.
- In the JavaScript SDK, `await thread.waitFor("refund_issued")` waits for flow
  eligibility and claims one invocation. The application must perform the action
  only after it resolves. It cannot check future content from a step name alone.
- `.success(message, { waitFor: true })` or `.failed(message, { waitFor: true })`
  awaits validation of that exact reported event. It does not undo an action.
- Telemetry alone does not block tools, stop a process, or trigger business actions.
  The application must implement those responses.

For repeated actions, `step "approval" must succeed before each invocation`
requires a fresh approval. A repeatable action must not be terminal; use a
separate finishing step.

See the [contract vocabulary](https://github.com/creativeJoe007/ThreadifyEngine/blob/main/threadify-go/docs/GHERKIN_CONTRACTS.md)
and [execution waits](https://github.com/creativeJoe007/ThreadifyEngine/blob/main/threadify-go/docs/WAIT_FOR.md).

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

## OpenTelemetry Integration

Use existing OpenTelemetry instrumentation when available, or record workflow
steps through a Threadify SDK. Both feed execution evidence into the Engine.

**Mapping:**
- A reported span becomes a step, with span events represented as sub-steps.
- Explicit `threadify.thread_id` takes precedence. Otherwise correlation uses
  `threadify.external_ref`, then `workflow.run_id`, then the trace ID.
- Use a unique reference for each logical run, not a shared business category.
  Different traces can join the same run; incompatible contracts are rejected.
- Teams wanting trace-based grouping can disable `workflow.run_id` fallback in
  the SDK exporter or through `/v1/traces?use_workflow_run_id=false`.
- Ending one root span does not complete a run shared across traces. Use the
  explicit `threadify.run.complete` signal for a shared run.

Send standard OTLP/HTTP exports to your Engine's `/v1/traces` endpoint with a
Threadify API key. The SDK exporter also supports the Engine's WebSocket path.
Observing an export does not grant permission to perform an action; an application
must explicitly wait for a contract decision if it needs to gate execution.

## MCP: Investigate Recorded Execution

The Engine exposes a Streamable HTTP MCP endpoint at `/sse`, authenticated with
`X-API-Key`. Configure an MCP client that supports this transport with your own
Engine URL and an API key with the required read permissions. The Registry
license is for starting the Engine, not for authenticating MCP requests.

| Tool | Purpose | Example question |
| --- | --- | --- |
| `search_threads` | Find recorded workflow runs | Which refund runs reported a failure? |
| `get_thread` | Inspect a run's steps and results | What happened before this refund stopped? |
| `get_contract_violations` | Inspect reported rule violations | Which runs missed an approval? |
| `get_entity_profile` | Read a configured profile | What recorded history is linked to this agent? |
| `query` | Run a permitted GraphQL query | Retrieve the specific evidence needed for an investigation |

Use these tools to distinguish recorded facts from conclusions. Missing telemetry
is not proof that an action never occurred. A passed rule checks the reported
execution against that rule; it does not prove that an external service delivered
an outcome it did not report. An MCP investigation does not itself stop or approve
a tool call.

## When to Use Threadify

- Follow a workflow across services, AI agents, or partner handoffs.
- Check approval requirements, step order, repeated attempts, and submitted data.
- Keep the execution evidence needed to explain an outcome or a rule violation.
- Let applications await permission before selected actions.
- Look for recurring failures using recorded histories and configured profiles.

Start with one useful workflow or a small part of a larger process. Threadify
observes and checks that execution; the existing services and agents still run it.

---

## Documentation Links
- Full docs: https://docs.threadify.dev
- Quickstart: https://docs.threadify.dev/quickstart
- Core Concepts: https://docs.threadify.dev/core-concepts/overview
- API Reference: https://docs.threadify.dev/api-reference/overview
- MCP Integration: https://docs.threadify.dev/core-concepts/mcp-integration
