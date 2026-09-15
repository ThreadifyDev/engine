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
    <form key="profile-tab" onSubmit={onSubmit} className="space-y-6">
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
        className="px-4 py-2 rounded-lg bg-black text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {loading ? 'Saving...' : 'Save Changes'}
      </button>
    </form>
  );
}
