import { useNavigate } from '@remix-run/react';

export default function Nav() {
  const navigate = useNavigate();

  return (
    <nav className="bg-white sticky top-0 z-50 border-b border-gray-200">
      <div className="max-w-7xl mx-auto px-6 py-4 flex items-center justify-between">
        {/* Logo */}
        <div className="text-xl font-semibold text-black cursor-pointer" onClick={() => navigate('/')}>
          Threadify
        </div>

        {/* Links */}
        <div className="flex items-center gap-8">
          <a href="#" className="text-gray-600 hover:text-black transition text-sm">
            Docs
          </a>
          
          <div className="flex gap-3">
            <button
              onClick={() => navigate('/login')}
              className="px-4 py-2 text-sm text-gray-600 hover:text-black transition"
            >
              Login
            </button>
            <button
              onClick={() => navigate('/signup')}
              className="px-4 py-2 text-sm bg-black text-white font-medium rounded hover:bg-gray-800 transition"
            >
              Get Started
            </button>
          </div>
        </div>
      </div>
    </nav>
  );
}
