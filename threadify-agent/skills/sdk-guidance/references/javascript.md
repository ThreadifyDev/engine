# JavaScript SDK fallback

Bundled from the Threadify SDK and documentation. Confirm exact signatures
with `get_developer_reference` when it is available.

```sh
npm install @threadify/sdk
```

```js
import { Threadify } from '@threadify/sdk';

const connection = await Threadify.connect(
  process.env.THREADIFY_API_KEY,
  'orders-service',
  { engineUrl: 'https://threadify.example.com' }
);
try {
  const thread = await connection.thread('order:ORD-123', {
    label: 'Order ORD-123',
    contract: 'order_processing',
    refs: { order_id: 'ORD-123' },
  });
  await thread.step('order_received')
    .addContext({ order_id: 'ORD-123' })
    .success('Order accepted');
} finally {
  await connection.close();
}
```

Use the application's stable key to resume the same active thread. The Engine
URL option derives WebSocket and GraphQL paths, including proxy prefixes.
