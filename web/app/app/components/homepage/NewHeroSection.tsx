export default function NewHeroSection() {
  return (
    <section className="relative bg-white">
      {/* Navigation */}
      <nav className="max-w-7xl mx-auto px-6 py-6 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-xl font-bold">Threadify</span>
        </div>
        <div className="flex items-center gap-6">
          <a href="/login" className="text-sm text-gray-600 hover:text-black transition-colors">
            Sign In
          </a>
          <a 
            href="/signup" 
            className="px-4 py-2 bg-black text-white text-sm rounded-lg hover:bg-gray-800 transition-colors"
          >
            Get Started
          </a>
        </div>
      </nav>

      {/* Hero Content */}
      <div className="max-w-7xl mx-auto px-6 pt-20 pb-32">
        <div className="max-w-4xl mx-auto text-center">
          <h1 className="text-6xl font-bold mb-6">
            Workflow Intelligence for Your Business
          </h1>
          <p className="text-xl text-gray-600 mb-12 leading-relaxed">
            Build systems that understand your business workflow. Threadify creates context graphs 
            for your business processes by instrumenting from any source—agents, apps, humans, or frontend clients.
          </p>
          <div className="flex items-center justify-center gap-4">
            <a 
              href="/signup" 
              className="px-6 py-3 bg-black text-white rounded-lg hover:bg-gray-800 transition-colors font-medium"
            >
              Get Started
            </a>
            <a 
              href="#how-it-works" 
              className="px-6 py-3 border-2 border-gray-900 text-gray-900 rounded-lg hover:bg-gray-50 transition-colors font-medium"
            >
              Learn More
            </a>
          </div>
        </div>

        {/* Video Placeholder */}
        <div className="max-w-5xl mx-auto mt-16">
          <div className="relative bg-gray-900 rounded-2xl overflow-hidden shadow-2xl" style={{ paddingBottom: '56.25%' }}>
            {/* 16:9 aspect ratio container */}
            <div className="absolute inset-0 flex items-center justify-center">
              {/* Play button */}
              <button className="w-20 h-20 bg-white bg-opacity-90 rounded-full flex items-center justify-center hover:bg-opacity-100 transition-all transform hover:scale-110">
                <svg className="w-8 h-8 text-gray-900 ml-1" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M8 5v14l11-7z" />
                </svg>
              </button>
              {/* Optional: Add a subtle grid or pattern */}
              <div className="absolute inset-0 bg-gradient-to-br from-gray-800 to-gray-900 opacity-50" />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
