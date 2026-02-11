import { useEffect, useState } from 'react';

interface Step {
  name: string;
  status: 'completed' | 'in-progress' | 'waiting';
  duration?: string;
}

export default function LiveThreadDemo() {
  const [steps, setSteps] = useState<Step[]>([
    { name: 'validate_cart', status: 'completed', duration: '0.15s' },
    { name: 'check_inventory', status: 'completed', duration: '0.42s' },
    { name: 'charge_payment', status: 'completed', duration: '0.89s' },
    { name: 'generate_shipping_label', status: 'in-progress', duration: '1.2s' },
    { name: 'send_confirmation', status: 'waiting' },
  ]);

  return (
    <div className="bg-white rounded-lg p-6 border border-gray-300 w-full max-w-md shadow-lg">
      {/* Thread Header */}
      <div className="mb-6">
        <div className="text-sm text-gray-600 mb-1">
          Thread: <span className="text-black font-mono">customer_checkout_abc123</span>
        </div>
        <div className="text-xs text-gray-500">
          sarah@example.com • $247.50
        </div>
      </div>

      {/* Steps Timeline */}
      <div className="space-y-4 mb-6">
        {steps.map((step, index) => (
          <div key={index} className="relative">
            {/* Connecting Line */}
            {index < steps.length - 1 && (
              <div className="absolute left-3 top-8 w-0.5 h-8 bg-gray-300" />
            )}
            
            {/* Step Card */}
            <div className="flex items-start gap-3">
              {/* Status Dot */}
              <div className={`w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-1 ${
                step.status === 'completed' ? 'bg-green-500/20' :
                step.status === 'in-progress' ? 'bg-blue-500/20' :
                'bg-gray-200'
              }`}>
                <div className={`w-2.5 h-2.5 rounded-full ${
                  step.status === 'completed' ? 'bg-green-500' :
                  step.status === 'in-progress' ? 'bg-blue-500 animate-pulse' :
                  'bg-gray-500'
                }`} />
              </div>

              {/* Step Content */}
              <div className="flex-1 bg-gray-50 rounded-lg p-3 border border-gray-200">
                <div className="font-mono text-sm text-black mb-1">
                  {step.name}
                </div>
                <div className="text-xs text-gray-600">
                  {step.status === 'completed' && `Completed • ${step.duration}`}
                  {step.status === 'in-progress' && `In progress • ${step.duration}`}
                  {step.status === 'waiting' && 'Waiting'}
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Context Section */}
      <div className="border-t border-gray-300 pt-4">
        <div className="text-xs text-gray-500 uppercase tracking-wider mb-2">
          Context
        </div>
        <div className="bg-gray-100 rounded p-3 font-mono text-xs text-gray-600 space-y-1">
          <div>customer_tier: "VIP"</div>
          <div>trial_ends: 3 days</div>
          <div>sentiment: "price_sensitive"</div>
          <div>cart_abandoned: 2</div>
          <div>llm_recommendation: upgrade_shipping</div>
        </div>
        <div className="mt-2 inline-block px-2 py-1 bg-green-500/10 text-green-400 text-xs rounded border border-green-500/20">
          VALIDATED
        </div>
      </div>
    </div>
  );
}
