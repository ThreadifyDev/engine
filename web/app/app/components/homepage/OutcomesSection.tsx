export default function OutcomesSection() {
  const outcomes = [
    {
      title: 'Cryptographically Verified Audit Log',
      description: 'Funnels into an auditable log that is cryptographically verified.'
    },
    {
      title: 'Works How You Already Do',
      description: "Doesn't require changing current infrastructure, can fit into what you already have."
    },
    {
      title: 'Built-in SLA Tracking',
      description: 'SLA tracking comes in-built with time tracking for a thread.'
    }
  ];

  return (
    <section className="py-20 bg-gray-50">
      <div className="max-w-7xl mx-auto px-6">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-8 max-w-6xl mx-auto">
          {outcomes.map((outcome, index) => (
            <div key={index} className="bg-white border border-gray-200 rounded-lg p-6">
              <h3 className="text-lg font-semibold mb-3">{outcome.title}</h3>
              <p className="text-sm text-gray-600 leading-relaxed">{outcome.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
