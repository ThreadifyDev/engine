import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import Alert from '~/components/Alert';

export default function Team() {
  const navigate = useNavigate();
  const [teamMembers, setTeamMembers] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [inviteForm, setInviteForm] = useState({ email: '', role: 'member' });
  const [inviting, setInviting] = useState(false);

  useEffect(() => {
    // Check authentication
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }

    fetchTeamMembers();
  }, [navigate]);

  const fetchTeamMembers = async () => {
    try {
      setLoading(true);
      // TODO: Implement team members API endpoint
      // For now, show current user only
      const user = api.getStoredUser();
      setTeamMembers(user ? [{ ...user, role: 'owner' }] : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load team members');
    } finally {
      setLoading(false);
    }
  };

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault();
    setInviting(true);
    setError('');

    try {
      // TODO: Implement invite API endpoint
      alert(`Invite sent to ${inviteForm.email} as ${inviteForm.role}`);
      setShowInviteModal(false);
      setInviteForm({ email: '', role: 'member' });
      fetchTeamMembers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send invite');
    } finally {
      setInviting(false);
    }
  };

  const handleRemoveMember = async (memberId: string) => {
    if (!confirm('Are you sure you want to remove this team member?')) return;

    try {
      // TODO: Implement remove member API endpoint
      alert('Member removed');
      fetchTeamMembers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove member');
    }
  };

  return (
    <AppLayout>
      <div className="p-8">
        <div className="flex justify-between items-center mb-8">
          <div>
            <h2 className="text-2xl font-bold mb-2">Team Members</h2>
            <p className="text-gray-600">
              Manage your team and invite new members
            </p>
          </div>
          <button
            onClick={() => setShowInviteModal(true)}
            className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium"
          >
            Invite Member
          </button>
        </div>

        {error && <Alert type="error" message={error} className="mb-6" />}

        {loading ? (
          <div className="text-center py-12">
            <p className="text-gray-600">Loading team members...</p>
          </div>
        ) : (
          <div className="border-4 border-black">
            <table className="w-full">
              <thead className="border-b-4 border-black">
                <tr>
                  <th className="text-left px-6 py-4 font-bold">Name</th>
                  <th className="text-left px-6 py-4 font-bold">Email</th>
                  <th className="text-left px-6 py-4 font-bold">Role</th>
                  <th className="text-left px-6 py-4 font-bold">Status</th>
                  <th className="text-left px-6 py-4 font-bold">Actions</th>
                </tr>
              </thead>
              <tbody>
                {teamMembers.map((member, index) => (
                  <tr
                    key={member.id || index}
                    className="border-b-2 border-black last:border-b-0 hover:bg-gray-50"
                  >
                    <td className="px-6 py-4">{member.full_name || 'N/A'}</td>
                    <td className="px-6 py-4">{member.email}</td>
                    <td className="px-6 py-4">
                      <span className="px-3 py-1 border-2 border-black text-sm font-medium">
                        {member.role?.toUpperCase() || 'MEMBER'}
                      </span>
                    </td>
                    <td className="px-6 py-4">
                      <span className="text-green-600 font-medium">Active</span>
                    </td>
                    <td className="px-6 py-4">
                      {member.role !== 'owner' && (
                        <button
                          onClick={() => handleRemoveMember(member.id)}
                          className="px-4 py-2 border-2 border-black hover:bg-black hover:text-white transition-colors font-medium"
                        >
                          Remove
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Info Box */}
        <div className="mt-8 border-4 border-black p-6 bg-gray-50">
          <h3 className="text-lg font-bold mb-2">Team Roles</h3>
          <div className="space-y-2 text-sm">
            <div>
              <strong>Owner:</strong> Full access to all features and settings
            </div>
            <div>
              <strong>Admin:</strong> Can manage team members and contracts
            </div>
            <div>
              <strong>Member:</strong> Can view and create threads
            </div>
          </div>
        </div>

        {/* Invite Modal */}
        {showInviteModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50">
          <div className="bg-white border-4 border-black max-w-md w-full">
            <div className="border-b-4 border-black p-6 flex justify-between items-center">
              <h2 className="text-2xl font-bold">Invite Team Member</h2>
              <button
                onClick={() => setShowInviteModal(false)}
                className="text-2xl font-bold hover:text-gray-600"
              >
                ×
              </button>
            </div>

            <form onSubmit={handleInvite} className="p-6">
              <div className="mb-6">
                <label className="block text-sm font-medium mb-2">
                  Email Address <span className="text-red-600">*</span>
                </label>
                <input
                  type="email"
                  required
                  value={inviteForm.email}
                  onChange={(e) => setInviteForm({ ...inviteForm, email: e.target.value })}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                  placeholder="colleague@company.com"
                />
              </div>

              <div className="mb-6">
                <label className="block text-sm font-medium mb-2">
                  Role <span className="text-red-600">*</span>
                </label>
                <select
                  required
                  value={inviteForm.role}
                  onChange={(e) => setInviteForm({ ...inviteForm, role: e.target.value })}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black bg-white"
                >
                  <option value="member">Member</option>
                  <option value="admin">Admin</option>
                  <option value="viewer">Viewer</option>
                </select>
              </div>

              <div className="flex gap-4">
                <button
                  type="submit"
                  disabled={inviting}
                  className="flex-1 px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {inviting ? 'Sending...' : 'Send Invite'}
                </button>
                <button
                  type="button"
                  onClick={() => setShowInviteModal(false)}
                  className="px-6 py-3 border-2 border-black hover:bg-gray-100 transition-colors font-medium"
                >
                  Cancel
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
      </div>
    </AppLayout>
  );
}
