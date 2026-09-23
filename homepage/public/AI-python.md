# Threadify Python SDK - Syntax Guide

> **Prerequisites:** Read [AI.md](https://threadify.dev/AI.md) for core concepts.

This file contains **Python-specific syntax only**. For concepts, see AI.md.

Threadify follows work your services and agents already perform. Bind a contract
when rules are needed; use recorded steps for evidence and explicitly await a
permission check where the application must wait before acting. Recording
telemetry alone does not stop actions or guarantee a rule passed.

---

## Installation

```bash
pip install threadify-sdk
```

**Optional (OTel only):**
```bash
pip install opentelemetry-api opentelemetry-sdk
```

## Import

```python
from threadify import Threadify
```

---

## Syntax Reference

### Connect
```python
conn = await Threadify.connect("api-key", service_name="my-service")

# With options
conn = await Threadify.connect(
    "api-key",
    service_name="my-service",
    debug=True,
)
```

### Create or Resume a Thread

```python
# Initialize once before requests or telemetry use this session.
thread = await conn.thread(session_id, {
    "label": "Agent session",
    "contract": "agent_contract:3",  # Omit for a free-form session.
    "refs": {"customer_id": customer_id},
    "tags": ["production"],
})

# A later request or worker uses only the application's session ID.
resumed = await conn.thread(session_id)
```

Options: `label`, `contract`, `refs` (string values), `tags`, `service_name`, and
`role`. Use `thread.add_refs(refs)` for explicit reference updates.

The key identifies one workflow or agent session within your company; Threadify
manages the internal `threadId`. Reuse the same key across requests and workers.
Keys are trimmed, nonblank strings of at most 1024 UTF-8 bytes. Concurrent calls
resolve to one thread and normal write permissions still apply.

The optional object supplies creation defaults. Resuming loads the stored contract
and pinned version without redefining them. A conflicting contract or version is
rejected. Existing labels, refs, and tags are preserved; update refs explicitly
with the SDK's reference method.

A new key with no options creates a free-form thread. Initialize contracted
sessions before their turns or telemetry begin: a free-form thread cannot acquire
a contract on resume. Closed threads cannot resume or accept writes; choose a new
key for a new execution. Set `threadify.thread_key` to this same key in OTLP spans
or `ThreadifySpanExporter` instrumentation.

### Record Step
```python
result = await (
    thread.step("order_placed")
    .add_context({"order_id": "123", "amount": "99.99"})
    .success()
)
```

### Add Context
```python
.add_context({
    "key": "value",
    "another_key": "another_value",
})
```

### Set Idempotency Key

**Manual idempotency key** (use external system IDs):
```python
# Using payment provider transaction ID
result = await (
    thread.step("charge_payment")
    .idempotency_key(payment.id)  # e.g., "pi_3ABC123"
    .add_context({"amount": "99.99"})
    .success()
)

# Using user-initiated retry with request ID
result = await (
    thread.step("retry_payment")
    .idempotency_key(request.headers.get("x-request-id", ""))
    .add_context({"attempt": "2"})
    .success()
)
```

**Auto-generated** (default - no `.idempotency_key()` call):
```python
# SDK generates hash from stepName + context
result = await (
    thread.step("validate_cart")
    .add_context({"items": "3", "total": "99.99"})
    .success()
)
# Idempotency key auto-generated from: 'validate_cart' + '{"items": "3", "total": "99.99"}'
```

### Add Sub-Steps
```python
result = await (
    thread.step("payment_processed")
    .sub_step("validate_card", {"card_type": "visa"}, "success")
    .sub_step("check_fraud", {"fraud_score": 0.15}, "success")
    .sub_step("authorize_payment", {"auth_code": "AUTH-123"}, "success")
    .add_context({"total_amount": "299.99"})
    .success()
)

# Sub-step status can be "success" or "failed" (optional, defaults to "success")
.sub_step("error_handler", {"error": "timeout"}, "failed")
```

### Step Status
```python
.success()
.success("Order placed successfully")
.success({"message": "Order placed", "order_id": "ORD-123"})

.failed()
.failed("Payment declined")
.failed({"error": "Payment declined", "code": "DECLINED"})

.error()
.error("Service unavailable")
.error({"error": "Timeout", "service": "payment-api"})
```

### Add External References

**Thread-level references** (called on thread object):

```python
# Add references to the thread
await thread.add_refs({
    "payment_id": "pi_123",
    "order_id": "ORD-456",
})
```

### Link Threads
```python
await child_thread.link_thread(parent_thread.thread_id, "parent")

# Default relationship is "parent" if not specified
await child_thread.link_thread(parent_thread.thread_id)
```

### Add Private Context
```python
# Private context is prefixed with "private_" and excluded from certain queries
result = await (
    thread.step("process_payment")
    .add_private_context({
        "card_number": "4111111111111111",
        "cvv": "123",
    })
    .add_context({"amount": "99.99"})
    .success()
)
```

### Retrieve Thread Data

**Important:** `get_thread()` returns a **read-only** thread object for querying data. To add steps or modify a thread, resume with `await conn.thread(thread_key)` or use `join()` with an internal ID or invitation.

**Recommended:** Use `get_complete_data()` for efficiency (single query):

```python
# Wait for archival (1-2 seconds)
await asyncio.sleep(2)

# Get thread for READ-ONLY access
archived = await conn.get_thread(thread_id)

# Get everything in one query (recommended)
data = await archived.get_complete_data(
    step_history_limit=50,    # History per step
    validation_limit=10       # Validation results
)

# Access: data.steps, data.validation_results, data.status, etc.
```

**Alternative:** Separate queries (use only if you need partial data):

```python
# Read-only access
archived = await conn.get_thread(thread_id)

steps = await archived.steps()                          # All steps
validations = await archived.validation_results(10)      # All validations
```

**To modify a thread:** Use `join()` instead:

```python
# Join thread to add steps
thread = await conn.join(thread_id=thread_id, role="participant")

# Now you can record steps
await thread.step("new_step").success()
```

### Query Thread Chain
```python
# Wait for archival (1-2 seconds)
await asyncio.sleep(2)

# Query from any thread in chain
chain = await conn.get_thread_chain(start_thread_id, max_depth=3)
for t in chain:
    print("Thread ID:", t.id)
```

### Query Threads by Reference
```python
# Find threads by external reference
threads = await conn.get_threads_by_ref(
    RefQuery(ref_key="order_id", ref_value="ORD-12345", status="completed", limit=10)
)
for thread in threads:
    print("Thread:", thread.id, "Status:", thread.status)
```

### Subscribe to Events
```python
conn.subscribe("step.success", "order_placed", lambda n: (
    print("Order placed:", n.step_name),
    n.ack(),
))

# For contracted threads: Subscribe to rule-check results
conn.subscribe("rule.violated", "payment_processed", lambda n: (
    print("Violation:", n.severity),
    n.ack(),
))
```

### Unsubscribe from Events
```python
# Unsubscribe from specific step event
conn.unsubscribe("step.success", "order_placed")

# Unsubscribe from thread-level event
conn.unsubscribe("thread.completed")
```

### Notification Helper Methods
```python
# Check severity (for contract validation events)
if notif.is_critical:
    # Critical severity
if notif.is_warning:
    # Warning severity
if notif.is_info:
    # Info severity

# Check step status
if notif.is_success:
    # Step succeeded
if notif.is_failed:
    # Step failed
if notif.is_error:
    # Step errored

# Check if already acknowledged
if not notif.is_acknowledged:
    notif.ack()

# Convert to string
print(str(notif))  # "[critical] order_placed: Payment validation failed"
```

### Join Thread
```python
# With token (accessLevel comes from the invitation)
thread = await conn.join(token=invitation_token)

# Direct join (defaults to participant accessLevel)
thread = await conn.join(thread_id=thread_id)
thread = await conn.join(thread_id=thread_id, role="supplier")
```

### Invite Parties
```python
# Create invitation for external party (default)
invitation = await thread.invite_party(
    InviteOptions(
        role="supplier",
        access_level=Threadify.FOR_EXTERNAL,  # Optional: FOR_EXTERNAL (default), FOR_OBSERVER, FOR_PARTICIPANT
        expires_in="48h",                   # Optional, defaults to "24h"
    )
)

# Invite as observer (read-only)
invitation = await thread.invite_party(
    InviteOptions(role="supplier", access_level=Threadify.FOR_OBSERVER)
)

# Invite as participant (active)
invitation = await thread.invite_party(
    InviteOptions(role="inventory-service", access_level=Threadify.FOR_PARTICIPANT)
)

# Share invitation token
print("Token:", invitation.token)
print("Expires at:", invitation.expires_at)
```

### Thread Lifecycle Management
```python
# Complete thread successfully
resp = await thread.complete("Order fulfilled")
print("Thread completed at:", resp.ended_at)

# Close/cancel thread
resp = await thread.close("User cancelled order")

# End thread with custom status
resp = await thread.end("custom_status", "Custom reason")
```

### Error Handling
```python
try:
    await process_payment()
    await thread.step("process_payment").success()
except Exception as e:
    await (
        thread.step("process_payment")
        .add_context({"error": str(e)})
        .failed()
    )
```

### OpenTelemetry Integration — ONLY for Existing OTel Codebases

**Default to manual instrumentation.** Only use OTel if the user explicitly asks OR the codebase already imports `opentelemetry`.

```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from threadify import Threadify

# 1. Connect to Threadify
conn = await Threadify.connect("api-key", service_name="checkout-service")

# 2. Create the Exporter
exporter = conn.create_span_exporter(options={"refs": ["order.id", "customer.id"]})

# Tag threads via OTel span attributes
span.set_attribute("threadify.tags", ["production", "v2.1"])

# Filter spans by name — exact match or prefix wildcard with *
exporter = conn.create_span_exporter(options={
    "refs": ["order.id", "customer.id"],
    "filters": ["invoke_llm", "adk.before*", "llm.*"],
})

# 3. Register with OpenTelemetry
provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)
```

---

## Common Mistakes

### ❌ Wrong: `.context()`
```python
.context({"data": "value"})  # Method doesn't exist!
```

### ✅ Correct: `.add_context()`
```python
.add_context({"data": "value"})
```

### ❌ Wrong: Nested objects
```python
.add_context({"user": {"id": "1", "name": "John"}})
```

### ✅ Correct: Flat structure
```python
.add_context({"user_id": "1", "user_name": "John"})
```

### ❌ Wrong: Forgetting `await`
```python
thread.step("order_placed").success()  # Missing await!
```

### ✅ Correct: Always await async methods
```python
await thread.step("order_placed").success()
```

### ❌ Wrong: Using sync style for status methods
```python
result = thread.step("order_placed").success()  # Returns coroutine, not result!
```

### ✅ Correct: Await the final status call
```python
result = await thread.step("order_placed").success()
```

---

## Complete Example

```python
import asyncio
from threadify import Threadify


async def main():
    conn = await Threadify.connect(
        "api-key",
        service_name="checkout-service",
    )

    thread = await conn.thread("order:ORD-789", {"label": "Checkout Process"})

    # Add external references to the thread
    await thread.add_refs({
        "customer_id": "123",
        "order_id": "ORD-789",
    })

    await (
        thread.step("validate_cart")
        .add_context({"items": "3", "total": "99.99"})
        .success()
    )

    try:
        payment = await process_payment()

        # Add payment provider reference
        await thread.add_refs({"payment_id": payment["id"]})

        await (
            thread.step("charge_payment")
            .add_context({"amount": "99.99", "method": "card"})
            .success()
        )
    except Exception as e:
        await (
            thread.step("charge_payment")
            .add_context({"error": str(e)})
            .failed()
        )

    await conn.close()


async def process_payment():
    # Payment processing logic
    return {"id": "pi_123"}


if __name__ == "__main__":
    asyncio.run(main())
```
