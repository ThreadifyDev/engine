import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from '@remix-run/react';
import { api, type User, type CreditAccountDTO } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import Alert from '~/components/Alert';
import { CreditCard, Zap, ShieldCheck } from 'lucide-react';

export default function Settings() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [user, setUser] = useState<any>(null);
  const [activeTab, setActiveTab] = useState<'profile' | 'company' | 'billing'>('profile');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [billingInfo, setBillingInfo] = useState<CreditAccountDTO | null>(null);
  const [topUpAmount, setTopUpAmount] = useState<number>(10);
  const [customAmount, setCustomAmount] = useState<string>('');

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
      setBillingInfo(response.credit_account);
      setBillingInfo(response.credit_account);
    } catch (err) {
      console.error('Failed to load billing info:', err);
    }
  };

  const handleTopUp = async () => {
    setLoading(true);
    setError('');
    try {
      const amount = customAmount ? parseFloat(customAmount) : topUpAmount;
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
            <button
              onClick={() => setActiveTab('billing')}
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

        {/* Profile Tab */}
        {activeTab === 'profile' && (
          <form key="profile-tab" onSubmit={handleProfileUpdate} className="space-y-6">
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

        {/* Billing Tab */}
        {activeTab === 'billing' && (
          <div key="billing-tab" className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {/* Balance Card */}
              <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
                <div className="flex items-center gap-3 mb-4">
                  <div className="p-2 bg-blue-50 rounded-lg">
                    <CreditCard className="w-5 h-5 text-blue-600" />
                  </div>
                  <h4 className="font-semibold text-gray-900">Credit Balance</h4>
                </div>
                <div className="text-3xl font-bold text-gray-900 mb-1">
                  ${((billingInfo?.balance_millicents || 0) / 100000).toFixed(2)}
                </div>
                <p className="text-sm text-gray-500 mb-6">Available for AI agent and system usage</p>
                
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-2">
                    {[10, 25, 50, 100].map((amt) => (
                      <button
                        key={amt}
                        onClick={() => {
                          setTopUpAmount(amt);
                          setCustomAmount('');
                        }}
                        className={`px-3 py-2 text-xs font-medium rounded-lg border transition-colors ${
                          !customAmount && topUpAmount === amt
                            ? 'bg-gray-900 text-white border-gray-900'
                            : 'bg-white text-gray-700 border-gray-200 hover:border-gray-900'
                        }`}
                      >
                        ${amt}
                      </button>
                    ))}
                  </div>

                  <div className="relative">
                    <span className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 text-sm">$</span>
                    <input
                      type="number"
                      placeholder="Custom amount"
                      value={customAmount}
                      onChange={(e) => setCustomAmount(e.target.value)}
                      className="w-full pl-7 pr-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                    />
                  </div>


                  <button 
                    onClick={handleTopUp}
                    disabled={loading}
                    className="w-full px-4 py-2 bg-gray-900 text-white rounded-lg hover:bg-gray-800 transition-colors text-sm font-medium disabled:opacity-50"
                  >
                    {loading ? 'Initiating...' : `Top Up $${customAmount || topUpAmount}`}
                  </button>
                </div>
              </div>

              {/* Status Card */}
              <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
                <div className="flex items-center gap-3 mb-4">
                  <div className="p-2 bg-green-50 rounded-lg">
                    <Zap className="w-5 h-5 text-green-600" />
                  </div>
                  <h4 className="font-semibold text-gray-900">Monthly Charges</h4>
                </div>
                <div className="text-3xl font-bold text-gray-900 mb-1">
                  ${((billingInfo?.monthly_charged_millicents || 0) / 100000).toFixed(2)}
                </div>
                <div className="flex items-center justify-between text-xs text-gray-500 mt-2">
                  <span>Cumulative top-ups in current billing cycle</span>
                </div>
              </div>

              {/* Safeguard Card */}
              <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm">
                <div className="flex items-center gap-3 mb-4">
                  <div className="p-2 bg-purple-50 rounded-lg">
                    <ShieldCheck className="w-5 h-5 text-purple-600" />
                  </div>
                  <h4 className="font-semibold text-gray-900">Auto-Topup</h4>
                </div>
                <div className="flex items-center gap-2 mb-2">
                  <div className={`w-2.5 h-2.5 rounded-full ${billingInfo?.auto_topup_millicents && billingInfo.auto_topup_millicents > 0 ? 'bg-green-500' : 'bg-gray-300'}`}></div>
                  <span className="text-sm font-medium text-gray-700">
                    {billingInfo?.auto_topup_millicents && billingInfo.auto_topup_millicents > 0 ? 'Enabled' : 'Disabled'}
                  </span>
                </div>
                <p className="text-xs text-gray-500 leading-relaxed">
                  Automatically adds ${((billingInfo?.auto_topup_millicents || 0) / 100000).toFixed(2)} when balance drops below ${((billingInfo?.min_balance_millicents || 0) / 100000).toFixed(2)}.
                </p>
              </div>
            </div>

            {/* Detailed Info */}
            <div className="border border-gray-200 rounded-lg overflow-hidden bg-white shadow-sm">
              <div className="px-6 py-4 border-b border-gray-200 bg-gray-50">
                <h4 className="font-semibold text-gray-900">Billing Details</h4>
              </div>
              <div className="p-6">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-y-4 gap-x-12 text-sm">
                  <div className="flex justify-between py-2 border-b border-gray-50">
                    <span className="text-gray-500">Billing Cycle Start</span>
                    <span className="font-medium">{billingInfo?.billing_cycle_start ? new Date(billingInfo.billing_cycle_start).toLocaleDateString() : 'N/A'}</span>
                  </div>
                  <div className="flex justify-between py-2 border-b border-gray-50">
                    <span className="text-gray-500">Account ID</span>
                    <span className="font-mono text-xs">{billingInfo?.id || 'N/A'}</span>
                  </div>
                  <div className="flex justify-between py-2 border-b border-gray-50">
                    <span className="text-gray-500">Auto-Topup Threshold</span>
                    <span className="font-medium">${((billingInfo?.min_balance_millicents || 0) / 100000).toFixed(2)}</span>
                  </div>
                  <div className="flex justify-between py-2 border-b border-gray-50">
                    <span className="text-gray-500">Next Recharge Amount</span>
                    <span className="font-medium">${((billingInfo?.auto_topup_millicents || 0) / 100000).toFixed(2)}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Company Tab */}
        {activeTab === 'company' && (
          <form key="company-tab" onSubmit={handleProfileUpdate} className="space-y-6">
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

        {/* Billing Tab */}
        {activeTab === 'billing' && (
          <div>
            <BillingCard onUpgrade={() => setShowTierModal(true)} />
            
            {/* Tier Selection Modal */}
            <TierSelectionModal
              isOpen={showTierModal}
              onClose={() => setShowTierModal(false)}
              currentTier={user?.subscription_tier}
            />
          </div>
        )}
      </div>
    </AppLayout>
  );
}
