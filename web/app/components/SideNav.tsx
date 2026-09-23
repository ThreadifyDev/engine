import { useNavigate, useLocation } from 'react-router';
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
  Users,
  Settings,
  UserCircle
} from 'lucide-react';
import { api } from '~/lib/api';

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

  // Use controlled state if provided, otherwise use internal state
  const isCollapsed = controlledCollapsed !== undefined ? controlledCollapsed : internalCollapsed;
  const compact = isCollapsed && !isMobileOpen;
  const handleToggle = onToggle || (() => setInternalCollapsed(!internalCollapsed));

  const handleLogout = async () => {
    await api.logout();
  };

  const isActive = (path: string) => {
    // Exact match or starts with the path (for nested routes)
    return location.pathname === path || location.pathname.startsWith(path + '/');
  };

  const navItems = [
    { path: '/u/dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { path: '/u/threads', label: 'Threads', icon: GitBranch },
    { path: '/u/profiles', label: 'Entity Profiles', icon: UserCircle },
    { path: '/u/contracts', label: 'Contracts', icon: FileText },
    { path: '/u/developer', label: 'Developer', icon: Key },
    { path: '/u/team', label: 'Team', icon: Users },
    { path: '/u/settings', label: 'Settings', icon: Settings },
  ];

  return (
    <div
      className={`h-screen bg-black flex-col fixed left-0 top-0 transition-all duration-300 z-[90] ${isMobileOpen ? 'w-64' : isCollapsed ? 'w-16' : 'w-64'} ${isMobileOpen ? 'translate-x-0 flex visible' : '-translate-x-full invisible lg:visible lg:translate-x-0 lg:flex'} `}
    >
      {/* Logo & Toggle */}
      <div className={`h-14 border-b border-gray-800 flex items-center ${compact ? 'justify-center' : 'justify-between px-4'}`}>
        {!compact && (
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

      {/* Navigation Items */}
      <nav className="flex-1 py-4">
        {navItems.map((item) => (
          <button
            key={item.path}
            onClick={() => { navigate(item.path); onCloseMobile?.(); }}
            className={`w-full py-3 text-left text-sm font-medium transition-all flex items-center gap-3 ${compact ? 'justify-center px-0' : 'px-4'} ${isActive(item.path)
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
          className={`w-full py-2 text-sm bg-white text-black hover:bg-gray-200 transition-colors font-medium rounded flex items-center gap-2 ${compact ? 'justify-center px-0' : 'justify-start px-4'
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
