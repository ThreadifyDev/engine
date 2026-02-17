import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams, Link } from '@remix-run/react';
import { api } from '~/lib/api';

export default function ResetPassword() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token');
  
  const [formData, setFormData] = useState({
    password: '',
    confirmPassword: '',
  });
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);
  const [loading, setLoading] = useState(false);
  const [tokenValid, setTokenValid] = useState(true);

  useEffect(() => {
    if (!token) {
      setTokenValid(false);
      setError('Invalid or missing reset token');
    }
  }, [token]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    // Validate passwords match
    if (formData.password !== formData.confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    // Validate password strength
    if (formData.password.length < 8) {
      setError('Password must be at least 8 characters long');
      return;
    }

    if (!token) {
      setError('Invalid reset token');
      return;
    }

    setLoading(true);

    try {
      await api.resetPassword({ token, password: formData.password });
      setSuccess(true);
      // Redirect to login after 3 seconds
      setTimeout(() => {
        navigate('/login');
      }, 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset password');
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  if (!tokenValid) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center px-4">
        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <h1 className="text-4xl font-bold text-black" style={{ fontFamily: 'Block, monospace' }}>
              Threadify
            </h1>
            <div className="mt-8 bg-gray-50 border-2 border-black p-6">
              <div className="w-16 h-16 bg-red-600 rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-3xl text-white">✕</span>
              </div>
              <h2 className="text-2xl font-bold text-black mb-2">Invalid Reset Link</h2>
              <p className="text-sm text-gray-600 mb-6">
                This password reset link is invalid or has expired.
              </p>
              <Link
                to="/auth/forgot-password"
                className="inline-block w-full bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 transition-colors text-center mb-3"
              >
                Request New Link
              </Link>
              <Link
                to="/login"
                className="inline-block w-full border-2 border-black text-black py-3 px-4 font-medium hover:bg-gray-50 transition-colors text-center"
              >
                Back to Sign In
              </Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (success) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center px-4">
        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <h1 className="text-4xl font-bold text-black" style={{ fontFamily: 'Block, monospace' }}>
              Threadify
            </h1>
            <div className="mt-8 bg-gray-50 border-2 border-black p-6">
              <div className="w-16 h-16 bg-black rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-3xl text-white">✓</span>
              </div>
              <h2 className="text-2xl font-bold text-black mb-2">Password Reset Successful</h2>
              <p className="text-sm text-gray-600 mb-6">
                Your password has been successfully reset. You can now sign in with your new password.
              </p>
              <p className="text-xs text-gray-500 mb-6">
                Redirecting to sign in...
              </p>
              <Link
                to="/login"
                className="inline-block w-full bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 transition-colors text-center"
              >
                Sign In Now
              </Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-white flex items-center justify-center px-4">
      <div className="max-w-md w-full space-y-8">
        {/* Header */}
        <div className="text-center">
          <h1 className="text-4xl font-bold text-black" style={{ fontFamily: 'Block, monospace' }}>
            Threadify
          </h1>
          <h2 className="mt-6 text-3xl font-bold text-black">Set new password</h2>
          <p className="mt-2 text-sm text-gray-600">
            Enter a new password for your account
          </p>
        </div>

        {/* Form */}
        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          {error && (
            <div className="bg-black text-white px-4 py-3 text-sm">
              {error}
            </div>
          )}

          <div className="space-y-4">
            {/* New Password */}
            <div>
              <label htmlFor="password" className="block text-sm font-medium text-black mb-1">
                New Password <span className="text-red-600">*</span>
              </label>
              <input
                id="password"
                name="password"
                type="password"
                required
                value={formData.password}
                onChange={handleChange}
                className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="Enter new password"
                minLength={8}
              />
              <p className="mt-1 text-xs text-gray-500">
                Must be at least 8 characters long
              </p>
            </div>

            {/* Confirm Password */}
            <div>
              <label htmlFor="confirmPassword" className="block text-sm font-medium text-black mb-1">
                Confirm Password <span className="text-red-600">*</span>
              </label>
              <input
                id="confirmPassword"
                name="confirmPassword"
                type="password"
                required
                value={formData.confirmPassword}
                onChange={handleChange}
                className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="Confirm new password"
                minLength={8}
              />
            </div>
          </div>

          {/* Submit Button */}
          <button
            type="submit"
            disabled={loading}
            className="w-full bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Resetting Password...' : 'Reset Password'}
          </button>

          {/* Back to Login */}
          <div className="text-center text-sm">
            <Link to="/login" className="text-black font-medium hover:underline">
              ← Back to Sign In
            </Link>
          </div>
        </form>
      </div>
    </div>
  );
}
