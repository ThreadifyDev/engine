import { Link } from '@remix-run/react';

export default function HeroSection() {
  return (
    <section className="relative min-h-[90vh] flex items-center justify-center bg-black text-white">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 text-center py-20">
        {/* Logo/Brand */}
        <h1 className="text-6xl md:text-7xl font-bold mb-6 tracking-tight" style={{ fontFamily: 'Block, monospace' }}>
          Threadify
        </h1>

        {/* Tagline */}
        <p className="text-3xl md:text-4xl text-white mb-6 font-light leading-tight">
          Keep your workflows honest.
        </p>

        {/* Description */}
        <p className="text-lg md:text-xl text-gray-400 mb-12 max-w-2xl mx-auto leading-relaxed">
          From instrumentation to validation, Threadify ensures every step in your business process is tracked, verified, and auditable.
        </p>

        {/* CTA Buttons */}
        <div className="flex flex-col sm:flex-row gap-4 justify-center items-center mb-20">
          <Link
            to="/auth/signup"
            className="px-8 py-3 bg-white text-black text-base font-medium hover:bg-gray-100 transition-colors rounded"
          >
            Start for free
          </Link>
          <Link
            to="/docs"
            className="px-8 py-3 text-gray-300 text-base font-medium hover:text-white transition-colors"
          >
            Read docs →
          </Link>
        </div>

        {/* Trust Badge */}
        <div className="text-sm text-gray-500">
          Processing millions of workflow steps every day
        </div>
      </div>
    </section>
  );
}
