import { TabBar } from '~/components/TabBar';
import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from '@remix-run/react';
import { api, type User, type GetCurrentPlanResponse, ValidationError } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import Alert, { isCreditError } from '~/components/Alert';
import { ProfileTab } from '~/components/settings/ProfileTab';
import { BillingTab } from '~/components/settings/BillingTab';
import { CompanyTab } from '~/components/settings/CompanyTab';
import { EngineTab } from '~/components/settings/EngineTab';

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
  const [activeTab, setActiveTab] = useState<'profile' | 'company' | 'billing' | 'engine'>('profile');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);
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
    if (tabParam === 'billing' || tabParam === 'company' || tabParam === 'profile' || tabParam === 'engine') {
      setActiveTab(tabParam as any);
    }
  }, [searchParams]);

  useEffect(() => {
    // Check authentication
    const token = api.isAuthenticated();
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
    }

    // Load company info
    loadCompanyInfo();
    loadBillingInfo();
  }, [navigate]);

  const loadCompanyInfo = async () => {
    try {
      const response = await api.getUserProfile();
      if (response.company) {
        setCompanyForm({
          industry: response.company.industry || '',
          company_size: response.company.company_size || '',
          use_case: response.company.use_case || '',
        });
      }
    } catch (err) {
      console.error('Failed to load company info:', err);
    }
  };

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
    setError(null);
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
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({
          message: err.message || 'Failed to initiate checkout',
        });
      }
      setLoading(false);
    }
  };

  const handleUpdateMonthlyLimit = async (limitStr: string) => {
    setLoading(true);
    setError(null);
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
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({
          message: err.message || 'Failed to update monthly limit',
        });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleProfileUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
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
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({
          message: err instanceof Error ? err.message : 'Failed to update profile',
        });
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <AppLayout>
      <div className="p-8 text-sm">
        <div className="mb-8">
          <h2 className="text-xl font-bold mb-2">Settings</h2>
          <p className="text-gray-600">
            Manage your profile, company and Engine settings
          </p>
        </div>

        <TabBar label="Settings" value={activeTab} onChange={tab => navigate(`?tab=${tab}`)} panelId="settings-panel" className="mb-6"
          items={[{value:'engine',label:'Engine'},{value:'profile',label:'Profile'},{value:'company',label:'Company'},{value:'billing',label:'Billing & Credits'}]} />
        <div id="settings-panel" role="tabpanel" aria-label="Settings content">

        {/* Messages */}
        {error && (
          <Alert
            type="error"
            message={error.message}
            details={error.details}
            className="mb-6"
            action={
              isCreditError(error.message)
                ? { label: 'Go to Billing', onClick: () => navigate('?tab=billing'), variant: 'primary' }
                : undefined
            }
          />
        )}
        {success && (
          <div className="bg-green-600 text-white px-4 py-3 mb-6">
            {success}
          </div>
        )}

        {activeTab === 'engine' && <EngineTab />}
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
      </div>
    </AppLayout>
  );
}
