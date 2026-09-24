import { User } from '~/lib/api';

interface ProfileTabProps {
  user: User | null;
  profileForm: {
    full_name: string;
    job_role: string;
  };
  setProfileForm: (form: { full_name: string; job_role: string }) => void;
  loading: boolean;
  onSubmit: (e: React.FormEvent) => void;
}

export function ProfileTab({ user, profileForm, setProfileForm, loading, onSubmit }: ProfileTabProps) {
  return (
    <form key="profile-tab" onSubmit={onSubmit} className="space-y-5">
      <div className="rounded-2xl border border-stone-200 bg-white p-6 shadow-sm sm:p-8">
        <h3 className="mb-6 border-b border-stone-100 pb-5 text-lg font-semibold tracking-tight text-stone-900">Personal Information</h3>

        <div className="grid gap-5 md:grid-cols-2">
          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Email
            </label>
            <input
              type="email"
              value={user?.email || ''}
              disabled
              className="w-full cursor-not-allowed rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 text-sm text-stone-500"
            />
            <p className="text-sm text-gray-600 mt-1">
              Email cannot be changed
            </p>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Full Name <span className="text-red-600">*</span>
            </label>
            <input
              type="text"
              required
              value={profileForm.full_name}
              onChange={(e) => setProfileForm({ ...profileForm, full_name: e.target.value })}
              className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
            />
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              Job Role <span className="text-red-600">*</span>
            </label>
            <input
              type="text"
              required
              value={profileForm.job_role}
              onChange={(e) => setProfileForm({ ...profileForm, job_role: e.target.value })}
              className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
            />
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
