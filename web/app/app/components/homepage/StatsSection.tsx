export default function StatsSection() {
  const stats = [
    { label: '99.9% Uptime', description: 'Highly available infrastructure' },
    { label: '<10ms P95', description: 'Low latency event processing' },
    { label: 'SOC2 Compliant', description: 'Enterprise-grade security' },
  ];

  return (
    <section className="py-32 bg-black text-white">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="text-center mb-20">
          <h2 className="text-4xl md:text-5xl font-bold mb-6">
            Scalable, secure, and compliant
          </h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-12">
          {stats.map((stat) => (
            <div key={stat.label} className="text-center">
              <div className="text-3xl font-bold mb-2">{stat.label}</div>
              <div className="text-gray-400">{stat.description}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
