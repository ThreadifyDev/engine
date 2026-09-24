import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { api, ValidationError } from '~/lib/api';
import Alert from '~/components/Alert';
import AgentToggleButton from '~/components/agent/AgentToggleButton';


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
  const [error, setError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [loading, setLoading] = useState(false);
  const [skipCompanyStep, setSkipCompanyStep] = useState(false);

  useEffect(() => {
    // Redirect if not authenticated
    if (!api.isAuthenticated()) {
      navigate('/login');
      return;
    }

    const user = api.getStoredUser();
    
    // Redirect if already onboarded
    if (user?.onboarding_completed) {
      navigate('/u/dashboard');
      return;
    }
    
    // Check if user joined via invitation (company info already exists)
    // Invited users have first_instrumentation_done = true
    const checkCompanyStatus = async () => {
      try {
        const response = await api.getUserProfile(true); // minimal=true for onboarding
        
        // If user has first_instrumentation_done = true, they joined via invitation
        // Skip company step since company details already exist
        if (response.user?.first_instrumentation_done) {
          setSkipCompanyStep(true);
        }
      } catch (err) {
        // If we can't check, default to showing both steps
        console.error('Failed to check company status:', err);
      }
    };
    
    checkCompanyStatus();
  }, [navigate]); // Only run on mount and when navigate changes

  const handleNext = (e: React.FormEvent) => {
    e.preventDefault();
    
    // If company step should be skipped, submit directly
    if (skipCompanyStep) {
      handleSubmit(e);
    } else {
      setStep(2);
    }
  };

  const handleBack = () => {
    setStep(1);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      // Prepare data - use "Other" text if selected, otherwise use dropdown value
      const profileData: any = {
        full_name: formData.full_name,
        job_role: formData.job_role === 'Other' ? formData.job_role_other : formData.job_role,
      };
      
      // Only include company data if we're not skipping the company step
      if (!skipCompanyStep) {
        profileData.industry = formData.industry === 'Other' ? formData.industry_other : formData.industry;
        profileData.company_size = formData.company_size;
        profileData.use_case = formData.use_case === 'Other' ? formData.use_case_other : formData.use_case;
      }

      // Call API to update profile
      const response = await api.updateProfile(profileData);
      
      // Update stored user with onboarding_completed flag
      const updatedUser = { ...response.user, onboarding_completed: true };
      api.setUser(updatedUser);
      
      // Navigate to getting-started (mandatory, non-skippable)
      navigate('/u/getting-started');
    } catch (err) {
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({
          message: err instanceof Error ? err.message : 'Failed to complete onboarding',
        });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  const handleSkip = () => {
    navigate('/u/dashboard');
  };

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-[#f8f8f6] px-4 py-12">
      <div className="absolute right-4 top-4"><AgentToggleButton /></div>
      <div className="w-full max-w-xl">
        <header className="mb-7 text-center">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Welcome / Setup</p>
          <h1 className="text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">Set up your workspace</h1>
          <p className="mt-3 text-sm text-stone-500">A few details help tailor your Threadify experience.</p>
          {!skipCompanyStep && <div className="mt-6 flex items-center justify-center gap-2" aria-label={`Step ${step} of 2`}>
            <span className={`flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold ${step === 1 ? 'bg-emerald-700 text-white' : 'bg-emerald-50 text-emerald-700'}`}>1</span>
            <span className="h-px w-12 bg-stone-200" />
            <span className={`flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold ${step === 2 ? 'bg-emerald-700 text-white' : 'bg-stone-100 text-stone-500'}`}>2</span>
          </div>}
        </header>

        {/* Step 1: Personal Information */}
        {step === 1 && (
          <form className="space-y-6 rounded-2xl border border-stone-200 bg-white p-6 shadow-sm sm:p-8" onSubmit={handleNext}>
            <div className="mb-6 border-b border-stone-100 pb-5">
              <h2 className="text-xl font-semibold tracking-tight text-stone-900">Tell us about yourself</h2>
              <p className="mt-1 text-sm text-stone-500">
                This helps us personalize your Threadify experience
              </p>
            </div>

            {error && <Alert type="error" message={error.message} details={error.details} />}

            <div className="space-y-4">
              {/* Full Name */}
              <div>
                <label htmlFor="full_name" className="mb-2 block text-sm font-medium text-stone-700">
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
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                  placeholder="John Doe"
                />
              </div>

              {/* Job Role */}
              <div>
                <label htmlFor="job_role" className="mb-2 block text-sm font-medium text-stone-700">
                  Job Role <span className="text-red-600">*</span>
                </label>
                <select
                  id="job_role"
                  name="job_role"
                  required
                  value={formData.job_role}
                  onChange={handleChange}
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
                  <label htmlFor="job_role_other" className="mb-2 block text-sm font-medium text-stone-700">
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
                    className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                    placeholder="Enter your job role"
                  />
                </div>
              )}
            </div>

            {/* Buttons */}
            <div className="flex gap-3 border-t border-stone-100 pt-5">
              <button
                type="button"
                onClick={handleSkip}
                className="flex-1 rounded-lg border border-stone-200 px-4 py-2.5 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
              >
                Skip for now
              </button>
              <button
                type="submit"
                className="flex-1 rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700"
              >
                {skipCompanyStep ? 'Complete Setup' : 'Continue'}
              </button>
            </div>
          </form>
        )}

        {/* Step 2: Company Information */}
        {step === 2 && (
          <form className="space-y-6 rounded-2xl border border-stone-200 bg-white p-6 shadow-sm sm:p-8" onSubmit={handleSubmit}>
            <div className="mb-6 border-b border-stone-100 pb-5">
              <h2 className="text-xl font-semibold tracking-tight text-stone-900">About your company</h2>
              <p className="mt-1 text-sm text-stone-500">
                Help us understand your use case and tailor recommendations
              </p>
            </div>

            {error && <Alert type="error" message={error.message} details={error.details} />}

            <div className="space-y-4">
              {/* Industry */}
              <div>
                <label htmlFor="industry" className="mb-2 block text-sm font-medium text-stone-700">
                  Industry <span className="text-red-600">*</span>
                </label>
                <select
                  id="industry"
                  name="industry"
                  required
                  value={formData.industry}
                  onChange={handleChange}
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
                  <label htmlFor="industry_other" className="mb-2 block text-sm font-medium text-stone-700">
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
                    className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                    placeholder="Enter your industry"
                  />
                </div>
              )}

              {/* Company Size */}
              <div>
                <label htmlFor="company_size" className="mb-2 block text-sm font-medium text-stone-700">
                  Company Size <span className="text-red-600">*</span>
                </label>
                <select
                  id="company_size"
                  name="company_size"
                  required
                  value={formData.company_size}
                  onChange={handleChange}
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
                <label htmlFor="use_case" className="mb-2 block text-sm font-medium text-stone-700">
                  What will you use Threadify for? <span className="text-red-600">*</span>
                </label>
                <select
                  id="use_case"
                  name="use_case"
                  required
                  value={formData.use_case}
                  onChange={handleChange}
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
                  <label htmlFor="use_case_other" className="mb-2 block text-sm font-medium text-stone-700">
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
                    className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                    placeholder="Describe what you'll monitor with Threadify"
                  />
                </div>
              )}
            </div>

            {/* Buttons */}
            <div className="flex gap-3 border-t border-stone-100 pt-5">
              <button
                type="button"
                onClick={handleBack}
                className="flex-1 rounded-lg border border-stone-200 px-4 py-2.5 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
              >
                Back
              </button>
              <button
                type="submit"
                disabled={loading}
                className="flex-1 rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-50"
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
