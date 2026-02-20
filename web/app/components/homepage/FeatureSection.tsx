interface FeatureSectionProps {
  title: string;
  description: string;
  visual: 'left' | 'right';
  children?: React.ReactNode;
  gradient?: string;
  tagline?: string;
  footerTagline?: string;
}

export default function FeatureSection({ title, description, visual, children, gradient, tagline, footerTagline }: FeatureSectionProps) {
  // Split description by line breaks to support multi-paragraph content
  const paragraphs = description.split('\n').filter(p => p.trim());
  
  return (
    <section className="py-24 px-6">
      <div className="max-w-7xl mx-auto">
        <div className={`grid lg:grid-cols-2 gap-16 items-center ${
          visual === 'right' ? '' : 'lg:grid-flow-dense'
        }`}>
          {/* Text Content */}
          <div className={visual === 'right' ? '' : 'lg:col-start-2'}>
            {tagline && (
              <p className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">
                {tagline}
              </p>
            )}
            <h2 className={`text-5xl font-light mb-6 ${
              gradient 
                ? `bg-gradient-to-r ${gradient} bg-clip-text text-transparent` 
                : 'text-black'
            }`}>
              {title}
            </h2>
            <div className="space-y-4">
              {paragraphs.map((paragraph, index) => (
                <p key={index} className="text-xl text-gray-600 leading-relaxed">
                  {paragraph}
                </p>
              ))}
            </div>
            {footerTagline && (
              <p className="text-xs text-gray-500 mt-8">
                {footerTagline}
              </p>
            )}
          </div>

          {/* Visual Content */}
          <div className={`${visual === 'right' ? 'lg:col-start-2' : 'lg:col-start-1 lg:row-start-1'}`}>
            <div className="bg-gray-100 rounded-3xl aspect-square flex items-center justify-center p-12 border border-gray-200">
              {children || (
                <div className="w-32 h-32 bg-white rounded-full shadow-lg" />
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
