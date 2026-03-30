import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from '@remix-run/react';
import { api, type User, type GetCurrentPlanResponse } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import Alert from '~/components/Alert';
import { ProfileTab } from '~/components/settings/ProfileTab';
import { BillingTab } from '~/components/settings/BillingTab';
import { CompanyTab } from '~/components/settings/CompanyTab';

function formatBillingDate(dateString: string): string {
  const date = new Date(dateString);
  const day = date.getDate();
  const month = date.toLocaleDateString('en-US', { month: 'short' });
  const year = date.getFullYear();
  
  // Add ordinal suffix (st, nd, rd, th)
  const suffix = (day: number) => {
    if (day > 3 && day < 21) return 'th';
    switch (day % 10) {
      case 1: return 'st';
      case 2: return 'nd';
      case 3: return 'rd';
      default: return 'th';
    }
  };
  
  return `${day}${suffix(day)} ${month} ${year}`;
}

export default function Settings() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [user, setUser] = useState<any>(null);
  const [activeTab, setActiveTab] = useState<'profile' | 'company' | 'billing'>('profile');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [billingInfo, setBillingInfo] = useState<GetCurrentPlanResponse | null>(null);

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
    // Check for tab in query params
    const tabParam = searchParams.get('tab');
    if (tabParam === 'billing' || tabParam === 'company' || tabParam === 'profile') {
      setActiveTab(tabParam as any);
    }

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

    loadBillingInfo();
  }, [navigate]);

  const loadBillingInfo = async () => {
    try {
      const response = await api.getBillingInfo();
      setBillingInfo(response);
    } catch (err) {
      console.error('Failed to load billing info:', err);
    }
  };

  const handleTopUp = async (amount: number) => {
    setLoading(true);
    setError('');
    try {
      if (isNaN(amount) || amount <= 0) {
        throw new Error('Please enter a valid top-up amount');
      }
      
      // Convert USD to millicents (1 USD = 100,000 millicents)
      const amountMillicents = Math.round(amount * 100000);
      const response = await api.createCheckoutSession(amountMillicents);
      if (response.url) {
        window.location.href = response.url;
      } else {
        throw new Error('No checkout URL returned from server');
      }
    } catch (err: any) {
      setError(err.message || 'Failed to initiate checkout');
      setLoading(false);
    }
  };

  const handleUpdateMonthlyLimit = async (limitStr: string) => {
    setLoading(true);
    setError('');
    setSuccess('');
    try {
      const limit = parseFloat(limitStr);

      if (isNaN(limit) || limit <= 0) {
        throw new Error('Please enter a valid monthly spending limit');
      }
      
      // Convert USD to millicents (1 USD = 100,000 millicents)
      const maxLimitMillicents = Math.round(limit * 100000);

      await api.updateMonthlyLimit(maxLimitMillicents);
      setSuccess('Monthly spending limit updated successfully!');
      await loadBillingInfo(); // Reload billing info
    } catch (err: any) {
      setError(err.message || 'Failed to update monthly limit');
    } finally {
      setLoading(false);
    }
  };

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
              onClick={() => navigate('?tab=profile')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'profile'
                  ? 'border-b-4 border-black -mb-0.5'
                  : 'text-gray-600 hover:text-black'
              }`}
            >
              Profile
            </button>
            <button
              onClick={() => navigate('?tab=company')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'company'
                  ? 'border-b-4 border-black -mb-0.5'
                  : 'text-gray-600 hover:text-black'
              }`}
            >
              Company
            </button>
            <button
              onClick={() => navigate('?tab=billing')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'billing'
                  ? 'border-b-4 border-black -mb-0.5'
                  : 'text-gray-600 hover:text-black'
              }`}
            >
              Billing & Credits
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

        {activeTab === 'profile' && (
          <ProfileTab
            user={user}
            profileForm={profileForm}
            setProfileForm={setProfileForm}
            loading={loading}
            onSubmit={handleProfileUpdate}
          />
        )}

        {activeTab === 'billing' && (
          <BillingTab
            billingInfo={billingInfo}
            loading={loading}
            onTopUp={handleTopUp}
            onUpdateMonthlyLimit={handleUpdateMonthlyLimit}
          />
        )}

        {activeTab === 'company' && (
          <CompanyTab
            user={user}
            companyForm={companyForm}
            setCompanyForm={setCompanyForm}
            loading={loading}
            onSubmit={handleProfileUpdate}
          />
        )}
      </div>
    </AppLayout>
  );
}
