import { useState, useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate, Link, useSearchParams } from '@remix-run/react';
import { api, type SignupData, ValidationError } from '~/lib/api';
import Alert, { type AlertType } from '~/components/Alert';
import { Eye, EyeOff } from 'lucide-react';

export const meta: MetaFunction = () => {
  return [
    { title: "Sign Up - Threadify" },
    { name: "description", content: "Create your Threadify account" },
  ];
};

export default function Signup() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [formData, setFormData] = useState<SignupData>({
    company_name: '',
    email: '',
    password: '',
    full_name: '',
    job_role: '',
    industry: undefined,
    company_size: undefined,
    use_case: undefined,
    middle_name: '',
  });
  const [alert, setAlert] = useState<{ type: AlertType; message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [loading, setLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [invitationToken, setInvitationToken] = useState<string | null>(null);
  const [companyName, setCompanyName] = useState<string>('');
  const [loadingInvitation, setLoadingInvitation] = useState(false);

  useEffect(() => {
    const token = searchParams.get('invitation_token');
    const email = searchParams.get('email');
    
    if (token) {
      setInvitationToken(token);
      setLoadingInvitation(true);
      
      // Fetch invitation details using API client
      api.validateInvitation(token)
        .then(data => {
          if (data.company_name) {
            setCompanyName(data.company_name);
            setFormData(prev => ({ ...prev, company_name: data.company_name }));
          }
          if (data.email) {
            setFormData(prev => ({ ...prev, email: data.email }));
          } else if (email) {
            setFormData(prev => ({ ...prev, email }));
          }
        })
        .catch(() => {
          setAlert({
            type: 'error',
            message: 'Invalid or expired invitation link',
          });
        })
        .finally(() => setLoadingInvitation(false));
    }
  }, [searchParams]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAlert(null);
    setLoading(true);

    try {
      const signupData = { ...formData };
      if (invitationToken) {
        signupData.invitation_token = invitationToken;
      }
      await api.signup(signupData);
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
          <h2 className="mt-6 text-3xl font-bold text-black">
            {invitationToken ? `Join ${companyName || 'the team'}` : 'Create your account'}
          </h2>
          <p className="mt-2 text-sm text-gray-600">
            {invitationToken 
              ? 'Complete your account setup to join the team' 
              : 'Service-delivery intelligence for how your business delivers'}
          </p>
        </div>

        {/* Form */}
        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          {alert && <Alert type={alert.type} message={alert.message} details={alert.details} />}

          <div className="space-y-4">
            {/* Honeypot field - hidden from humans */}
            <div 
              style={{ position: 'absolute', left: '-9999px', top: '-9999px', opacity: 0, width: 0, height: 0, zIndex: -1 }} 
              aria-hidden="true"
            >
              <input
                id="middle_name"
                name="middle_name"
                type="text"
                tabIndex={-1}
                autoComplete="off"
                value={formData.middle_name}
                onChange={handleChange}
              />
            </div>

            {/* Company Name - conditionally shown/disabled */}
            {!invitationToken && (
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
                  className="w-full px-4 py-3 border-2 border-black rounded-xl focus:outline-none focus:ring-2 focus:ring-black outline-none transition-all bg-white font-medium"
                  placeholder="Acme Corp"
                />
              </div>
            )}
            {invitationToken && companyName && (
              <div>
                <label className="block text-sm font-medium text-black mb-1">
                  Company
                </label>
                <div className="w-full px-4 py-3 border-2 border-black rounded-xl bg-gray-50 text-gray-700 font-medium">
                  {companyName}
                </div>
              </div>
            )}

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
                disabled={!!invitationToken}
                className={`w-full px-4 py-3 border-2 rounded-xl focus:outline-none focus:ring-2 transition-all font-medium ${
                  invitationToken 
                    ? 'border-gray-300 bg-gray-50 text-gray-400 cursor-not-allowed' 
                    : 'border-black focus:ring-black'
                }`}
                placeholder="you@company.com"
              />
            </div>

            {/* Password */}
            <div>
              <label htmlFor="password" className="block text-sm font-medium text-black mb-1">
                Password <span className="text-red-600">*</span>
              </label>
              <div className="relative">
                <input
                  id="password"
                  name="password"
                  type={showPassword ? "text" : "password"}
                  required
                  minLength={8}
                  value={formData.password}
                  onChange={handleChange}
                  className="w-full px-4 py-3 pr-12 border-2 border-black rounded-xl focus:outline-none focus:ring-2 focus:ring-black outline-none transition-all bg-white font-medium"
                  placeholder="Min. 8 characters"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 hover:text-gray-700"
                >
                  {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                </button>
              </div>
              <p className="mt-1 text-xs text-gray-500">Must be at least 8 characters</p>
            </div>

          </div>

          {/* Submit Button */}
          <button
            type="submit"
            disabled={loading || loadingInvitation}
            className="w-full bg-black text-white py-3 px-4 rounded-xl font-medium hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
          >
            {loading ? (invitationToken ? 'Joining team...' : 'Creating account...') : (invitationToken ? 'Join Team' : 'Create Account')}
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
