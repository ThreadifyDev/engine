import { useState } from 'react';

export default function CodeExampleSection() {
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null);

  const examples = [
    {
      title: 'Start a Thread',
      language: 'javascript',
      code: `// Initialize connection
const connection = new ThreadifyConnection({
  apiKey: process.env.THREADIFY_API_KEY,
  wsUrl: 'wss://api.threadify.com'
});

// Start a new workflow thread
const thread = await connection.start(
  'order-processing',
  'merchant-service'
);

console.log('Thread started:', thread.id);`
    },
    {
      title: 'Record Steps',
      language: 'javascript',
      code: `// Record successful step
await thread.step('order_placed')
  .context({ orderId: '12345', amount: 99.99 })
  .success();

// Record failed step with retry
await thread.step('payment_processing')
  .context({ gateway: 'stripe' })
  .failed('Payment gateway timeout');

// Record with external refs
await thread.step('shipment_created')
  .addRefs({ trackingId: 'USPS-123456' })
  .success();`
    },
    {
      title: 'Subscribe to Notifications',
      language: 'javascript',
      code: `// Get notified of violations
connection.onViolation('order_placed', (notification) => {
  console.log('Violation detected:', notification);
  
  // Take action based on severity
  if (notification.severity === 'critical') {
    alertOncall(notification);
  }
  
  notification.ack();
});

// Handle step failures
connection.onFailed('payment_processing', (notification) => {
  retryPayment(notification.context);
  notification.ack();
});`
    }
  ];

  const copyToClipboard = (code: string, index: number) => {
    navigator.clipboard.writeText(code);
    setCopiedIndex(index);
    setTimeout(() => setCopiedIndex(null), 2000);
  };

  return (
    <section className="py-32 bg-white">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Section Header */}
        <div className="text-center mb-20">
          <h2 className="text-4xl md:text-5xl font-bold text-gray-900 mb-6">
            Simple to integrate
          </h2>
          <p className="text-xl text-gray-600 max-w-2xl mx-auto">
            Get started in minutes with our intuitive SDK.
          </p>
        </div>

        {/* Code Examples */}
        <div className="space-y-12">
          {examples.slice(0, 2).map((example, index) => (
            <div key={example.title}>
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold text-gray-900">{example.title}</h3>
                <button
                  onClick={() => copyToClipboard(example.code, index)}
                  className="text-sm text-gray-600 hover:text-gray-900 transition-colors"
                >
                  {copiedIndex === index ? 'Copied!' : 'Copy'}
                </button>
              </div>
              <div className="bg-gray-900 rounded-lg overflow-hidden border border-gray-800">
                <div className="p-6 overflow-x-auto">
                  <pre className="text-sm text-gray-300 font-mono leading-relaxed">
                    <code>{example.code}</code>
                  </pre>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
