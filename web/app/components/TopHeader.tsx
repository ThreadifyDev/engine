import { useNavigate } from '@remix-run/react';
import { Wallet } from 'lucide-react';
import { useCurrentPlan } from '~/hooks/useBilling';

export default function TopHeader() {
  const navigate = useNavigate();
  const { data: billingData } = useCurrentPlan();

  const formatBalance = (millicents: number) => {
    const dollars = millicents / 100000;
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(dollars);
  };

  const balance = billingData?.credit_account?.balance_millicents ?? 0;
  const minBalance = billingData?.credit_account?.min_balance_millicents ?? 0;
  const isLow = balance > 0 && balance < minBalance;

  return (
    <div className="sticky top-0 z-30 bg-white/80 backdrop-blur-sm border-b border-gray-100">
      <div className="flex items-center justify-end px-6 py-3">
        <button
          onClick={() => navigate('/u/settings?tab=billing')}
          className={`flex items-center gap-2 px-3 py-1.5 rounded-lg transition-colors hover:bg-gray-50 ${
            isLow ? 'text-yellow-600' : 'text-gray-700'
          }`}
          title="View billing"
        >
          <Wallet className="w-4 h-4" />
          <span className="text-sm font-medium">
            {billingData?.credit_account
              ? formatBalance(balance)
              : '--'}
          </span>
        </button>
      </div>
    </div>
  );
}
