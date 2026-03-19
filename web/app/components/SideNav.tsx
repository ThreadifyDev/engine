import { useNavigate, useLocation } from '@remix-run/react';
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
  Sparkles
} from 'lucide-react';
import { api } from '~/lib/api';

interface SideNavProps {
  isCollapsed?: boolean;
  onToggle?: () => void;
}

export default function SideNav({ isCollapsed: controlledCollapsed, onToggle }: SideNavProps = {}) {
  const navigate = useNavigate();
  const location = useLocation();
  const [internalCollapsed, setInternalCollapsed] = useState(true);

  // Use controlled state if provided, otherwise use internal state
  const isCollapsed = controlledCollapsed !== undefined ? controlledCollapsed : internalCollapsed;
  const handleToggle = onToggle || (() => setInternalCollapsed(!internalCollapsed));

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
    { path: '/u/contracts', label: 'Contracts', icon: FileText },
    { path: '/u/assistant', label: 'AI Assistant', icon: Sparkles },
    { path: '/u/api-keys', label: 'API Keys', icon: Key },
    { path: '/u/service-accounts', label: 'Service Accounts', icon: Bot },
    { path: '/u/team', label: 'Team', icon: Users },
    { path: '/u/settings', label: 'Settings', icon: Settings },
  ];

  return (
    <div
      className={`hidden lg:flex h-screen bg-black flex-col fixed left-0 top-0 transition-all duration-300 ${isCollapsed ? 'w-16' : 'w-64'
        }`}
    >
      {/* Logo & Toggle */}
      <div className="p-4 border-b border-gray-800 flex items-center justify-between">
        {!isCollapsed && (
          <h1
            className="text-xl font-bold cursor-pointer text-white"
            style={{ fontFamily: 'Block, sans-serif' }}
            onClick={() => navigate('/u/dashboard')}
          >
            Threadify
          </h1>
        )}
        <button
          onClick={handleToggle}
          className="p-2 hover:bg-gray-800 rounded transition-colors text-white"
          title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {isCollapsed ? <ChevronRight className="w-5 h-5" /> : <ChevronLeft className="w-5 h-5" />}
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
            <item.icon className="w-5 h-5" />
            {!isCollapsed && <span>{item.label}</span>}
          </button>
        ))}
      </nav>

      {/* Logout Button */}
      <div className="p-4 border-t border-gray-800">
        <button
          onClick={handleLogout}
          className={`w-full px-4 py-2 text-sm bg-white text-black hover:bg-gray-200 transition-colors font-medium rounded flex items-center gap-2 ${isCollapsed ? 'justify-center' : 'justify-start'
            }`}
          title={isCollapsed ? 'Logout' : undefined}
        >
          <LogOut className="w-4 h-4" />
          {!isCollapsed && <span>Logout</span>}
        </button>
      </div>
    </div>
  );
}
