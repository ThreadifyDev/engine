import { useState } from 'react';
import { useTiers, useCreateCheckout } from '~/hooks/useBilling';

interface TierSelectionModalProps {
  isOpen: boolean;
  onClose: () => void;
  currentTier?: string;
}

export default function TierSelectionModal({ isOpen, onClose, currentTier }: TierSelectionModalProps) {
  const { data, isLoading, error } = useTiers();
  const checkoutMutation = useCreateCheckout();
  const [billingCycle, setBillingCycle] = useState<'monthly' | 'yearly'>('monthly');
  const [selectedTier, setSelectedTier] = useState<string | null>(null);

  if (!isOpen) return null;

  const handleSelectTier = async (tierName: string) => {
    setSelectedTier(tierName);
    try {
      const result = await checkoutMutation.mutateAsync({
        tier: tierName,
        billingCycle,
      });
      if (result.checkout_url) {
        window.open(result.checkout_url, '_blank');
        onClose();
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to create checkout session');
      setSelectedTier(null);
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
    return `${(bytes / Math.pow(k, i)).toFixed(0)} ${sizes[i]}`;
  };

  const formatNumber = (num: number) => {
    return new Intl.NumberFormat().format(num);
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4 overflow-y-auto">
      <div className="bg-white rounded-lg p-8 max-w-6xl w-full my-8">
        <div className="flex justify-between items-start mb-6">
          <div>
            <h2 className="text-3xl font-bold mb-2">Choose Your Plan</h2>
            <p className="text-gray-600">Select the plan that best fits your needs</p>
          </div>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-700 text-2xl font-bold"
          >
            ×
          </button>
        </div>

        {/* Billing Cycle Toggle */}
        <div className="flex justify-center mb-8">
          <div className="inline-flex border-2 border-black rounded-lg overflow-hidden">
            <button
              onClick={() => setBillingCycle('monthly')}
              className={`px-6 py-3 font-medium transition-colors ${
                billingCycle === 'monthly'
                  ? 'bg-black text-white'
                  : 'bg-white text-black hover:bg-gray-100'
              }`}
            >
              Monthly
            </button>
            <button
              onClick={() => setBillingCycle('yearly')}
              className={`px-6 py-3 font-medium transition-colors ${
                billingCycle === 'yearly'
                  ? 'bg-black text-white'
                  : 'bg-white text-black hover:bg-gray-100'
              }`}
            >
              Yearly <span className="text-sm">(Save 20%)</span>
            </button>
          </div>
        </div>

        {isLoading && (
          <div className="text-center py-12">
            <p className="text-gray-600">Loading plans...</p>
          </div>
        )}

        {error && (
          <div className="text-center py-12">
            <p className="text-red-600">Failed to load plans</p>
          </div>
        )}

        {data?.tiers && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {data.tiers.map((tier) => {
              const isCurrentTier = tier.name === currentTier;
              const isProcessing = selectedTier === tier.name;

              return (
                <div
                  key={tier.name}
                  className={`border-2 rounded-lg p-6 transition-all ${
                    isCurrentTier
                      ? 'border-blue-600 bg-blue-50'
                      : 'border-gray-200 hover:border-gray-400'
                  }`}
                >
                  <div className="mb-4">
                    <h3 className="text-2xl font-bold capitalize mb-2">{tier.name}</h3>
                    {isCurrentTier && (
                      <span className="inline-block px-3 py-1 bg-blue-600 text-white text-sm font-medium rounded">
                        Current Plan
                      </span>
                    )}
                  </div>

                  {/* Pricing */}
                  <div className="mb-6">
                    <div className="text-3xl font-bold">
                      ${billingCycle === 'monthly' 
                        ? (tier.monthly_price_cents / 100).toFixed(0)
                        : (tier.yearly_price_cents / 100).toFixed(0)}
                    </div>
                    <div className="text-sm text-gray-600">
                      per {billingCycle === 'monthly' ? 'month' : 'year'}
                    </div>
                    {billingCycle === 'yearly' && tier.monthly_price_cents > 0 && (
                      <div className="text-xs text-green-600 mt-1">
                        Save ${(((tier.monthly_price_cents * 12) - tier.yearly_price_cents) / 100).toFixed(0)}/year
                      </div>
                    )}
                  </div>

                  <div className="space-y-3 mb-6 text-sm">
                    <div className="flex justify-between">
                      <span className="text-gray-600">API Requests</span>
                      <span className="font-medium">{formatNumber(tier.limits.bandwidth_ingress)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Data Transfer</span>
                      <span className="font-medium">{formatBytes(tier.limits.bandwidth_egress)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Team Seats</span>
                      <span className="font-medium">{tier.limits.team_seats}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Contracts</span>
                      <span className="font-medium">{tier.limits.contract_limit}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Rate Limit</span>
                      <span className="font-medium">{tier.limits.rate_limit} req/s</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Max Payload</span>
                      <span className="font-medium">{formatBytes(tier.limits.max_payload_bytes)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Hot Storage</span>
                      <span className="font-medium">{tier.limits.hot_storage_days} days</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Cold Storage</span>
                      <span className="font-medium">{tier.limits.cold_storage_days} days</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Support</span>
                      <span className="font-medium capitalize">{tier.limits.support}</span>
                    </div>
                    {tier.limits.overage_allowed && (
                      <div className="pt-2 border-t border-gray-200">
                        <p className="text-xs text-gray-600">
                          Overage: ${(tier.limits.bandwidth_ingress_overage_cents_per_million / 100).toFixed(2)}/1M requests,
                          ${(tier.limits.bandwidth_egress_overage_cents_per_gb / 100).toFixed(2)}/GB
                        </p>
                      </div>
                    )}
                  </div>

                  <button
                    onClick={() => handleSelectTier(tier.name)}
                    disabled={isCurrentTier || isProcessing}
                    className={`w-full py-3 font-medium transition-colors ${
                      isCurrentTier
                        ? 'bg-gray-300 text-gray-600 cursor-not-allowed'
                        : isProcessing
                        ? 'bg-gray-400 text-white cursor-wait'
                        : 'bg-black text-white hover:bg-gray-800'
                    }`}
                  >
                    {isCurrentTier
                      ? 'Current Plan'
                      : isProcessing
                      ? 'Processing...'
                      : 'Select Plan'}
                  </button>
                </div>
              );
            })}

            {/* Enterprise Tier - Hardcoded */}
            <div className="border-2 rounded-lg p-6 transition-all border-gray-200 hover:border-gray-400">
              <div className="mb-4">
                <h3 className="text-2xl font-bold capitalize mb-2">Enterprise</h3>
              </div>

              {/* Pricing */}
              <div className="mb-6">
                <div className="text-3xl font-bold">Custom</div>
                <div className="text-sm text-gray-600">Contact for pricing</div>
              </div>

              <div className="space-y-3 mb-6 text-sm">
                <div className="flex justify-between">
                  <span className="text-gray-600">API Requests</span>
                  <span className="font-medium">Unlimited</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Data Transfer</span>
                  <span className="font-medium">Unlimited</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Team Seats</span>
                  <span className="font-medium">Unlimited</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Contracts</span>
                  <span className="font-medium">Unlimited</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Rate Limit</span>
                  <span className="font-medium">Custom</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Max Payload</span>
                  <span className="font-medium">Custom</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Hot Storage</span>
                  <span className="font-medium">Custom</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Cold Storage</span>
                  <span className="font-medium">Custom</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Support</span>
                  <span className="font-medium">Dedicated</span>
                </div>
                <div className="pt-2 border-t border-gray-200">
                  <p className="text-xs text-gray-600">
                    Custom SLA, dedicated support, and tailored solutions
                  </p>
                </div>
              </div>

              <a
                href="mailto:sales@threadify.dev?subject=Enterprise Plan Inquiry"
                className="block w-full py-3 text-center font-medium bg-black text-white hover:bg-gray-800 transition-colors"
              >
                Contact Us
              </a>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
