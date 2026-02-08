export default function FourPillarsSection() {
  const pillars = [
    {
      number: '01',
      name: 'Observe',
      description: 'Instrument and trace how your business apps deliver services to your customers'
    },
    {
      number: '02',
      name: 'Validate',
      description: 'Avoid race conditions and keep your system honest from a workflow perspective'
    },
    {
      number: '03',
      name: 'Detect',
      description: 'Use LLMs to detect patterns and business inefficiencies'
    },
    {
      number: '04',
      name: 'React',
      description: 'Intelligently react with the context needed to make smarter decisions'
    }
  ];

  return (
    <section className="py-32 bg-white">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Section Header */}
        <div className="text-center mb-20">
          <h2 className="text-4xl md:text-5xl font-bold text-gray-900 mb-6">
            Built-in workflow intelligence
          </h2>
          <p className="text-xl text-gray-600 max-w-2xl mx-auto">
            Threadify provides the infrastructure to manage the full lifecycle of your business workflows
          </p>
        </div>

        {/* Timeline */}
        <div className="relative">
          {/* Connection Line */}
          <div className="hidden md:block absolute top-12 left-0 right-0 h-0.5 bg-gray-200" style={{ top: '3rem' }} />
          
          {/* Timeline Items */}
          <div className="grid grid-cols-1 md:grid-cols-4 gap-8 md:gap-4">
            {pillars.map((pillar, index) => (
              <div key={pillar.name} className="relative">
                {/* Number Badge */}
                <div className="flex items-center justify-center md:justify-start mb-6">
                  <div className="w-12 h-12 rounded-full bg-black text-white flex items-center justify-center font-bold text-lg relative z-10">
                    {pillar.number}
                  </div>
                </div>

                {/* Content */}
                <div>
                  <h3 className="text-xl font-semibold text-gray-900 mb-2">
                    {pillar.name}
                  </h3>
                  <p className="text-sm text-gray-600 leading-relaxed">
                    {pillar.description}
                  </p>
                </div>

                {/* Arrow (mobile only) */}
                {index < pillars.length - 1 && (
                  <div className="md:hidden flex justify-center my-6">
                    <svg className="w-6 h-6 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 14l-7 7m0 0l-7-7m7 7V3" />
                    </svg>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
