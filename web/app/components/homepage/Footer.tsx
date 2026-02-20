export default function Footer() {
  const currentYear = new Date().getFullYear();

  return (
    <footer className="bg-gray-50 border-t border-gray-200 py-12 px-6">
      <div className="max-w-7xl mx-auto">
        <div className="grid md:grid-cols-3 gap-8 mb-8">
          {/* Brand */}
          <div className="flex flex-col gap-2">
            <h3 className="text-xl font-semibold text-black">Threadify</h3>
            <p className="text-sm text-gray-600">Enabling intelligent systems and teams.</p>
          </div>

          {/* Resources */}
          <div className="flex flex-col gap-3">
            <h4 className="text-sm font-semibold text-black uppercase tracking-wider">Resources</h4>
            <a 
              href="https://docs.threadify.dev" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-sm text-gray-600 hover:text-black transition-colors"
            >
              Documentation
            </a>
            <a 
              href="/AI.md" 
              target="_blank"
              className="text-sm text-gray-600 hover:text-black transition-colors"
            >
              AI Assistant Guide
            </a>
          </div>

          {/* Empty column for spacing */}
          <div></div>
        </div>

        <div className="flex justify-center pt-6 border-t border-gray-200">
          <p className="text-sm text-gray-500">
            © {currentYear} Threadify. All rights reserved.
          </p>
        </div>
      </div>
    </footer>
  );
}
