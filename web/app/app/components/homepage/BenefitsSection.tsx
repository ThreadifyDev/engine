export default function BenefitsSection() {
  const benefits = [
    {
      title: 'Cryptographically verified audit log',
      description: 'Every step is recorded in an immutable, tamper-proof audit log with cryptographic hash chains.'
    },
    {
      title: 'No infrastructure changes required',
      description: 'Threadify fits into your existing infrastructure. Works with any tech stack or framework.'
    },
    {
      title: 'Built-in SLA tracking',
      description: 'Track time for every thread and step. Know exactly where bottlenecks occur and measure performance.'
    }
  ];

  return (
    <section className="py-32 bg-gray-50">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Section Header */}
        <div className="text-center mb-20">
          <h2 className="text-4xl md:text-5xl font-bold text-gray-900 mb-6">
            Works the way you build
          </h2>
        </div>

        {/* Benefits List */}
        <div className="space-y-16">
          {benefits.map((benefit) => (
            <div key={benefit.title} className="max-w-3xl">
              <h3 className="text-2xl font-semibold text-gray-900 mb-3">
                {benefit.title}
              </h3>
              <p className="text-lg text-gray-600 leading-relaxed">
                {benefit.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
