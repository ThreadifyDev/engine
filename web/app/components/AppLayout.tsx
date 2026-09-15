import { ReactNode, useState } from 'react';
import { useNavigate } from '@remix-run/react';
import { Wallet } from 'lucide-react';
import SideNav from './SideNav';
import TopHeader from './TopHeader';
import { useCurrentPlan } from '~/hooks/useBilling';

interface AppLayoutProps {
  children: ReactNode;
  showRightSidebar?: boolean;
  rightSidebarContent?: ReactNode;
  rightSidebarWidth?: string;
}

export default function AppLayout({
  children,
  showRightSidebar = false,
  rightSidebarContent,
  rightSidebarWidth = '400px'
}: AppLayoutProps) {
  const navigate = useNavigate();
  const [isNavCollapsed, setIsNavCollapsed] = useState(true);
  const [isMobileOpen, setIsMobileOpen] = useState(false);
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

  // navWidth only applies to desktop (lg and up)
  const navWidth = isNavCollapsed ? 64 : 256; 

  return (
    <div className="min-h-screen bg-white flex">
      {/* Mobile Nav Overlay */}
      {isMobileOpen && (
        <div 
          className="fixed inset-0 bg-black/50 z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
        />
      )}

      {/* Left Navigation */}
      <SideNav 
        isCollapsed={isNavCollapsed} 
        onToggle={() => setIsNavCollapsed(!isNavCollapsed)} 
        isMobileOpen={isMobileOpen}
        onCloseMobile={() => setIsMobileOpen(false)}
      />
      
      {/* Main Content Area */}
      <main 
        className="flex-1 transition-all duration-300 overflow-auto w-full"
        style={{
          // Use CSS variables or calc to handle responsive margin
        }}
      >
        {/* Mobile Header */}
        <div className="lg:hidden flex items-center justify-between p-4 border-b border-gray-200">
          <div className="flex items-center">
            <button
              onClick={() => setIsMobileOpen(true)}
              className="p-2 hover:bg-gray-100 rounded text-black transition-colors mr-3"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="3" y1="12" x2="21" y2="12"></line><line x1="3" y1="6" x2="21" y2="6"></line><line x1="3" y1="18" x2="21" y2="18"></line></svg>
            </button>
            <span className="font-semibold">Threadify</span>
          </div>
          <button
            onClick={() => navigate('/u/settings?tab=billing')}
            className={`flex items-center gap-1.5 px-2 py-1 rounded-lg transition-colors hover:bg-gray-50 ${
              isLow ? 'text-yellow-600' : 'text-gray-700'
            }`}
          >
            <Wallet className="w-4 h-4" />
            <span className="text-sm font-medium">
              {billingData?.billing_source === 'registry' ? 'Plan' : billingData?.credit_account ? formatBalance(balance) : '--'}
            </span>
          </button>
        </div>

        <div className="lg:ml-auto transition-all duration-300" style={{ marginLeft: `var(--desktop-margin, 0px)` }}>
          <style>{`
            @media (min-width: 1024px) {
              :root {
                --desktop-margin: ${navWidth}px;
              }
            }
          `}</style>
          <div className="hidden lg:block">
            <TopHeader />
          </div>
          {children}
        </div>
      </main>

      {/* Right Sidebar (Optional) */}
      {showRightSidebar && (
        <aside 
          className="fixed right-0 top-0 h-screen bg-white border-l border-gray-200 overflow-y-auto transition-all duration-300 z-10"
          style={{ width: rightSidebarWidth }}
        >
          {rightSidebarContent}
        </aside>
      )}
    </div>
  );
}
