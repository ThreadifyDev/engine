import { useNavigate, useLocation } from '@remix-run/react';
import { api } from '~/lib/api';

export default function SideNav() {
  const navigate = useNavigate();
  const location = useLocation();

  const handleLogout = () => {
    api.logout();
    navigate('/auth/login');
  };

  const isActive = (path: string) => {
    // Exact match or starts with the path (for nested routes)
    return location.pathname === path || location.pathname.startsWith(path + '/');
  };

  const navItems = [
    { path: '/dashboard', label: 'Dashboard' },
    { path: '/contracts', label: 'Contracts' },
    { path: '/api-keys', label: 'API Keys' },
    { path: '/service-accounts', label: 'Service Accounts' },
    { path: '/team', label: 'Team' },
    { path: '/settings', label: 'Settings' },
  ];

  return (
    <div className="hidden lg:flex w-64 h-screen bg-black flex-col fixed left-0 top-0">
      {/* Logo */}
      <div className="p-6 border-b border-gray-800">
        <h1 
          className="text-xl font-bold cursor-pointer text-white" 
          style={{ fontFamily: 'Block, sans-serif' }}
          onClick={() => navigate('/dashboard')}
        >
          Threadify
        </h1>
      </div>

      {/* Navigation Items */}
      <nav className="flex-1 py-4">
        {navItems.map((item) => (
          <button
            key={item.path}
            onClick={() => navigate(item.path)}
            className={`w-full px-6 py-3 text-left text-sm font-medium transition-all ${
              isActive(item.path)
                ? 'bg-white text-black'
                : 'text-gray-300 hover:bg-gray-900 hover:text-white'
            }`}
          >
            {item.label}
          </button>
        ))}
      </nav>

      {/* Logout Button */}
      <div className="p-4 border-t border-gray-800">
        <button
          onClick={handleLogout}
          className="w-full px-4 py-2 text-sm bg-white text-black hover:bg-gray-200 transition-colors font-medium rounded"
        >
          Logout
        </button>
      </div>
    </div>
  );
}
