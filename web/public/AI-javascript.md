# Threadify JavaScript SDK - Syntax Guide

> **Prerequisites:** Read [AI.md](https://threadify.dev/AI.md) for core concepts.

This file contains **JavaScript-specific syntax only**. For concepts, see AI.md.

---

## Installation

```bash
npm install @threadify/sdk
```

## Import

```javascript
import { Threadify } from '@threadify/sdk';
```

---

## Syntax Reference

### Connect
```javascript
const connection = await Threadify.connect('api-key', 'my-service');

// With options
const connection = await Threadify.connect('api-key', 'my-service', {
  wsUrl: 'wss://eng.threadify.dev/threads',
  debug: true
});
```

### Start Thread
```javascript
// With label (Recommended)
const thread = await connection.start('Order-123');

// With label and contract
const thread = await connection.start('Order-789', 'order_fulfillment');

// With label, contract, and options
const thread = await connection.start('Order-789', 'order_fulfillment', { serviceName: 'merchant-service' });
```

> **Tip:** Always provide a human-readable `label` when starting a thread. This makes it much easier to find and identify threads in the Threadify UI.

### Record Step
```javascript
await thread.step('order_placed')
  .addContext({ order_id: '123', amount: '99.99' })
  .success();
```

### Add Context
```javascript
.addContext({ key: 'value', another_key: 'another_value' })
```

### Set Idempotency Key

**Manual idempotency key** (use external system IDs):
```javascript
// Using payment provider transaction ID
await thread.step('charge_payment')
  .idempotencyKey(stripePayment.id)  // e.g., 'pi_3ABC123'
  .addContext({ amount: '99.99' })
  .success();

// Using user-initiated retry with request ID
await thread.step('retry_payment')
  .idempotencyKey(req.headers['x-request-id'])
  .addContext({ attempt: '2' })
  .success();
```

**Auto-generated** (default - no `.idempotencyKey()` call):
```javascript
// SDK generates hash from stepName + context
await thread.step('validate_cart')
  .addContext({ items: '3', total: '99.99' })
  .success();
// Idempotency key auto-generated from: 'validate_cart' + '{items:3,total:99.99}'
```

### Add Sub-Steps
```javascript
.subStep('validate_inventory', { items: 5 }, 'success')
.subStep('calculate_tax', { tax: 12.50 }, 'success')
```

### Step Status
```javascript
.success()
.success('Order placed successfully')
.success({ message: 'Order placed', order_id: 'ORD-123' })

.failed()
.failed('Payment declined')
.failed({ error: 'Payment declined', code: 'DECLINED' })

.error()
.error('Service unavailable')
.error({ error: 'Timeout', service: 'payment-api' })
```

### Add External References

Called on the **thread object**:

```javascript
// Add references to the thread
await thread.addRefs({
  stripe_payment_id: 'pi_123',
  order_id: 'ORD-456'
});

// Then record steps as normal
await thread.step('process_payment')
  .addContext({ amount: '99.99' })
  .success();
```

### Link Threads
```javascript
await childThread.linkThread(parentThread.id, 'parent');
```

### Retrieve Thread Data

**Important:** `getThread()` returns a **read-only** thread object for querying data. To add steps or modify a thread, you must use `join()`.

**Recommended:** Use `getCompleteData()` for efficiency (single query):

```javascript
// Wait for archival (1-2 seconds)
await new Promise(resolve => setTimeout(resolve, 2000));

// Get thread for READ-ONLY access
const thread = await connection.getThread(threadId);

// Get everything in one query (recommended)
const data = await thread.getCompleteData({
  stepHistoryLimit: 50,    // History per step
  validationLimit: 10      // Validation results
});

// Access: data.steps, data.validationResults, data.status, etc.
```

**Alternative:** Separate queries (use only if you need partial data):
```javascript
// Read-only access
const thread = await connection.getThread(threadId);
const steps = await thread.steps();                    // All steps
const validations = await thread.validationResults();  // All validations
```

**To modify a thread:** Use `join()` instead:
```javascript
// Join thread to add steps
const thread = await connection.join(threadId, 'participant');

// Now you can record steps
await thread.step('new_step').success();
```

### Query Thread Chain
```javascript
// Wait for archival (1-2 seconds)
await new Promise(resolve => setTimeout(resolve, 2000));

// Query from any thread in chain
const chain = await connection.getThreadChain(startThreadId, 3);
console.log('Chain:', chain.map(t => t.id));
```

### Subscribe to Events
```javascript
connection.subscribe('step.success', 'order_placed', (notification) => {
  console.log('Order placed:', notification.context);
  notification.ack();
});

connection.subscribe('rule.violated', 'payment_processed', (notification) => {
  console.log('Violation:', notification.severity);
  notification.ack();
});
```

### Unsubscribe from Events
```javascript
// Unsubscribe from specific step event
connection.unsubscribe('step.success', 'order_placed');

// Unsubscribe from thread-level event
connection.unsubscribe('thread.completed');
```

### Join Thread
```javascript
// With token (accessLevel comes from the invitation)
const thread = await connection.join(invitationToken);

// Direct join (defaults to participant accessLevel)
const thread = await connection.join(threadId);
const thread = await connection.join(threadId, 'supplier');
```

### Invite Party

```javascript
// Invite as external (default)
const invite = await thread.inviteParty({
  role: 'supplier'
});

// Invite as observer (read-only)
const invite = await thread.inviteParty({
  role: 'supplier',
  accessLevel: Threadify.FOR_OBSERVER
});

// Invite as participant (active)
const invite = await thread.inviteParty({
  role: 'inventory-service',
  accessLevel: Threadify.FOR_PARTICIPANT
});

// Join using the token
const thread = await connection.join(invite.token);
```

### Error Handling
```javascript
try {
  await processPayment();
  await thread.step('process_payment').success();
} catch (error) {
  await thread.step('process_payment')
    .addContext({ error: error.message })
    .failed();
}
```

### OpenTelemetry Exporter
```javascript
import { trace } from '@opentelemetry/api';
import { BasicTracerProvider, SimpleSpanProcessor } from '@opentelemetry/sdk-trace-base';
import { Threadify } from '@threadify/sdk';

const connection = await Threadify.connect('api-key', 'checkout-service');

// Create exporter (Optionally extract OTel attributes into Threadify refs)
const exporter = connection.createSpanExporter({ refs: ['order.id'] });

// Register with OTel
const provider = new BasicTracerProvider();
provider.addSpanProcessor(new SimpleSpanProcessor(exporter));
trace.setGlobalTracerProvider(provider);
```

---

## Common Mistakes

### ❌ Wrong: `.context()`
```javascript
.context({ data })  // Method doesn't exist!
```

### ✅ Correct: `.addContext()`
```javascript
.addContext({ data })
```

### ❌ Wrong: Nested objects
```javascript
.addContext({ user: { id: 1, name: 'John' } })
```

### ✅ Correct: Flat structure
```javascript
.addContext({ user_id: '1', user_name: 'John' })
```

### ❌ Wrong: Old start() signature
```javascript
connection.start({ customer_id: '123' }, 'customer')
```

### ✅ Correct: New start() signature
```javascript
const thread = await connection.start();
await thread.addRefs({ customer_id: '123' });
```

---

## Complete Example

```javascript
import { Threadify } from '@threadify/sdk';

const connection = await Threadify.connect('api-key', 'checkout-service');
const thread = await connection.start('Checkout Process');

// Add external references to the thread
await thread.addRefs({
  customer_id: '123',
  order_id: 'ORD-789'
});

await thread.step('validate_cart')
  .addContext({ items: '3', total: '99.99' })
  .success();

try {
  const payment = await processPayment();
  
  // Add payment provider reference
  await thread.addRefs({
    stripe_payment_id: payment.id
  });
  
  await thread.step('charge_payment')
    .addContext({ amount: '99.99', method: 'card' })
    .success();
} catch (error) {
  await thread.step('charge_payment')
    .addContext({ error: error.message })
    .failed();
}
```
