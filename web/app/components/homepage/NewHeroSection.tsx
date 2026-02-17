export default function NewHeroSection() {
  return (
    <section className="relative bg-black text-white min-h-screen">
      {/* Navigation */}
      <nav className="max-w-7xl mx-auto px-6 py-6 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-xl font-bold tracking-wide">THREADIFY</span>
        </div>
        <div className="flex items-center gap-6">
          <a href="/login" className="text-sm text-gray-400 hover:text-white transition-colors">
            Sign In
          </a>
          <a 
            href="/signup" 
            className="px-4 py-2 bg-white text-black text-sm rounded-lg hover:bg-gray-200 transition-colors font-medium"
          >
            Get Started
          </a>
        </div>
      </nav>

      {/* Hero Content */}
      <div className="max-w-7xl mx-auto px-6 py-20">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center">
          {/* Left Column - Text Content */}
          <div>
            <h1 className="text-6xl font-bold mb-8 leading-tight">
              See execution as it happens, not hours later
            </h1>
            
            <p className="text-xl text-gray-400 mb-6">
              Distributed systems are complex. Customer requests flow through microservices, APIs, databases, and third-party integrations. When something breaks or a customer asks where's my order, you're stuck correlating logs across a dozen services.
            </p>
            
            <p className="text-xl text-gray-400 mb-6">
              Threadify captures execution as connected graphs with full context at every step. Not scattered events. Not inferred from logs. Purpose-built workflow data that shows you exactly what's happening—in real-time.
            </p>

            <div className="flex items-center gap-4 mb-12">
              <a 
                href="/signup" 
                className="px-8 py-3 bg-white text-black rounded-lg hover:bg-gray-200 transition-colors font-medium"
              >
                Start discovering
              </a>
              <a 
                href="#demo" 
                className="px-8 py-3 border border-gray-700 text-white rounded-lg hover:bg-gray-900 transition-colors font-medium"
              >
                See demo
              </a>
            </div>

            <p className="text-sm text-gray-600">
              Purpose-built workflow data • Cryptographically verified
            </p>
          </div>

          {/* Right Column - Execution Graph Visualization */}
          <div className="relative">
            <div className="bg-gray-900 rounded-lg p-6 border border-gray-800">
              {/* Thread Header */}
              <div className="mb-6 pb-4 border-b border-gray-800">
                <div className="text-xs text-gray-500 mb-1">Thread: customer_checkout_abc123</div>
                <div className="text-xs text-gray-600">sarah@example.com • $247.50</div>
              </div>

              {/* Execution Steps */}
              <div className="space-y-3">
                {/* Step 1 - Completed */}
                <div className="flex items-center gap-3 p-3 bg-gray-800 rounded-lg border border-gray-700">
                  <div className="w-3 h-3 bg-green-500 rounded-full flex-shrink-0"></div>
                  <div className="flex-1">
                    <div className="text-sm font-medium">validate_cart</div>
                    <div className="text-xs text-gray-500">Completed • 0.15s</div>
                  </div>
                </div>

                {/* Step 2 - Completed */}
                <div className="flex items-center gap-3 p-3 bg-gray-800 rounded-lg border border-gray-700">
                  <div className="w-3 h-3 bg-green-500 rounded-full flex-shrink-0"></div>
                  <div className="flex-1">
                    <div className="text-sm font-medium">check_inventory</div>
                    <div className="text-xs text-gray-500">Completed • 0.23s</div>
                  </div>
                </div>

                {/* Step 3 - Completed */}
                <div className="flex items-center gap-3 p-3 bg-gray-800 rounded-lg border border-gray-700">
                  <div className="w-3 h-3 bg-green-500 rounded-full flex-shrink-0"></div>
                  <div className="flex-1">
                    <div className="text-sm font-medium">charge_payment</div>
                    <div className="text-xs text-gray-500">Completed • 0.89s</div>
                  </div>
                </div>

                {/* Step 4 - In Progress */}
                <div className="flex items-center gap-3 p-3 bg-gray-800 rounded-lg border border-blue-700">
                  <div className="w-3 h-3 bg-blue-500 rounded-full flex-shrink-0 animate-pulse"></div>
                  <div className="flex-1">
                    <div className="text-sm font-medium">generate_shipping_label</div>
                    <div className="text-xs text-gray-500">In progress • 1.2s</div>
                  </div>
                </div>

                {/* Step 5 - Waiting */}
                <div className="flex items-center gap-3 p-3 bg-gray-800/50 rounded-lg border border-gray-800">
                  <div className="w-3 h-3 bg-gray-600 rounded-full flex-shrink-0"></div>
                  <div className="flex-1">
                    <div className="text-sm font-medium text-gray-500">send_confirmation</div>
                    <div className="text-xs text-gray-600">Waiting</div>
                  </div>
                </div>
              </div>

              {/* Context Panel */}
              <div className="mt-6 pt-4 border-t border-gray-800">
                <div className="text-xs text-gray-500 mb-2">CONTEXT</div>
                <div className="bg-black rounded p-3 font-mono text-xs text-gray-400">
                  <div>customer_tier: "VIP" • trial_ends: 3 days</div>
                  <div className="mt-1">sentiment: "price_sensitive" • cart_abandoned: 2</div>
                  <div className="mt-1">tip_recommended: upgrade_shipping</div>
                  <div className="mt-2 text-green-500">VALIDATED</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
