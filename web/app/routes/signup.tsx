import { useState } from 'react';
import { useNavigate, Link } from '@remix-run/react';
import { api, type SignupData, ValidationError } from '~/lib/api';
import Alert, { type AlertType } from '~/components/Alert';

export default function Signup() {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<SignupData>({
    company_name: '',
    email: '',
    password: '',
    full_name: '',
    job_role: '',
    industry: undefined,
    company_size: undefined,
    use_case: undefined,
  });
  const [alert, setAlert] = useState<{ type: AlertType; message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAlert(null);
    setLoading(true);

    try {
      await api.signup(formData);
      // Navigate to OTP verification with email
      navigate(`/auth/verify-otp?email=${encodeURIComponent(formData.email)}`);
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
          message: err instanceof Error ? err.message : 'Signup failed',
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
          <h2 className="mt-6 text-3xl font-bold text-black">Create your account</h2>
          <p className="mt-2 text-sm text-gray-600">
            Turn customer requests into intelligence
          </p>
        </div>

        {/* Form */}
        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          {alert && <Alert type={alert.type} message={alert.message} details={alert.details} />}

          <div className="space-y-4">
            {/* Company Name */}
            <div>
              <label htmlFor="company_name" className="block text-sm font-medium text-black mb-1">
                Company Name <span className="text-red-600">*</span>
              </label>
              <input
                id="company_name"
                name="company_name"
                type="text"
                required
                minLength={2}
                value={formData.company_name}
                onChange={handleChange}
                className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="Acme Corp"
              />
            </div>

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
                className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="you@company.com"
              />
            </div>

            {/* Password */}
            <div>
              <label htmlFor="password" className="block text-sm font-medium text-black mb-1">
                Password <span className="text-red-600">*</span>
              </label>
              <input
                id="password"
                name="password"
                type="password"
                required
                minLength={8}
                value={formData.password}
                onChange={handleChange}
                className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                placeholder="Min. 8 characters"
              />
              <p className="mt-1 text-xs text-gray-500">Must be at least 8 characters</p>
            </div>

          </div>

          {/* Submit Button */}
          <button
            type="submit"
            disabled={loading}
            className="w-full bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Creating account...' : 'Create Account'}
          </button>

          {/* Login Link */}
          <div className="text-center text-sm">
            <span className="text-gray-600">Already have an account? </span>
            <Link to="/login" className="text-black font-medium hover:underline">
              Sign in
            </Link>
          </div>
        </form>
      </div>
    </div>
  );
}
