import { useEffect, useRef, useState } from 'react';

interface TimelineStepProps {
  number: string;
  title: string;
  description: string;
  isVisible: boolean;
  isLast?: boolean;
}

function TimelineStep({ number, title, description, isVisible, isLast = false }: TimelineStepProps) {
  return (
    <div className={`flex gap-0 transition-all duration-700 ${isVisible ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-8'}`}>
      {/* Left side - Number and line */}
      <div className="flex flex-col items-center w-20 flex-shrink-0">
        <div className="w-12 h-12 rounded-full border-2 border-black bg-white flex items-center justify-center">
          <span className="text-sm font-semibold">{number}</span>
        </div>
        {!isLast && <div className="w-px bg-gray-300 flex-grow mt-2" />}
      </div>

      {/* Right side - Content */}
      <div className="pb-32 pt-2">
        <h3 className="text-2xl font-bold mb-2">{title}</h3>
        <p className="text-base text-gray-600 leading-relaxed max-w-md">
          {description}
        </p>
      </div>
    </div>
  );
}

interface TimelineOutcomeProps {
  title: string;
  description: string;
  isVisible: boolean;
}

function TimelineOutcome({ title, description, isVisible }: TimelineOutcomeProps) {
  return (
    <div className={`transition-all duration-700 ${isVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}>
      <div className="border border-gray-200 rounded-lg p-6">
        <h4 className="text-lg font-semibold mb-2">{title}</h4>
        <p className="text-sm text-gray-600 leading-relaxed">{description}</p>
      </div>
    </div>
  );
}

export default function ThreadTimeline() {
  const [visibleSteps, setVisibleSteps] = useState<Set<number>>(new Set());
  const stepRefs = useRef<(HTMLDivElement | null)[]>([]);
  const outcomeRefs = useRef<(HTMLDivElement | null)[]>([]);

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const index = parseInt(entry.target.getAttribute('data-index') || '0');
            setVisibleSteps((prev) => new Set(prev).add(index));
          }
        });
      },
      { threshold: 0.2 }
    );

    [...stepRefs.current, ...outcomeRefs.current].forEach((ref) => {
      if (ref) observer.observe(ref);
    });

    return () => observer.disconnect();
  }, []);

  const pillars = [
    {
      number: '01',
      title: 'Observe.',
      description: 'Instrument and trace how your business apps deliver services to your customers. Capture every interaction across agents, apps, and human touchpoints.'
    },
    {
      number: '02',
      title: 'Validate.',
      description: 'Avoid workflow race conditions and keep your system honest. Threadify acts as your accountability buddy, ensuring your system follows the right process—not just guessing.'
    },
    {
      number: '03',
      title: 'Detect.',
      description: 'Leverage LLMs to detect patterns and business inefficiencies. Identify bottlenecks and opportunities for optimization automatically.'
    },
    {
      number: '04',
      title: 'React.',
      description: 'Intelligently respond with full workflow context. Make smarter decisions by accessing the complete history and state of your business processes.'
    }
  ];

  const outcomes = [
    {
      title: 'Cryptographically Verified Audit Log',
      description: 'Every workflow step is cryptographically verified, creating a tamper-proof audit trail for compliance and accountability.'
    },
    {
      title: 'Works How You Already Do',
      description: 'No infrastructure changes required. Threadify fits seamlessly into your existing systems and workflows.'
    },
    {
      title: 'Built-in SLA Tracking',
      description: 'Time tracking comes standard with every thread, giving you instant visibility into service delivery performance.'
    }
  ];

  return (
    <section className="bg-white py-24">
      <div className="max-w-4xl mx-auto px-6">
        <div className="text-center mb-16">
          <h2 className="text-4xl font-bold mb-4">How Threadify Works</h2>
          <p className="text-lg text-gray-600 max-w-2xl mx-auto">
            A complete workflow intelligence system that observes, validates, detects, and enables intelligent reactions
          </p>
        </div>

        {/* Timeline Steps */}
        <div className="mb-20">
          {pillars.map((pillar, index) => (
            <div
              key={index}
              ref={(el) => (stepRefs.current[index] = el)}
              data-index={index}
            >
              <TimelineStep
                number={pillar.number}
                title={pillar.title}
                description={pillar.description}
                isVisible={visibleSteps.has(index)}
                isLast={index === pillars.length - 1}
              />
            </div>
          ))}

          {/* Final node */}
          <div className="flex pl-6">
            <div className="w-12 h-12 rounded-full bg-black flex items-center justify-center">
              <div className="w-2 h-2 bg-white rounded-full" />
            </div>
          </div>
        </div>

        {/* Outcomes */}
        <div>
          <h3 className="text-3xl font-bold text-center mb-12">What You Get</h3>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {outcomes.map((outcome, index) => (
              <div
                key={index}
                ref={(el) => (outcomeRefs.current[index] = el)}
                data-index={pillars.length + index}
              >
                <TimelineOutcome
                  title={outcome.title}
                  description={outcome.description}
                  isVisible={visibleSteps.has(pillars.length + index)}
                />
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
