import { useState, useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api, type User } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import Alert from '~/components/Alert';

export const meta: MetaFunction = () => {
  return [
    { title: "Settings - Threadify" },
    { name: "description", content: "Manage your account settings" },
  ];
};

export default function Settings() {
  const navigate = useNavigate();
  const [user, setUser] = useState<any>(null);
  const [activeTab, setActiveTab] = useState<'profile' | 'company'>('profile');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  // Profile form
  const [profileForm, setProfileForm] = useState({
    full_name: '',
    job_role: '',
  });

  // Company form
  const [companyForm, setCompanyForm] = useState({
    industry: '',
    company_size: '',
    use_case: '',
  });

  useEffect(() => {
    // Check authentication
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }

    const storedUser = api.getStoredUser();
    if (storedUser) {
      setUser(storedUser);
      setProfileForm({
        full_name: storedUser.full_name || '',
        job_role: storedUser.job_role || '',
      });
      // Company info would come from a separate API call
    }
  }, [navigate]);

  const handleProfileUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    setSuccess('');

    try {
      const response = await api.updateProfile({
        ...profileForm,
        ...companyForm,
      });
      api.setUser(response.user);
      setUser(response.user);
      setSuccess('Profile updated successfully!');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update profile');
    } finally {
      setLoading(false);
    }
  };

  return (
    <AppLayout>
      <div className="p-8">
        <div className="mb-8">
          <h2 className="text-2xl font-bold mb-2">Settings</h2>
          <p className="text-gray-600">
            Manage your profile and company information
          </p>
        </div>

        {/* Tabs */}
        <div className="border-b-2 border-black mb-8">
          <div className="flex gap-4">
            <button
              onClick={() => setActiveTab('profile')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'profile'
                  ? 'border-b-4 border-black -mb-0.5'
                  : 'text-gray-600 hover:text-black'
              }`}
            >
              Profile
            </button>
            <button
              onClick={() => setActiveTab('company')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'company'
                  ? 'border-b-4 border-black -mb-0.5'
                  : 'text-gray-600 hover:text-black'
              }`}
            >
              Company
            </button>
          </div>
        </div>

        {/* Messages */}
        {error && <Alert type="error" message={error} className="mb-6" />}
        {success && (
          <div className="bg-green-600 text-white px-4 py-3 mb-6">
            {success}
          </div>
        )}

        {/* Profile Tab */}
        {activeTab === 'profile' && (
          <form onSubmit={handleProfileUpdate} className="space-y-6">
            <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
              <h3 className="text-xl font-bold mb-6">Personal Information</h3>

              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Email
                  </label>
                  <input
                    type="email"
                    value={user?.email || ''}
                    disabled
                    className="w-full px-4 py-3 border-2 border-gray-300 bg-gray-100 cursor-not-allowed"
                  />
                  <p className="text-sm text-gray-600 mt-1">
                    Email cannot be changed
                  </p>
                </div>

                <div>
                  <label className="block text-sm font-medium mb-2">
                    Full Name <span className="text-red-600">*</span>
                  </label>
                  <input
                    type="text"
                    required
                    value={profileForm.full_name}
                    onChange={(e) => setProfileForm({ ...profileForm, full_name: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium mb-2">
                    Job Role <span className="text-red-600">*</span>
                  </label>
                  <input
                    type="text"
                    required
                    value={profileForm.job_role}
                    onChange={(e) => setProfileForm({ ...profileForm, job_role: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                  />
                </div>
              </div>
            </div>

            <button
              type="submit"
              disabled={loading}
              className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? 'Saving...' : 'Save Changes'}
            </button>
          </form>
        )}

        {/* Company Tab */}
        {activeTab === 'company' && (
          <form onSubmit={handleProfileUpdate} className="space-y-6">
            <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
              <h3 className="text-xl font-bold mb-6">Company Information</h3>

              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Company Name
                  </label>
                  <input
                    type="text"
                    value={user?.company_name || ''}
                    disabled
                    className="w-full px-4 py-3 border-2 border-gray-300 bg-gray-100 cursor-not-allowed"
                  />
                  <p className="text-sm text-gray-600 mt-1">
                    Company name cannot be changed
                  </p>
                </div>

                <div>
                  <label className="block text-sm font-medium mb-2">
                    Industry <span className="text-red-600">*</span>
                  </label>
                  <select
                    required
                    value={companyForm.industry}
                    onChange={(e) => setCompanyForm({ ...companyForm, industry: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent bg-white"
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

                <div>
                  <label className="block text-sm font-medium mb-2">
                    Company Size <span className="text-red-600">*</span>
                  </label>
                  <select
                    required
                    value={companyForm.company_size}
                    onChange={(e) => setCompanyForm({ ...companyForm, company_size: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent bg-white"
                  >
                    <option value="">Select size</option>
                    <option value="small">Small (1-10 employees)</option>
                    <option value="medium">Medium (11-50 employees)</option>
                    <option value="large">Large (51-200 employees)</option>
                    <option value="enterprise">Enterprise (200+ employees)</option>
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-medium mb-2">
                    Primary Use Case <span className="text-red-600">*</span>
                  </label>
                  <select
                    required
                    value={companyForm.use_case}
                    onChange={(e) => setCompanyForm({ ...companyForm, use_case: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent bg-white"
                  >
                    <option value="">Select use case</option>
                    <option value="Order Processing">Order Processing</option>
                    <option value="Approval Workflows">Approval Workflows</option>
                    <option value="Payment Processing">Payment Processing</option>
                    <option value="Customer Onboarding">Customer Onboarding</option>
                    <option value="Logistics Tracking">Logistics Tracking</option>
                    <option value="Other">Other</option>
                  </select>
                </div>
              </div>
            </div>

            <button
              type="submit"
              disabled={loading}
              className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? 'Saving...' : 'Save Changes'}
            </button>
          </form>
        )}
      </div>
    </AppLayout>
  );
}
