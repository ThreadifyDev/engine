import { User } from '~/lib/api';

interface CompanyTabProps {
  user: User | null;
  companyForm: {
    industry: string;
    company_size: string;
    use_case: string;
  };
  setCompanyForm: (form: { industry: string; company_size: string; use_case: string }) => void;
  loading: boolean;
  onSubmit: (e: React.FormEvent) => void;
}

export function CompanyTab({ user, companyForm, setCompanyForm, loading, onSubmit }: CompanyTabProps) {
  return (
    <form key="company-tab" onSubmit={onSubmit} className="space-y-6">
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
              className="w-full px-4 py-3 border-2 rounded-lg border-gray-300 bg-gray-100 cursor-not-allowed"
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
  );
}
