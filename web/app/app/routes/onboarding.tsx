import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';

export default function Onboarding() {
  const navigate = useNavigate();
  const user = api.getStoredUser();
  const [step, setStep] = useState(1);
  const [formData, setFormData] = useState({
    full_name: '',
    job_role: '',
    job_role_other: '',
    industry: '',
    industry_other: '',
    company_size: '',
    use_case: '',
    use_case_other: '',
  });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    // Redirect if not authenticated
    if (!api.isAuthenticated()) {
      navigate('/auth/login');
      return;
    }

    // Redirect if already onboarded
    if (user?.onboarding_completed) {
      navigate('/dashboard');
    }
  }, [navigate, user]);

  const handleNext = (e: React.FormEvent) => {
    e.preventDefault();
    setStep(2);
  };

  const handleBack = () => {
    setStep(1);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      // Prepare data - use "Other" text if selected, otherwise use dropdown value
      const profileData = {
        full_name: formData.full_name,
        job_role: formData.job_role === 'Other' ? formData.job_role_other : formData.job_role,
        industry: formData.industry === 'Other' ? formData.industry_other : formData.industry,
        company_size: formData.company_size,
        use_case: formData.use_case === 'Other' ? formData.use_case_other : formData.use_case,
      };

      // Call API to update profile
      const response = await api.updateProfile(profileData);
      
      // Update stored user with onboarding_completed flag
      const updatedUser = { ...response.user, onboarding_completed: true };
      api.setUser(updatedUser);
      
      // Navigate to getting-started (mandatory, non-skippable)
      navigate('/getting-started');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to complete onboarding');
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  const handleSkip = () => {
    navigate('/dashboard');
  };

  return (
    <div className="min-h-screen bg-white flex items-center justify-center px-4 py-12">
      <div className="max-w-md w-full space-y-8">
        {/* Header */}
        <div className="text-center">
          <h1 className="text-4xl font-bold text-black" style={{ fontFamily: 'Block, monospace' }}>
            Threadify
          </h1>
          <div className="mt-6">
            <div className="flex items-center justify-center gap-2 mb-4">
              <div className={`w-8 h-8 flex items-center justify-center border-2 ${step === 1 ? 'bg-black text-white border-black' : 'border-gray-300 text-gray-400'} font-bold`}>
                1
              </div>
              <div className="w-12 h-0.5 bg-gray-300"></div>
              <div className={`w-8 h-8 flex items-center justify-center border-2 ${step === 2 ? 'bg-black text-white border-black' : 'border-gray-300 text-gray-400'} font-bold`}>
                2
              </div>
            </div>
          </div>
        </div>

        {/* Step 1: Personal Information */}
        {step === 1 && (
          <form className="space-y-6" onSubmit={handleNext}>
            <div className="text-center mb-6">
              <h2 className="text-2xl font-bold text-black">Tell us about yourself</h2>
              <p className="mt-2 text-sm text-gray-600">
                This helps us personalize your Threadify experience
              </p>
            </div>

            {error && (
              <div className="bg-black text-white px-4 py-3 text-sm">
                {error}
              </div>
            )}

            <div className="space-y-4">
              {/* Full Name */}
              <div>
                <label htmlFor="full_name" className="block text-sm font-medium text-black mb-1">
                  Full Name <span className="text-red-600">*</span>
                </label>
                <input
                  id="full_name"
                  name="full_name"
                  type="text"
                  required
                  minLength={2}
                  value={formData.full_name}
                  onChange={handleChange}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                  placeholder="John Doe"
                />
              </div>

              {/* Job Role */}
              <div>
                <label htmlFor="job_role" className="block text-sm font-medium text-black mb-1">
                  Job Role <span className="text-red-600">*</span>
                </label>
                <select
                  id="job_role"
                  name="job_role"
                  required
                  value={formData.job_role}
                  onChange={handleChange}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black bg-white"
                >
                  <option value="">Select role</option>
                  <option value="Software Engineer">Software Engineer</option>
                  <option value="Quality Engineer">Quality Engineer</option>
                  <option value="Operations Specialist">Operations Specialist</option>
                  <option value="Technical Support">Technical Support</option>
                  <option value="CEO/C-Suite">CEO / C-Suite</option>
                  <option value="CTO/Technical Lead">CTO / Technical Lead</option>
                  <option value="Other">Other</option>
                </select>
              </div>

              {/* Job Role Other */}
              {formData.job_role === 'Other' && (
                <div>
                  <label htmlFor="job_role_other" className="block text-sm font-medium text-black mb-1">
                    Please specify your role <span className="text-red-600">*</span>
                  </label>
                  <input
                    id="job_role_other"
                    name="job_role_other"
                    type="text"
                    required
                    minLength={2}
                    value={formData.job_role_other}
                    onChange={handleChange}
                    className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                    placeholder="Enter your job role"
                  />
                </div>
              )}
            </div>

            {/* Buttons */}
            <div className="flex gap-4">
              <button
                type="button"
                onClick={handleSkip}
                className="flex-1 border-2 border-black text-black py-3 px-4 font-medium hover:bg-gray-100 transition-colors"
              >
                Skip for now
              </button>
              <button
                type="submit"
                className="flex-1 bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 transition-colors"
              >
                Continue
              </button>
            </div>
          </form>
        )}

        {/* Step 2: Company Information */}
        {step === 2 && (
          <form className="space-y-6" onSubmit={handleSubmit}>
            <div className="text-center mb-6">
              <h2 className="text-2xl font-bold text-black">About your company</h2>
              <p className="mt-2 text-sm text-gray-600">
                Help us understand your use case and tailor recommendations
              </p>
            </div>

            {error && (
              <div className="bg-black text-white px-4 py-3 text-sm">
                {error}
              </div>
            )}

            <div className="space-y-4">
              {/* Industry */}
              <div>
                <label htmlFor="industry" className="block text-sm font-medium text-black mb-1">
                  Industry <span className="text-red-600">*</span>
                </label>
                <select
                  id="industry"
                  name="industry"
                  required
                  value={formData.industry}
                  onChange={handleChange}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black bg-white"
                >
                  <option value="">Select industry</option>
                  <option value="E-commerce">E-commerce</option>
                  <option value="SaaS">SaaS</option>
                  <option value="FinTech">FinTech</option>
                  <option value="Healthcare">Healthcare</option>
                  <option value="Logistics">Logistics & Supply Chain</option>
                  <option value="Manufacturing">Manufacturing</option>
                  <option value="Retail">Retail</option>
                  <option value="Other">Other</option>
                </select>
              </div>

              {/* Industry Other */}
              {formData.industry === 'Other' && (
                <div>
                  <label htmlFor="industry_other" className="block text-sm font-medium text-black mb-1">
                    Please specify <span className="text-red-600">*</span>
                  </label>
                  <input
                    id="industry_other"
                    name="industry_other"
                    type="text"
                    required
                    minLength={2}
                    value={formData.industry_other}
                    onChange={handleChange}
                    className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                    placeholder="Enter your industry"
                  />
                </div>
              )}

              {/* Company Size */}
              <div>
                <label htmlFor="company_size" className="block text-sm font-medium text-black mb-1">
                  Company Size <span className="text-red-600">*</span>
                </label>
                <select
                  id="company_size"
                  name="company_size"
                  required
                  value={formData.company_size}
                  onChange={handleChange}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black bg-white"
                >
                  <option value="">Select size</option>
                  <option value="small">Small (1-10 employees)</option>
                  <option value="medium">Medium (11-50 employees)</option>
                  <option value="large">Large (51-200 employees)</option>
                  <option value="enterprise">Enterprise (200+ employees)</option>
                </select>
              </div>

              {/* Use Case */}
              <div>
                <label htmlFor="use_case" className="block text-sm font-medium text-black mb-1">
                  What will you use Threadify for? <span className="text-red-600">*</span>
                </label>
                <select
                  id="use_case"
                  name="use_case"
                  required
                  value={formData.use_case}
                  onChange={handleChange}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black bg-white"
                >
                  <option value="">Select use case</option>
                  <option value="Order Fulfillment">Order Fulfillment & Tracking</option>
                  <option value="Payment Processing">Payment Processing</option>
                  <option value="User Onboarding">User Onboarding Flows</option>
                  <option value="Approval Workflows">Approval Workflows</option>
                  <option value="Inventory Management">Inventory Management</option>
                  <option value="Customer Support">Customer Support Tickets</option>
                  <option value="Delivery Tracking">Delivery & Shipping Tracking</option>
                  <option value="Compliance">Compliance & Audit Trails</option>
                  <option value="Other">Other</option>
                </select>
              </div>

              {/* Use Case Other */}
              {formData.use_case === 'Other' && (
                <div>
                  <label htmlFor="use_case_other" className="block text-sm font-medium text-black mb-1">
                    Please describe your use case <span className="text-red-600">*</span>
                  </label>
                  <input
                    id="use_case_other"
                    name="use_case_other"
                    type="text"
                    required
                    minLength={5}
                    value={formData.use_case_other}
                    onChange={handleChange}
                    className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                    placeholder="Describe what you'll monitor with Threadify"
                  />
                </div>
              )}
            </div>

            {/* Buttons */}
            <div className="flex gap-4">
              <button
                type="button"
                onClick={handleBack}
                className="flex-1 border-2 border-black text-black py-3 px-4 font-medium hover:bg-gray-100 transition-colors"
              >
                Back
              </button>
              <button
                type="submit"
                disabled={loading}
                className="flex-1 bg-black text-white py-3 px-4 font-medium hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              >
                {loading ? 'Completing...' : 'Complete Setup'}
              </button>
            </div>

            {/* Skip option */}
            <div className="text-center">
              <button
                type="button"
                onClick={handleSkip}
                className="text-sm text-gray-600 hover:text-black underline"
              >
                Skip and go to dashboard
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
