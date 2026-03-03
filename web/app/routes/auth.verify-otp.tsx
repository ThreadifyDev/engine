import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams, Link } from '@remix-run/react';
import { api, type VerifyOTPData } from '~/lib/api';

export default function VerifyOTP() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const email = searchParams.get('email') || '';
  const isLoginFlow = searchParams.get('type') === 'login';


  const [code, setCode] = useState('');
  const [error, setError] = useState('');
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
    setError('');
    setLoading(true);

    try {
      const data: VerifyOTPData = { email, token: code };
      const response = await api.verifyOTP(data);
      // Store token and user info
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
      setError(err instanceof Error ? err.message : 'Verification failed');
    } finally {
      setLoading(false);
    }
  };

  const handleCodeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value.replace(/\D/g, '').slice(0, 8);
    setCode(value);
  };

  const handleResendVerification = async () => {
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
          {error && (
            <div className="bg-black text-white px-4 py-3 text-sm">
              {error}
            </div>
          )}

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
              value={code}
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
            disabled={loading || code.length !== 8}
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

        {/* Resend Code — only shown on signup flow */}
        {!isLoginFlow && (
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
        )}
      </div>
    </div>
  );
}
