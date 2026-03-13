import { useState } from 'react';
import { useCurrentPlan, useCancelSubscription } from '~/hooks/useBilling';
import { format } from 'date-fns';

interface BillingCardProps {
  onUpgrade: () => void;
}

export default function BillingCard({ onUpgrade }: BillingCardProps) {
  const { data, isLoading, error } = useCurrentPlan();
  const cancelMutation = useCancelSubscription();
  const [showCancelConfirm, setShowCancelConfirm] = useState(false);

  if (isLoading) {
    return (
      <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm animate-pulse">
        <div className="h-8 bg-gray-200 rounded w-1/3 mb-4"></div>
        <div className="space-y-3">
          <div className="h-4 bg-gray-200 rounded w-full"></div>
          <div className="h-4 bg-gray-200 rounded w-2/3"></div>
          <div className="h-4 bg-gray-200 rounded w-1/2"></div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="border border-red-200 rounded-lg p-6 bg-red-50">
        <p className="text-red-600">Failed to load billing information</p>
      </div>
    );
  }

  const { plan, usage_meter } = data || {};

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
    return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
  };

  const formatNumber = (num: number) => {
    return new Intl.NumberFormat().format(num);
  };

  const calculatePercentage = (used: number, total: number) => {
    if (total === 0) return 0;
    const remaining = Math.max(0, used);
    return Math.min(100, (remaining / total) * 100);
  };

  const getStatusColor = (status?: string) => {
    switch (status) {
      case 'active':
        return 'bg-green-600';
      case 'cancelled':
        return 'bg-red-600';
      case 'suspended':
        return 'bg-yellow-600';
      default:
        return 'bg-gray-600';
    }
  };

  const handleCancelSubscription = async () => {
    try {
      await cancelMutation.mutateAsync();
      setShowCancelConfirm(false);
      alert('Subscription cancelled successfully');
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to cancel subscription');
    }
  };

  if (!plan) {
    return (
      <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
        <h3 className="text-xl font-bold mb-4">No Active Plan</h3>
        <p className="text-gray-600 mb-6">
          You're currently on the free tier. Upgrade to unlock more features and higher limits.
        </p>
        <button
          onClick={onUpgrade}
          className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium"
        >
          Upgrade Plan
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Current Plan Card */}
      <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
        <div className="flex justify-between items-start mb-6">
          <div>
            <h3 className="text-xl font-bold mb-2">Current Plan</h3>
            <div className="flex items-center gap-3">
              <span className="text-2xl font-bold capitalize">{plan.subscription_tier}</span>
              <span className={`px-3 py-1 text-white text-sm font-medium ${getStatusColor(plan.status)}`}>
                {plan.status}
              </span>
            </div>
            <p className="text-gray-600 mt-1 capitalize">
              Billed {plan.billing_cycle}
            </p>
          </div>
          <div className="text-right">
            <p className="text-sm text-gray-600">Next billing date</p>
            <p className="font-medium">{format(new Date(plan.billing_end), 'MMM dd, yyyy')}</p>
          </div>
        </div>

        <div className="flex gap-3">
          <button
            onClick={onUpgrade}
            className="px-6 py-3 border-2 border-black hover:bg-black hover:text-white transition-colors font-medium"
          >
            Change Plan
          </button>
          {plan.status === 'active' && (
            <button
              onClick={() => setShowCancelConfirm(true)}
              className="px-6 py-3 border-2 border-red-600 text-red-600 hover:bg-red-600 hover:text-white transition-colors font-medium"
            >
              Cancel Subscription
            </button>
          )}
        </div>
      </div>

      {/* Usage Metrics Card */}
      {usage_meter && (
        <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
          <h3 className="text-xl font-bold mb-6">Usage & Limits</h3>

          <div className="space-y-6">
            {/* Bandwidth Ingress */}
            <div>
              <div className="flex justify-between mb-2">
                <span className="font-medium">API Requests</span>
                <span className="text-sm text-gray-600">
                  {formatNumber(Math.max(0, usage_meter.bandwidth_ingress_balance))} / {formatNumber(usage_meter.max_bandwidth_ingress)} remaining
                </span>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-3">
                <div
                  className="bg-blue-600 h-3 rounded-full transition-all"
                  style={{
                    width: `${calculatePercentage(usage_meter.bandwidth_ingress_balance, usage_meter.max_bandwidth_ingress)}%`,
                  }}
                ></div>
              </div>
              {usage_meter.bandwidth_ingress_balance < 0 && (
                <p className="text-sm text-red-600 mt-1">
                  Overage: {formatNumber(Math.abs(usage_meter.bandwidth_ingress_balance))} requests
                </p>
              )}
            </div>

            {/* Bandwidth Egress */}
            <div>
              <div className="flex justify-between mb-2">
                <span className="font-medium">Data Transfer</span>
                <span className="text-sm text-gray-600">
                  {formatBytes(Math.max(0, usage_meter.bandwidth_egress_balance))} / {formatBytes(usage_meter.max_bandwidth_egress)} remaining
                </span>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-3">
                <div
                  className="bg-green-600 h-3 rounded-full transition-all"
                  style={{
                    width: `${calculatePercentage(usage_meter.bandwidth_egress_balance, usage_meter.max_bandwidth_egress)}%`,
                  }}
                ></div>
              </div>
              {usage_meter.bandwidth_egress_balance < 0 && (
                <p className="text-sm text-red-600 mt-1">
                  Overage: {formatBytes(Math.abs(usage_meter.bandwidth_egress_balance))}
                </p>
              )}
            </div>

            {/* Other Limits */}
            <div className="grid grid-cols-2 gap-4 pt-4 border-t border-gray-200">
              <div>
                <p className="text-sm text-gray-600">Team Seats</p>
                <p className="font-medium">{usage_meter.max_team_seats}</p>
              </div>
              <div>
                <p className="text-sm text-gray-600">Contracts</p>
                <p className="font-medium">{usage_meter.max_contract_limit}</p>
              </div>
              <div>
                <p className="text-sm text-gray-600">Rate Limit</p>
                <p className="font-medium">{usage_meter.max_rate_limit} req/s</p>
              </div>
              <div>
                <p className="text-sm text-gray-600">Max Payload</p>
                <p className="font-medium">{formatBytes(usage_meter.max_payload_bytes)}</p>
              </div>
              <div>
                <p className="text-sm text-gray-600">Hot Storage</p>
                <p className="font-medium">{usage_meter.hot_storage_days} days</p>
              </div>
              <div>
                <p className="text-sm text-gray-600">Cold Storage</p>
                <p className="font-medium">{usage_meter.cold_storage_days} days</p>
              </div>
              <div>
                <p className="text-sm text-gray-600">Support</p>
                <p className="font-medium capitalize">{usage_meter.support}</p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Cancel Confirmation Modal */}
      {showCancelConfirm && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-8 max-w-md w-full mx-4">
            <h3 className="text-2xl font-bold mb-4">Cancel Subscription?</h3>
            <p className="text-gray-600 mb-6">
              Are you sure you want to cancel your subscription? You'll lose access to premium features at the end of your billing period.
            </p>
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => setShowCancelConfirm(false)}
                disabled={cancelMutation.isPending}
                className="px-6 py-3 border-2 border-gray-300 hover:bg-gray-100 transition-colors font-medium"
              >
                Keep Subscription
              </button>
              <button
                onClick={handleCancelSubscription}
                disabled={cancelMutation.isPending}
                className="px-6 py-3 bg-red-600 text-white hover:bg-red-700 transition-colors font-medium disabled:opacity-50"
              >
                {cancelMutation.isPending ? 'Cancelling...' : 'Yes, Cancel'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
