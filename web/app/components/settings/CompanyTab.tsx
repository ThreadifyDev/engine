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
    <form key="company-tab" onSubmit={onSubmit} className="space-y-5">
      <div className="rounded-2xl border border-stone-200 bg-white p-6 shadow-sm sm:p-8">
        <h3 className="mb-6 border-b border-stone-100 pb-5 text-lg font-semibold tracking-tight text-stone-900">Company Information</h3>

        <div className="grid gap-5 md:grid-cols-2">
          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Company Name
            </label>
            <input
              type="text"
              value={user?.company_name || ''}
              disabled
              className="w-full cursor-not-allowed rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 text-sm text-stone-500"
            />
            <p className="text-sm text-gray-600 mt-1">
              Company name cannot be changed
            </p>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Industry <span className="text-red-600">*</span>
            </label>
            <select
              required
              value={companyForm.industry}
              onChange={(e) => setCompanyForm({ ...companyForm, industry: e.target.value })}
              className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Company Size <span className="text-red-600">*</span>
            </label>
            <select
              required
              value={companyForm.company_size}
              onChange={(e) => setCompanyForm({ ...companyForm, company_size: e.target.value })}
              className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
            >
              <option value="">Select size</option>
              <option value="small">Small (1-10 employees)</option>
              <option value="medium">Medium (11-50 employees)</option>
              <option value="large">Large (51-200 employees)</option>
              <option value="enterprise">Enterprise (200+ employees)</option>
            </select>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Primary Use Case <span className="text-red-600">*</span>
            </label>
            <select
              required
              value={companyForm.use_case}
              onChange={(e) => setCompanyForm({ ...companyForm, use_case: e.target.value })}
              className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
        className="rounded-lg bg-stone-900 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? 'Saving...' : 'Save changes'}
      </button>
    </form>
  );
}
