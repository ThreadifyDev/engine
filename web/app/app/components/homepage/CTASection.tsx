export default function CTASection() {
  return (
    <section className="py-24 bg-gray-50">
      <div className="max-w-4xl mx-auto px-6 text-center">
        <h2 className="text-4xl font-bold text-gray-900 mb-6">
          Ready to Get Started?
        </h2>
        
        <p className="text-xl text-gray-600 mb-10">
          Start building workflow intelligence into your systems today
        </p>

        <div className="flex flex-col sm:flex-row gap-4 justify-center items-center">
          <a
            href="/signup"
            className="px-8 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors rounded-lg"
          >
            Get Started
          </a>
          <a
            href="mailto:contact@threadify.com"
            className="px-8 py-3 border-2 border-gray-900 text-gray-900 font-medium hover:bg-gray-50 transition-colors rounded-lg"
          >
            Contact Sales
          </a>
        </div>
      </div>
    </section>
  );
}
