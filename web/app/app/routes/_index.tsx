import { useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate, Link } from '@remix-run/react';
import { api } from '~/lib/api';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify - Business Workflow Instrumentation" },
    { name: "description", content: "Monitor and validate your business workflows in real-time" },
  ];
};

export default function Index() {
  const navigate = useNavigate();

  useEffect(() => {
    // Redirect to dashboard if already authenticated
    if (api.isAuthenticated()) {
      navigate('/dashboard');
    }
  }, [navigate]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-white px-4">
      <div className="text-center max-w-2xl">
        <h1 className="text-6xl font-bold mb-4 text-black" style={{ fontFamily: 'Block, monospace' }}>
          Threadify
        </h1>
        <p className="text-xl text-gray-600 mb-8">
          Monitor and validate your business workflows in real-time
        </p>
        <div className="flex gap-4 justify-center">
          <Link
            to="/auth/login"
            className="px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors"
          >
            Login
          </Link>
          <Link
            to="/auth/signup"
            className="px-6 py-3 border-2 border-black text-black font-medium hover:bg-gray-100 transition-colors"
          >
            Sign Up
          </Link>
        </div>
      </div>
    </div>
  );
}
