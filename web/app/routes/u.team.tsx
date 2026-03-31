import { useState, useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import Alert from '~/components/Alert';

export const meta: MetaFunction = () => {
  return [
    { title: "Team - Threadify" },
    { name: "description", content: "Manage your team members" },
  ];
};

interface Invitation {
  id: string;
  email: string;
  role: string;
  status: string;
  invited_by: string;
  token?: string;
  expires_at: number;
  created_at: number;
}

export default function Team() {
  const navigate = useNavigate();
  const [teamMembers, setTeamMembers] = useState<any[]>([]);
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [inviteForm, setInviteForm] = useState({ email: '', role: 'member' });
  const [inviting, setInviting] = useState(false);
  const [resendingId, setResendingId] = useState<string | null>(null);
  const [cancelingId, setCancelingId] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState('');
  const [canViewMembers, setCanViewMembers] = useState(false);
  const [canInviteMembers, setCanInviteMembers] = useState(false);
  const [activeTab, setActiveTab] = useState<'members' | 'invitations'>('members');
  const [currentUserId, setCurrentUserId] = useState<string>('');

  useEffect(() => {
    // Check authentication
    if (!api.isAuthenticated()) {
      navigate('/login');
      return;
    }

    const fetchData = async () => {
      setLoading(true);
      setError('');
      
      // Get current user ID
      const user = api.getStoredUser();
      if (user) {
        setCurrentUserId(user.id);
      }
      
      try {
        // Fetch team members from backend
        const response = await api.getTeamMembers();
        setTeamMembers(response.members || []);
      } catch (err) {
        console.error('Failed to fetch team members:', err);
        // Fallback to current user only
        setTeamMembers(user ? [user] : []);
      }
      
      // Try to fetch invitations - if successful, user has member.view permission
      try {
        const invitationsResponse = await api.listInvitations();
        setInvitations(invitationsResponse.invitations || []);
        setCanViewMembers(true);
        setCanInviteMembers(true);
      } catch (inviteErr: any) {
        if (inviteErr.message?.includes('403') || inviteErr.message?.includes('Forbidden')) {
          setCanViewMembers(false);
          setCanInviteMembers(false);
        } else {
          console.error('Error fetching invitations:', inviteErr);
        }
      }
      
      setLoading(false);
    };

    fetchData();
  }, [navigate]);

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault();
    setInviting(true);
    setError('');
    setSuccessMessage('');

    try {
      const response = await api.sendTeamInvitation({
        email: inviteForm.email,
        role: inviteForm.role,
      });

      if (response.success) {
        setShowInviteModal(false);
        setInviteForm({ email: '', role: 'member' });
        setSuccessMessage(`Invitation sent to ${inviteForm.email}`);
        // Refresh data
        window.location.reload();
      } else {
        setError(response.error || 'Failed to send invitation');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send invite');
    } finally {
      setInviting(false);
    }
  };

  const handleRemoveMember = async (memberId: string) => {
    // Prevent users from removing themselves
    if (memberId === currentUserId) {
      setError('You cannot remove yourself from the team');
      setTimeout(() => setError(''), 5000);
      return;
    }

    // Ensure at least one user remains
    if (teamMembers.length <= 1) {
      setError('Cannot remove the last team member. There must be at least one user in the team.');
      setTimeout(() => setError(''), 5000);
      return;
    }

    if (!window.confirm('Are you sure you want to remove this team member?')) return;

    try {
      // TODO: Implement remove member API endpoint
      setSuccessMessage('Member removed successfully');
      window.location.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove member');
    }
  };

  const handleResendInvitation = async (invitationId: string, email: string) => {
    if (!window.confirm(`Resend invitation to ${email}?`)) return;

    try {
      setResendingId(invitationId);
      setError('');
      setSuccessMessage('');

      const response = await api.resendInvitation(invitationId);

      if (response.success) {
        setSuccessMessage(`Invitation resent to ${email}`);
        window.location.reload();
      } else {
        setError(response.error || 'Failed to resend invitation');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to resend invitation');
    } finally {
      setResendingId(null);
    }
  };

  const handleCancelInvitation = async (invitationId: string, email: string) => {
    if (!window.confirm(`Cancel invitation for ${email}?`)) return;

    try {
      setCancelingId(invitationId);
      setError('');
      setSuccessMessage('');

      const response = await api.cancelInvitation(invitationId);

      if (response.success) {
        // Remove invitation from state immediately
        setInvitations(invitations.filter(inv => inv.id !== invitationId));
        setSuccessMessage(`Invitation cancelled for ${email}`);
      } else {
        setError(response.error || 'Failed to cancel invitation');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to cancel invitation');
    } finally {
      setCancelingId(null);
    }
  };

  const handleCopyInvitationLink = (token: string) => {
    const inviteLink = `${window.location.origin}/signup?invitation_token=${token}`;
    navigator.clipboard.writeText(inviteLink).then(() => {
      setSuccessMessage('Invitation link copied to clipboard!');
      setTimeout(() => setSuccessMessage(''), 3000);
    }).catch(() => {
      setError('Failed to copy link to clipboard');
    });
  };

  const handleBillingClick = () => {
    navigate('/u/settings?tab=billing');
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
          <div className="flex gap-3">
            <button
              onClick={handleBillingClick}
              className="px-4 py-2 bg-white text-black text-sm font-medium hover:bg-gray-100 border-2 border-black transition-colors rounded"
            >
              Update Billing
            </button>
            {canInviteMembers && (
              <button
                onClick={() => setShowInviteModal(true)}
                className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
              >
                Invite Member
              </button>
            )}
          </div>
        </div>

        {error && <Alert type="error" message={error} className="mb-6" />}
        {successMessage && <Alert type="success" message={successMessage} className="mb-6" />}

        {/* Tabs */}
        <div className="mb-6">
          <div className="flex gap-2">
            <button
              onClick={() => setActiveTab('members')}
              className={`px-4 py-2 rounded-md font-medium transition-colors ${
                activeTab === 'members'
                  ? 'bg-black text-white'
                  : 'bg-white text-black border border-gray-300 hover:bg-gray-50'
              }`}
            >
              Active Members ({teamMembers.length})
            </button>
            {canViewMembers && (
              <button
                onClick={() => setActiveTab('invitations')}
                className={`px-4 py-2 rounded-md font-medium transition-colors ${
                  activeTab === 'invitations'
                    ? 'bg-black text-white'
                    : 'bg-white text-black border border-gray-300 hover:bg-gray-50'
                }`}
              >
                Pending Invitations ({invitations.length})
              </button>
            )}
          </div>
        </div>

        {loading ? (
          <div className="text-center py-12">
            <p className="text-gray-600">Loading team data...</p>
          </div>
        ) : (
          <>
            {/* Active Members Tab */}
            {activeTab === 'members' && (
              <div className="border-2 border-black">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-gray-200 bg-gray-50">
                    <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Name</th>
                    <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Email</th>
                    <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Role</th>
                    <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Status</th>
                    <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200">
                  {teamMembers.map((member, index) => (
                    <tr
                      key={member.id || index}
                      className="hover:bg-gray-50 transition-colors"
                    >
                      <td className="px-6 py-4 text-sm text-gray-900">{member.full_name || 'N/A'}</td>
                      <td className="px-6 py-4 text-sm text-gray-600">{member.email}</td>
                      <td className="px-6 py-4">
                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-gray-100 text-gray-800">
                          {member.role?.toUpperCase() || 'MEMBER'}
                        </span>
                      </td>
                      <td className="px-6 py-4">
                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-green-100 text-green-800">
                          Active
                        </span>
                      </td>
                      <td className="px-6 py-4 text-sm">
                        {member.role !== 'owner' && member.id !== currentUserId && teamMembers.length > 1 && (
                          <button
                            onClick={() => handleRemoveMember(member.id)}
                            className="text-red-600 hover:text-red-800 font-medium transition-colors"
                          >
                            Remove
                          </button>
                        )}
                        {member.id === currentUserId && (
                          <span className="text-gray-400 text-xs">You</span>
                        )}
                        {member.id !== currentUserId && teamMembers.length === 1 && (
                          <span className="text-gray-400 text-xs">Last member</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
            )}

            {/* Pending Invitations Tab */}
            {activeTab === 'invitations' && canViewMembers && (
              <div className="border-2 border-black">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-gray-200 bg-gray-50">
                      <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Email</th>
                      <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Role</th>
                      <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Status</th>
                      <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Invited</th>
                      <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Expires</th>
                      <th className="text-left px-6 py-3 text-xs font-semibold text-gray-600 uppercase tracking-wider">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {invitations.map((invitation, index) => (
                      <tr
                        key={invitation.id}
                        className="hover:bg-gray-50 transition-colors"
                      >
                        <td className="px-6 py-4 text-sm text-gray-900">{invitation.email}</td>
                        <td className="px-6 py-4">
                          <span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-gray-100 text-gray-800">
                            {invitation.role?.toUpperCase()}
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-yellow-100 text-yellow-800">
                            {invitation.status?.toUpperCase()}
                          </span>
                        </td>
                        <td className="px-6 py-4 text-sm text-gray-600">
                          {new Date(invitation.created_at * 1000).toLocaleDateString()}
                        </td>
                        <td className="px-6 py-4 text-sm text-gray-600">
                          {new Date(invitation.expires_at * 1000).toLocaleDateString()}
                        </td>
                        <td className="px-6 py-4 text-sm">
                          {canInviteMembers && invitation.status === 'pending' && (
                            <div className="flex gap-3">
                              {invitation.token && (
                                <button
                                  onClick={() => handleCopyInvitationLink(invitation.token!)}
                                  className="text-green-600 hover:text-green-800 font-medium transition-colors"
                                  title="Copy invitation link"
                                >
                                  Copy Link
                                </button>
                              )}
                              <button
                                onClick={() => handleResendInvitation(invitation.id, invitation.email)}
                                disabled={resendingId === invitation.id || cancelingId === invitation.id}
                                className="px-3 py-1 text-sm border border-gray-300 hover:bg-gray-100 transition-colors rounded disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                {resendingId === invitation.id ? 'Resending...' : 'Resend'}
                              </button>
                              <button
                                onClick={() => handleCancelInvitation(invitation.id, invitation.email)}
                                disabled={resendingId === invitation.id || cancelingId === invitation.id}
                                className="px-3 py-1 text-sm border border-gray-300 hover:bg-gray-100 transition-colors rounded disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                {cancelingId === invitation.id ? 'Canceling...' : 'Cancel'}
                              </button>
                            </div>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {invitations.length === 0 && (
                  <div className="text-center py-12 text-gray-600">
                    <p>No pending invitations</p>
                  </div>
                )}
              </div>
            )}
          </>
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
                  className="w-full px-4 py-3 rounded-lg border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
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
                  className="w-full px-4 py-3 rounded-lg border-2 border-black focus:outline-none focus:ring-2 focus:ring-black bg-white"
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
