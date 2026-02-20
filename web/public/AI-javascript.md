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
// No contract
const thread = await connection.start();

// With service name
const thread = await connection.start('payment-service');

// With contract
const thread = await connection.start('order_fulfillment', 'merchant-service');
```

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
connection.on('step.success', 'order_placed', (notification) => {
  console.log('Order placed:', notification.context);
  notification.ack();
});

connection.on('rule.violated', 'payment_processed', (notification) => {
  console.log('Violation:', notification.severity);
  notification.ack();
});
```

### Join Thread
```javascript
// With token
const thread = await connection.join(invitationToken);

// Direct join
const thread = await connection.join(threadId, 'participant');
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
const thread = await connection.start();

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
