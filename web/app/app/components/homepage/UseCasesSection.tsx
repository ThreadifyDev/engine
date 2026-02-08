export default function UseCasesSection() {
  const useCases = [
    {
      title: 'E-Commerce Order Processing',
      icon: '🛒',
      description: 'Track orders from placement to delivery, validate payment flows, and detect fulfillment bottlenecks',
      metrics: [
        { label: 'Order Accuracy', value: '99.8%' },
        { label: 'Avg Processing Time', value: '2.3s' }
      ],
      color: 'from-blue-500 to-blue-600'
    },
    {
      title: 'Financial Transactions',
      icon: '💰',
      description: 'Ensure compliance, prevent fraud, and maintain audit trails for every transaction',
      metrics: [
        { label: 'Fraud Detection', value: '99.9%' },
        { label: 'Compliance Rate', value: '100%' }
      ],
      color: 'from-green-500 to-green-600'
    },
    {
      title: 'Supply Chain Management',
      icon: '📦',
      description: 'Monitor shipments, validate handoffs between parties, and react to delays in real-time',
      metrics: [
        { label: 'On-Time Delivery', value: '94%' },
        { label: 'Issue Detection', value: '<5min' }
      ],
      color: 'from-purple-500 to-purple-600'
    },
    {
      title: 'Customer Onboarding',
      icon: '👤',
      description: 'Track multi-step onboarding flows, validate document submissions, and ensure regulatory compliance',
      metrics: [
        { label: 'Completion Rate', value: '87%' },
        { label: 'Avg Time to Complete', value: '4.2 days' }
      ],
      color: 'from-orange-500 to-orange-600'
    }
  ];

  return (
    <section className="py-24 bg-gradient-to-b from-gray-50 to-white">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Section Header */}
        <div className="text-center mb-16">
          <h2 className="text-5xl font-bold text-gray-900 mb-4">
            Built for Real-World Workflows
          </h2>
          <p className="text-xl text-gray-600 max-w-3xl mx-auto">
            From e-commerce to finance, Threadify helps teams maintain accountability across complex business processes
          </p>
        </div>

        {/* Use Cases Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
          {useCases.map((useCase) => (
            <div
              key={useCase.title}
              className="bg-white rounded-xl p-8 shadow-lg hover:shadow-2xl transition-all duration-300 border border-gray-200 group"
            >
              {/* Icon with gradient */}
              <div className={`w-16 h-16 rounded-xl bg-gradient-to-br ${useCase.color} flex items-center justify-center text-3xl mb-6 shadow-lg group-hover:scale-110 transition-transform duration-300`}>
                {useCase.icon}
              </div>

              {/* Title */}
              <h3 className="text-2xl font-bold text-gray-900 mb-4">
                {useCase.title}
              </h3>

              {/* Description */}
              <p className="text-gray-600 mb-6 leading-relaxed">
                {useCase.description}
              </p>

              {/* Metrics */}
              <div className="grid grid-cols-2 gap-4 pt-6 border-t border-gray-200">
                {useCase.metrics.map((metric) => (
                  <div key={metric.label}>
                    <div className="text-2xl font-bold text-gray-900 mb-1">
                      {metric.value}
                    </div>
                    <div className="text-sm text-gray-500">
                      {metric.label}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>

        {/* Bottom CTA */}
        <div className="mt-16 text-center">
          <p className="text-lg text-gray-600 mb-6">
            Don't see your use case? Threadify adapts to any workflow.
          </p>
          <a
            href="mailto:sales@threadify.com"
            className="inline-block px-8 py-4 bg-black text-white font-semibold hover:bg-gray-800 transition-colors rounded-lg"
          >
            Talk to Sales
          </a>
        </div>
      </div>
    </section>
  );
}
