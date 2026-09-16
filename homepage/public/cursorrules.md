# Threadify SDK - Cursor Rules

## Core Principles
- Threadify tracks execution graphs (not just logs or events)
- Each thread = one customer request flowing through distributed systems
- Steps must have explicit status: success, failed, or error
- All context values are converted to strings automatically

## API Conventions

### Connection
```javascript
// Correct
const connection = await Threadify.connect('api-key', 'service-name');

// With options
const connection = await Threadify.connect('api-key', 'service-name', {
  wsUrl: 'wss://eng.threadify.dev/threads',
  graphqlUrl: 'https://eng.threadify.dev/graphql',
  debug: true
});
```

### Starting Threads
```javascript
// No contract
const thread = await connection.start();

// With specific service
const thread = await connection.start('payment-service');

// With contract
const thread = await connection.start('order_fulfillment', 'merchant-service');
```

### Recording Steps
```javascript
// CORRECT - use .addContext()
await thread.step('order_placed')
  .addContext({ order_id: '123', amount: '99.99' })
  .success();

// WRONG - don't use .context()
await thread.step('order_placed')
  .context({ order_id: '123' })  // ❌ This method doesn't exist
  .success();
```

### Step Status Methods
- `.success(messageOrData)` - Step completed successfully
- `.failed(messageOrData)` - Step failed (business logic failure)
- `.error(messageOrData)` - Step errored (system/technical error)

### Error Handling Pattern
```javascript
try {
  await processPayment();
  await thread.step('process_payment')
    .addContext({ transaction_id: txId })
    .success();
} catch (error) {
  await thread.step('process_payment')
    .addContext({ error: error.message })
    .failed();  // or .error() for system errors
}
```

## Common Mistakes to Avoid

1. **Don't create thread_id manually**
   - ❌ `const threadId = uuid.v4();`
   - ✅ Let SDK generate: `const thread = await connection.start();`

2. **Don't forget step status**
   - ❌ `thread.step('order_placed')`
   - ✅ `await thread.step('order_placed').success()`

3. **Don't use .context() method**
   - ❌ `.context({ data })`
   - ✅ `.addContext({ data })`

4. **Don't pass objects as context values**
   - ❌ `.addContext({ user: { id: 1, name: 'John' } })`
   - ✅ `.addContext({ user_id: '1', user_name: 'John' })`

5. **Don't mix up start() signatures**
   - ❌ `connection.start(context, role)` (old API)
   - ✅ `connection.start()` or `connection.start(contractName, serviceName)`

## Advanced Features

### Idempotency
```javascript
await thread.step('charge_payment')
  .idempotencyKey('payment-123')
  .addContext({ amount: '99.99' })
  .success();
```

### Sub-steps
```javascript
await thread.step('process_order')
  .subStep('validate_inventory', { items: 5 }, 'success')
  .subStep('calculate_tax', { tax: 12.50 }, 'success')
  .success();
```

### External References
```javascript
await thread.addRefs({
  payment_id: 'pi_123',
  order_id: 'ORD-456'
});
```

### Thread Linking
```javascript
await childThread.linkThread(parentThread.id, 'parent');
```

### Notifications (Event Subscriptions)
```javascript
// Subscribe to step events
connection.on('step.success', 'order_placed', (notification) => {
  console.log('Order placed:', notification.context);
  notification.ack();
});

// Subscribe to validation events
connection.on('rule.violated', 'payment_processed', (notification) => {
  console.log('Violation:', notification.severity);
  notification.ack();
});
```

### Joining Threads
```javascript
// With invitation token
const thread = await connection.join(invitationToken);

// Direct join (internal services)
const thread = await connection.join(threadId, 'participant');
```

## Step Naming Conventions
- Use snake_case: `order_placed`, `payment_processed`
- Be descriptive: `validate_inventory` not `step1`
- Use past tense: `order_placed` not `place_order`

## Context Best Practices
- Keep it flat (no nested objects)
- Use descriptive keys: `customer_id` not `cid`
- All values become strings automatically
- Include relevant business data for querying later

## Reference Documentation
- Full docs: https://docs.threadify.dev
- Quickstart: https://docs.threadify.dev/quickstart
- API Reference: https://docs.threadify.dev/api-reference/overview
- AI Guide: See AI.md in this repository
