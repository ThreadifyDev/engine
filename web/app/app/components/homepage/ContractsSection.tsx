export default function ContractsSection() {
  return (
    <section className="bg-white py-24">
      <div className="max-w-7xl mx-auto px-6">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center">
          {/* Text Content */}
          <div>
            {/* <div className="inline-block px-4 py-1 bg-gray-100 rounded-full text-sm text-gray-600 mb-6">
              Part of Validate
            </div> */}
            <h2 className="text-5xl font-bold mb-6">Contracts & Rulebooks</h2>
            <p className="text-xl text-gray-600 leading-relaxed mb-8">
              Define the rules that govern your business workflows. When delivering a business service, 
              multiple workflows operate together—contracts and rulebooks define the rules for each workflow.
            </p>
            <div className="space-y-4">
              <div className="flex items-start gap-3">
                <div className="w-6 h-6 rounded-full border-2 border-gray-900 flex-shrink-0 mt-1" />
                <div>
                  <h4 className="font-semibold mb-1">Workflow Rules</h4>
                  <p className="text-gray-600">Define expected behavior for each business process</p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <div className="w-6 h-6 rounded-full border-2 border-gray-900 flex-shrink-0 mt-1" />
                <div>
                  <h4 className="font-semibold mb-1">Race Condition Prevention</h4>
                  <p className="text-gray-600">Ensure workflows execute in the correct order</p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <div className="w-6 h-6 rounded-full border-2 border-gray-900 flex-shrink-0 mt-1" />
                <div>
                  <h4 className="font-semibold mb-1">Accountability</h4>
                  <p className="text-gray-600">Keep your system honest with automated validation</p>
                </div>
              </div>
            </div>
          </div>

          {/* Visual */}
          <div className="bg-gray-50 border-2 border-gray-200 rounded-2xl p-12">
            <div className="font-mono text-sm space-y-4">
              <div className="text-gray-400">// Example Contract</div>
              <div>
                <span className="text-gray-600">contract:</span> <span className="text-gray-900">order_fulfillment</span>
              </div>
              <div className="ml-4">
                <span className="text-gray-600">steps:</span>
              </div>
              <div className="ml-8 space-y-2">
                <div>- payment_processing</div>
                <div>- inventory_check</div>
                <div>- shipping_label</div>
                <div>- delivery_confirmation</div>
              </div>
              <div className="ml-4 mt-4">
                <span className="text-gray-600">rules:</span>
              </div>
              <div className="ml-8 space-y-2">
                <div>- payment before shipping</div>
                <div>- inventory before label</div>
                <div>- max_duration: 24h</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
