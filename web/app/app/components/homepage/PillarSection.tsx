interface PillarSectionProps {
  title: string;
  description: string;
  imageSide?: 'left' | 'right';
  children?: React.ReactNode;
  bordered?: boolean;
}

export default function PillarSection({ 
  title, 
  description, 
  imageSide = 'right',
  children,
  bordered = false
}: PillarSectionProps) {
  const containerClasses = bordered 
    ? "border-2 border-blue-400 rounded-2xl p-16" 
    : "p-16";

  return (
    <section className={`max-w-7xl mx-auto px-6 py-8 ${containerClasses}`}>
      <div className={`grid grid-cols-1 lg:grid-cols-2 gap-16 items-center ${imageSide === 'left' ? 'lg:flex-row-reverse' : ''}`}>
        {/* Text Content */}
        <div className={imageSide === 'left' ? 'lg:order-2' : ''}>
          <h2 className="text-5xl font-bold mb-6">{title}</h2>
          <p className="text-xl text-gray-500 leading-relaxed">
            {description}
          </p>
          {children}
        </div>

        {/* Image Placeholder */}
        <div className={imageSide === 'left' ? 'lg:order-1' : ''}>
          <div className="bg-gray-100 rounded-2xl h-96 flex items-center justify-center">
            <div className="text-gray-300 text-6xl font-bold">
              {title.charAt(0)}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
