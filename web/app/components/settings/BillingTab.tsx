import { useState, useEffect } from 'react';
import { GetCurrentPlanResponse } from '~/lib/api';

interface BillingTabProps {
  billingInfo: GetCurrentPlanResponse | null;
  loading: boolean;
  onTopUp: (amount: number) => void;
  onUpdateMonthlyLimit: (limit: string) => void;
}

export function BillingTab({ billingInfo, loading, onTopUp, onUpdateMonthlyLimit }: BillingTabProps) {
  const [topUpAmount, setTopUpAmount] = useState(10);
  const [customAmount, setCustomAmount] = useState('');
  const [showCustomInput, setShowCustomInput] = useState(false);
  const [maxMonthlyLimit, setMaxMonthlyLimit] = useState('');

  const creditAccount = billingInfo?.credit_account;

  // Initialize maxMonthlyLimit with current value when billingInfo loads
  useEffect(() => {
    if (creditAccount?.max_monthly_charge_millicents) {
      const currentLimit = (creditAccount.max_monthly_charge_millicents / 100000).toFixed(2);
      setMaxMonthlyLimit(currentLimit);
    }
  }, [creditAccount?.max_monthly_charge_millicents]);

  const formatMicroCurrency = (millicents: number | undefined | null) => {
    if (!millicents) return '0.00';
    const dollars = millicents / 100000;
    if (millicents % 1000 === 0) {
      return dollars.toFixed(2);
    }
    return dollars.toFixed(5).replace(/0+$/, '');
  };

  const handleTopUp = () => {
    const amount = showCustomInput ? parseFloat(customAmount) : topUpAmount;
    if (!isNaN(amount) && amount > 0) {
      onTopUp(amount);
    }
  };

  const handleUpdateLimit = () => {
    onUpdateMonthlyLimit(maxMonthlyLimit);
    setMaxMonthlyLimit('');
  };

  const formatBillingDate = (dateString: string): string => {
    const date = new Date(dateString);
    const day = date.getDate();
    const month = date.toLocaleDateString('en-US', { month: 'short' });
    const year = date.getFullYear();
    const suffix = (day: number) => {
      if (day > 3 && day < 21) return 'th';
      switch (day % 10) {
        case 1: return 'st';
        case 2: return 'nd';
        case 3: return 'rd';
        default: return 'th';
      }
    };
    return `${day}${suffix(day)} ${month} ${year}`;
  };

  const monthlyCharged = creditAccount?.monthly_charged_millicents || 0;
  const monthlyLimit = creditAccount?.max_monthly_charge_millicents || 0;
  const progressPercent = monthlyLimit > 0 ? Math.min(100, (monthlyCharged / monthlyLimit) * 100) : 0;
  const progressColor = monthlyLimit > 0 && monthlyCharged >= monthlyLimit
    ? 'bg-red-500'
    : monthlyLimit > 0 && (monthlyCharged / monthlyLimit) >= 0.8
    ? 'bg-yellow-500'
    : 'bg-blue-500';

  return (
    <div key="billing-tab" className="max-w-6xl">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column - Balance & Credits */}
        <div className="space-y-6">
          {/* Main Balance Section */}
          <div className="bg-white border border-gray-200 rounded-lg p-8">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h3 className="text-sm font-medium text-gray-500 mb-1">Current Balance</h3>
                <div className="text-4xl font-bold text-gray-900">
                  ${formatMicroCurrency(creditAccount?.balance_millicents)}
                </div>
              </div>
              <div className="text-right">
                <div className="text-sm text-gray-500 mb-1">Monthly Usage</div>
                <div className="text-2xl font-semibold text-gray-900">
                  ${formatMicroCurrency(creditAccount?.monthly_charged_millicents)}
                </div>
                <div className="text-xs text-gray-500">
                  of ${formatMicroCurrency(creditAccount?.max_monthly_charge_millicents)} limit
                </div>
              </div>
            </div>

            {/* Usage Progress Bar */}
            <div className="mb-8">
              <div className="w-full bg-gray-100 rounded-full h-2">
                <div 
                  className={`h-2 rounded-full transition-all ${progressColor}`}
                  style={{ width: `${progressPercent}%` }}
                ></div>
              </div>
            </div>

            {/* Add Credits Section */}
            <div className="border-t border-gray-100 pt-6">
              <h4 className="text-sm font-medium text-gray-900 mb-4">Add Credits</h4>
              
              {/* Preset amounts */}
              <div className="flex items-center gap-3 mb-4">
                {[10, 25, 50, 100].map((amt) => (
                  <button
                    key={amt}
                    onClick={() => {
                      setTopUpAmount(amt);
                      setCustomAmount('');
                      setShowCustomInput(false);
                    }}
                    className={`px-4 py-2 text-sm font-medium rounded-md transition-colors ${
                      !showCustomInput && topUpAmount === amt
                        ? 'bg-gray-900 text-white'
                        : 'bg-gray-50 text-gray-700 hover:bg-gray-100'
                    }`}
                  >
                    ${amt}
                  </button>
                ))}
                <button
                  onClick={() => {
                    setShowCustomInput(true);
                    setCustomAmount('');
                  }}
                  className={`px-4 py-2 text-sm font-medium rounded-md transition-colors ${
                    showCustomInput
                      ? 'bg-gray-900 text-white'
                      : 'bg-gray-50 text-gray-700 hover:bg-gray-100'
                  }`}
                >
                  Custom
                </button>
              </div>

              {/* Custom amount input */}
              {showCustomInput && (
                <div className="mb-4">
                  <div className="relative">
                    <span className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 text-sm">$</span>
                    <input
                      type="number"
                      placeholder="Enter amount"
                      value={customAmount}
                      onChange={(e) => setCustomAmount(e.target.value)}
                      className="w-full pl-8 pr-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                      autoFocus
                    />
                  </div>
                </div>
              )}

              <button 
                onClick={handleTopUp}
                disabled={loading}
                className="w-full px-6 py-2.5 bg-gray-900 text-white rounded-md hover:bg-gray-800 transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? 'Processing...' : `Add $${customAmount || topUpAmount}`}
              </button>
            </div>
          </div>
        </div>

        {/* Right Column - Billing Details */}
        <div className="space-y-6">
          {/* Billing Cycle */}
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <div className="text-sm text-gray-500 mb-1">Billing Cycle</div>
            <div className="text-lg font-semibold text-gray-900">
              {creditAccount?.billing_cycle_start
                ? `Started ${formatBillingDate(creditAccount.billing_cycle_start)}`
                : 'No active cycle'}
            </div>
          </div>

          {/* Auto-Topup Status */}
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <h4 className="text-sm font-medium text-gray-900 mb-4">Auto-Topup Status</h4>
            <div className="space-y-3">
              {creditAccount?.auto_topup_millicents && creditAccount.auto_topup_millicents > 0 ? (
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-sm font-medium text-gray-900">Enabled</div>
                    <div className="text-xs text-gray-500">
                      ${formatMicroCurrency(creditAccount.auto_topup_millicents)} when balance is low
                    </div>
                  </div>
                  <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                    Active
                  </span>
                </div>
              ) : (
                <div className="flex items-center justify-between">
                  <div className="text-sm text-gray-500">Not configured</div>
                  <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                    Inactive
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* Monthly Spending Limit */}
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <h4 className="text-sm font-medium text-gray-900 mb-4">Monthly Spending Limit</h4>
            <div className="space-y-3">
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 text-sm">$</span>
                <input
                  type="number"
                  value={maxMonthlyLimit}
                  onChange={(e) => setMaxMonthlyLimit(e.target.value)}
                  placeholder="Enter limit"
                  className="w-full pl-8 pr-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                />
              </div>
              <p className="text-xs text-gray-500">
                Set a maximum monthly budget for Threadify usage
              </p>
              <button
                onClick={handleUpdateLimit}
                disabled={loading}
                className="w-full px-4 py-2 bg-gray-900 text-white rounded-md hover:bg-gray-800 transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? 'Saving...' : 'Save Limit'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
