import { useState } from 'react';
import { useNavigate, Link } from '@remix-run/react';
import { api, type LoginData, ValidationError } from '~/lib/api';
import Alert, { type AlertType } from '~/components/Alert';

export default function Login() {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<LoginData>({
    email: '',
    password: '',
  });
  const [alert, setAlert] = useState<{ type: AlertType; message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAlert(null);
    setLoading(true);

    try {
      const response = await api.login(formData);

      if (response.email_verification_required) {
        // User hasn't verified email — send them through the verification flow
        navigate(`/auth/verify-otp?email=${encodeURIComponent(formData.email)}&type=signup`);
      } else {
        // Normal login — send them through the OTP flow
        navigate(`/auth/verify-otp?email=${encodeURIComponent(formData.email)}&type=login`);
      }
    } catch (err) {
      if (err instanceof ValidationError) {
        setAlert({
          type: 'error',
          message: err.message,
          details: err.details,
        });
      } else {
        setAlert({
          type: 'error',
          message: err instanceof Error ? err.message : 'Login failed',
        });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  return (
    <div className="min-h-screen bg-white flex items-center justify-center px-4">
      <div className="max-w-md w-full space-y-8">
        {/* Header */}
        <div className="text-center">
          <h1 className="text-4xl font-bold text-black" style={{ fontFamily: 'Block, monospace' }}>
            Threadify
          </h1>
          <h2 className="mt-6 text-3xl font-bold text-black">Welcome back</h2>
          <p className="mt-2 text-sm text-gray-600">
            Sign in to your account
          </p>
        </div>

        {/* Form */}
        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          {alert && <Alert type={alert.type} message={alert.message} details={alert.details} />}

          <div className="space-y-4">
            {/* Email */}
            <div>
              <label htmlFor="email" className="block text-sm font-medium text-black mb-1">
                Email Address <span className="text-red-600">*</span>
              </label>
              <input
                id="email"
                name="email"
                type="email"
                required
                value={formData.email}
                onChange={handleChange}
                className="w-full px-4 py-3 border-2 rounded-lg border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="you@company.com"
              />
            </div>

            {/* Password */}
            <div>
              <div className="flex justify-between items-center mb-1">
                <label htmlFor="password" className="block text-sm font-medium text-black">
                  Password <span className="text-red-600">*</span>
                </label>
                <Link to="/auth/forgot-password" className="text-xs text-black hover:underline">
                  Forgot password?
                </Link>
              </div>
              <input
                id="password"
                name="password"
                type="password"
                required
                value={formData.password}
                onChange={handleChange}
                className="w-full px-4 py-3 border-2 rounded-lg border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="Enter your password"
              />
            </div>
          </div>

          {/* Submit Button */}
          <button
            type="submit"
            disabled={loading}
            className="w-full bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Signing in...' : 'Sign In'}
          </button>

          {/* Signup Link */}
          <div className="text-center text-sm">
            <span className="text-gray-600">Don't have an account? </span>
            <Link to="/signup" className="text-black font-medium hover:underline">
              Create account
            </Link>
          </div>
        </form>
      </div>
    </div>
  );
}
