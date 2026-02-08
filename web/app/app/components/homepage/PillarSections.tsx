interface PillarProps {
  number: string;
  title: string;
  description: string;
  imageSide?: 'left' | 'right';
}

function Pillar({ number, title, description, imageSide = 'left' }: PillarProps) {
  return (
    <section className="py-20 bg-white">
      <div className="max-w-7xl mx-auto px-6">
        <div className={`grid grid-cols-1 lg:grid-cols-2 gap-16 items-center ${imageSide === 'right' ? '' : 'lg:grid-flow-dense'}`}>
          {/* Image/Visual */}
          <div className={imageSide === 'right' ? 'lg:col-start-2' : 'lg:col-start-1'}>
            <div className="bg-gray-50 border border-gray-200 rounded-2xl h-96 flex items-center justify-center">
              <div className="w-32 h-32 rounded-full bg-white border border-gray-200 flex items-center justify-center">
                <span className="text-4xl font-bold text-gray-300">{number}</span>
              </div>
            </div>
          </div>

          {/* Content */}
          <div className={imageSide === 'right' ? 'lg:col-start-1 lg:row-start-1' : 'lg:col-start-2'}>
            <div className="flex items-start gap-4 mb-4">
              <div className="w-12 h-12 rounded-full border-2 border-black bg-white flex items-center justify-center flex-shrink-0">
                <span className="text-sm font-semibold">{number}</span>
              </div>
              <div>
                <h2 className="text-3xl font-bold mb-3">{title}</h2>
                <p className="text-base text-gray-600 leading-relaxed">
                  {description}
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

export default function PillarSections() {
  return (
    <>
      <Pillar
        number="01"
        title="Observe."
        description="Instrument and trace how your business apps deliver services to your customers. Capture every interaction across agents, apps, and human touchpoints."
        imageSide="left"
      />

      <Pillar
        number="02"
        title="Validate."
        description="Avoid workflow race conditions and keep your system honest. Threadify acts as your accountability buddy, ensuring your system follows the right process—not just guessing."
        imageSide="right"
      />

      <Pillar
        number="03"
        title="Detect."
        description="Leverage LLMs to detect patterns and business inefficiencies. Identify bottlenecks and opportunities for optimization automatically."
        imageSide="left"
      />

      <Pillar
        number="04"
        title="React."
        description="Intelligently respond with full workflow context. Make smarter decisions by accessing the complete history and state of your business processes."
        imageSide="right"
      />
    </>
  );
}
