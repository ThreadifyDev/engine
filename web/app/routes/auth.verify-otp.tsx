import { useState, useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate, useSearchParams, Link } from '@remix-run/react';
import { api, type VerifyOTPData, ValidationError } from '~/lib/api';
import Alert from '~/components/Alert';

export const meta: MetaFunction = () => {
  return [
    { title: "Verify OTP - Threadify" },
    { name: "description", content: "Verify your email with the one-time password" },
  ];
};

export default function VerifyOTP() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const email = searchParams.get('email') || '';
  const isLoginFlow = searchParams.get('type') === 'login';

  const [formData, setFormData] = useState<VerifyOTPData>({
    email: email,
    token: '',
  });
  const [error, setError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [loading, setLoading] = useState(false);
  const [resending, setResending] = useState(false);
  const [resendMessage, setResendMessage] = useState('');

  useEffect(() => {
    if (!email) {
      navigate('/login');
    }
  }, [email, navigate]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setResendMessage('');
    setLoading(true);

    try {
      // Store token and user info
      const response = await api.verifyOTP(formData);
      api.setToken(response.token);
      api.setUser(response.user);

      // Redirect based on user status
      if (!response.user.onboarding_completed) {
        navigate('/u/onboarding');
      } else if (!response.user.first_instrumentation_done) {
        navigate('/u/getting-started');
      } else {
        navigate('/u/dashboard');
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Verification failed';
      
      // Check if token has expired (only auto-resend for expiration, not invalid tokens)
      if (errorMessage.toLowerCase().includes('token has expired')) {
        // Automatically resend the OTP
        setResending(true);
        try {
          await api.resendVerificationEmail({ email });
          setError({
            message: 'Token has expired. A new code has been sent to your email!',
          });
          setFormData({ ...formData, token: '' }); // Clear the expired token
        } catch (resendErr) {
          setError({
            message: 'Token has expired. Failed to send new code. Please try again.',
          });
        } finally {
          setResending(false);
        }
      } else {
        // Handle other errors normally
        if (err instanceof ValidationError) {
          setError({
            message: err.message,
            details: err.details,
          });
        } else {
          setError({
            message: errorMessage,
          });
        }
      }
    } finally {
      setLoading(false);
    }
  };

  const handleCodeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value.replace(/\D/g, '').slice(0, 8);
    setFormData({ ...formData, token: value });
  };

  const handleResendVerification = async () => {
    setError(null);
    setResendMessage('');
    setResending(true);

    try {
      const response = await api.resendVerificationEmail({ email });
      setResendMessage(response.message || 'Verification email sent!');
    } catch (err) {
      setResendMessage(err instanceof Error ? err.message : 'Failed to resend email');
    } finally {
      setResending(false);
    }
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
            {isLoginFlow ? 'Enter your login code' : 'Verify your email'}
          </h2>
          <p className="mt-2 text-sm text-gray-600">
            {isLoginFlow
              ? <>We sent a login code to <span className="font-medium text-black">{email}</span></>
              : <>We sent an 8-digit code to <span className="font-medium text-black">{email}</span></>}
          </p>
        </div>

        {/* Form */}
        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          {error && <Alert type="error" message={error.message} details={error.details} />}

          <div>
            <label htmlFor="code" className="block text-sm font-medium text-black mb-1">
              Verification Code <span className="text-red-600">*</span>
            </label>
            <input
              id="code"
              name="code"
              type="text"
              inputMode="numeric"
              pattern="[0-9]{8}"
              required
              maxLength={8}
              minLength={8}
              value={formData.token}
              onChange={handleCodeChange}
              className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black text-center text-2xl tracking-widest font-mono"
              placeholder="00000000"
              autoComplete="one-time-code"
            />
            <p className="mt-2 text-xs text-gray-500">
              Enter the 8-digit code sent to your email (expires in 10 minutes)
            </p>
          </div>

          {/* Submit Button */}
          <button
            type="submit"
            disabled={loading || formData.token.length !== 8}
            className="w-full bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Verifying...' : 'Verify Code'}
          </button>

          {/* Back Link */}
          <div className="text-center text-sm">
            <Link to={isLoginFlow ? '/login' : '/login'} className="text-gray-600 hover:text-black">
              ← Back to login
            </Link>
          </div>
        </form>

        {/* Resend Code */}
        <div className="space-y-3">
          {resendMessage && (
            <div className={`px-4 py-3 text-sm text-center ${resendMessage.includes('Failed') || resendMessage.includes('error')
              ? 'bg-red-50 text-red-800 border-2 border-red-500'
              : 'bg-green-50 text-green-800 border-2 border-green-500'
              }`}>
              {resendMessage}
            </div>
          )}

          <div className="text-center">
            <p className="text-sm text-gray-600">
              Didn't receive the code?{' '}
              <button
                type="button"
                className="text-black font-medium hover:underline disabled:opacity-50 disabled:cursor-not-allowed"
                onClick={handleResendVerification}
                disabled={resending}
              >
                {resending ? 'Sending...' : 'Resend'}
              </button>
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
