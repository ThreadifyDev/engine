import { useNavigate, useLocation } from '@remix-run/react';
import ThreadifyLogo from '~/components/ThreadifyLogo';
import { useState } from 'react';
import {
  ChevronLeft,
  ChevronRight,
  LogOut,
  LayoutDashboard,
  GitBranch,
  FileText,
  Key,
  Bot,
  Users,
  Settings,
  Sparkles,
  UserCircle,
  Wallet
} from 'lucide-react';
import { api } from '~/lib/api';
import { useCurrentPlan } from '~/hooks/useBilling';

interface SideNavProps {
  isCollapsed?: boolean;
  onToggle?: () => void;
  isMobileOpen?: boolean;
  onCloseMobile?: () => void;
}

export default function SideNav({ isCollapsed: controlledCollapsed, onToggle, isMobileOpen = false, onCloseMobile }: SideNavProps = {}) {
  const navigate = useNavigate();
  const location = useLocation();
  const [internalCollapsed, setInternalCollapsed] = useState(true);
  const { data: billingData } = useCurrentPlan();

  // Use controlled state if provided, otherwise use internal state
  const isCollapsed = controlledCollapsed !== undefined ? controlledCollapsed : internalCollapsed;
  const handleToggle = onToggle || (() => setInternalCollapsed(!internalCollapsed));

  const formatBalance = (millicents: number) => {
    const dollars = millicents / 100000;
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(dollars);
  };

  const handleLogout = async () => {
    await api.logout();
    navigate('/login');
  };

  const isActive = (path: string) => {
    // Exact match or starts with the path (for nested routes)
    return location.pathname === path || location.pathname.startsWith(path + '/');
  };

  const navItems = [
    { path: '/u/dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { path: '/u/threads', label: 'Threads', icon: GitBranch },
    { path: '/u/assistant', label: 'AI Assistant', icon: Sparkles },
    { path: '/u/contracts', label: 'Contracts', icon: FileText },
    { path: '/u/profiles', label: 'Entity Profiles', icon: UserCircle },
    { path: '/u/developer', label: 'Developer', icon: Key },
    { path: '/u/team', label: 'Team', icon: Users },
    { path: '/u/settings', label: 'Settings', icon: Settings },
  ];

  return (
    <div
      className={`h-screen bg-black flex-col fixed left-0 top-0 transition-all duration-300 z-50 ${isCollapsed ? 'w-16' : 'w-64'} ${isMobileOpen ? 'translate-x-0 flex' : '-translate-x-full lg:translate-x-0 lg:flex'} `}
    >
      {/* Logo & Toggle */}
      <div className="p-4 border-b border-gray-800 flex items-center justify-between">
        {!isCollapsed && (
          <div
            className="cursor-pointer text-white"
            onClick={() => navigate('/u/dashboard')}
          >
            <ThreadifyLogo height={24} />
          </div>
        )}
        <div className="flex gap-2">
          {/* Close button on mobile instead of expand/collapse */}
          {isMobileOpen ? (
            <button
              onClick={onCloseMobile}
              className="p-2 hover:bg-gray-800 rounded transition-colors text-white lg:hidden"
              title="Close menu"
            >
              <ChevronLeft className="w-5 h-5" />
            </button>
          ) : null}
          <button
            onClick={handleToggle}
            className="p-2 hover:bg-gray-800 rounded transition-colors text-white hidden lg:block"
            title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {isCollapsed ? <ChevronRight className="w-5 h-5" /> : <ChevronLeft className="w-5 h-5" />}
          </button>
        </div>
      </div>

      {/* Wallet Balance */}
      <div className="px-4 py-3 border-b border-gray-800">
        <button
          onClick={() => navigate('/u/settings?tab=billing')}
          className={`w-full flex items-center gap-3 transition-colors ${
            isCollapsed ? 'justify-center' : 'justify-start'
          }`}
          title={isCollapsed ? 'Wallet Balance' : undefined}
        >
          <Wallet className="w-5 h-5 flex-shrink-0 text-gray-300" />
          {(!isCollapsed || isMobileOpen) && (
            <div className="flex flex-col items-start overflow-hidden">
              <span className="text-xs text-gray-500">Balance</span>
              {billingData?.credit_account ? (
                <span
                  className={`text-sm font-semibold ${
                    billingData.credit_account.balance_millicents < billingData.credit_account.min_balance_millicents
                      ? 'text-yellow-400'
                      : 'text-white'
                  }`}
                >
                  {formatBalance(billingData.credit_account.balance_millicents)}
                </span>
              ) : (
                <span className="text-sm text-gray-400">--</span>
              )}
            </div>
          )}
        </button>
      </div>

      {/* Navigation Items */}
      <nav className="flex-1 py-4">
        {navItems.map((item) => (
          <button
            key={item.path}
            onClick={() => navigate(item.path)}
            className={`w-full px-4 py-3 text-left text-sm font-medium transition-all flex items-center gap-3 ${isActive(item.path)
                ? 'bg-white text-black'
                : 'text-gray-300 hover:bg-gray-900 hover:text-white'
              }`}
            title={isCollapsed ? item.label : undefined}
          >
            <item.icon className="w-5 h-5 flex-shrink-0" />
            {(!isCollapsed || isMobileOpen) && <span className="truncate">{item.label}</span>}
          </button>
        ))}
      </nav>

      {/* Logout Button */}
      <div className="p-4 border-t border-gray-800">
        <button
          onClick={handleLogout}
          className={`w-full px-4 py-2 text-sm bg-white text-black hover:bg-gray-200 transition-colors font-medium rounded flex items-center gap-2 ${isCollapsed && !isMobileOpen ? 'justify-center' : 'justify-start'
            }`}
          title={isCollapsed && !isMobileOpen ? 'Logout' : undefined}
        >
          <LogOut className="w-4 h-4 flex-shrink-0" />
          {(!isCollapsed || isMobileOpen) && <span>Logout</span>}
        </button>
      </div>
    </div>
  );
}
