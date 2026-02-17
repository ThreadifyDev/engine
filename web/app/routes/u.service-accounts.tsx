import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import { useServiceAccountRoles } from '~/hooks/useRoles';
import { useServiceAccounts, useCreateServiceAccount, useToggleServiceAccount, useDeleteServiceAccount } from '~/hooks/useServiceAccounts';

const ROLE_COLORS: Record<string, string> = {
  standard_service: 'bg-blue-50 text-blue-700 border-blue-200 text-xs px-2 py-0.5 rounded-full',
  reader: 'bg-gray-50 text-gray-700 border-gray-200 text-xs px-2 py-0.5 rounded-full',
  standard_account: 'bg-purple-50 text-purple-700 border-purple-200 text-xs px-2 py-0.5 rounded-full',
};

export default function ServiceAccounts() {
  const navigate = useNavigate();
  
  // TanStack Query hooks
  const { roles, isLoading: rolesLoading } = useServiceAccountRoles();
  const { data: serviceAccounts = [], isLoading: accountsLoading, error: accountsError } = useServiceAccounts();
  const createMutation = useCreateServiceAccount();
  const toggleMutation = useToggleServiceAccount();
  const deleteMutation = useDeleteServiceAccount();
  
  // Local UI state
  const [error, setError] = useState('');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showPermissionsModal, setShowPermissionsModal] = useState(false);
  const [selectedRole, setSelectedRole] = useState<string>('');
  const [permissions, setPermissions] = useState<any[]>([]);
  const [createForm, setCreateForm] = useState({ name: '', description: '', role: 'standard_service' });

  const loading = rolesLoading || accountsLoading;

  useEffect(() => {
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }
  }, [navigate]);

  // Set default role when roles are loaded
  useEffect(() => {
    if (roles.length > 0 && !createForm.role) {
      const defaultRole = roles.find(r => r.value === 'standard_service')?.value || roles[0].value;
      setCreateForm({ name: '', description: '', role: defaultRole });
    }
  }, [roles, createForm.role]);

  // Show query errors
  useEffect(() => {
    if (accountsError) {
      setError(accountsError instanceof Error ? accountsError.message : 'Failed to load service accounts');
    }
  }, [accountsError]);

  const handleCreateServiceAccount = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    createMutation.mutate(createForm, {
      onSuccess: () => {
        setShowCreateModal(false);
        setCreateForm({ name: '', description: '', role: 'standard_service' });
      },
      onError: (err) => {
        setError(err instanceof Error ? err.message : 'Failed to create service account');
      },
    });
  };

  const handleToggleActive = async (id: string, isActive: boolean) => {
    toggleMutation.mutate({ id, isActive }, {
      onError: (err) => {
        setError(err instanceof Error ? err.message : 'Failed to update service account');
      },
    });
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this service account?')) return;

    deleteMutation.mutate(id, {
      onError: (err) => {
        setError(err instanceof Error ? err.message : 'Failed to delete service account');
      },
    });
  };

  const handleViewPermissions = async (role: string) => {
    try {
      setSelectedRole(role);
      
      // Get permissions from the loaded roles data
      const roleData = roles.find(r => r.value === role);
      if (roleData) {
        // Fetch the full role details from the API to get permissions
        const response = await api.getRolesByLevel('api_level');
        const fullRoleData = response.roles[role];
        
        if (fullRoleData && fullRoleData.permissions) {
          // Convert permissions array to the expected format
          const perms = fullRoleData.permissions.map((perm: string) => ({
            name: perm,
            description: `Permission: ${perm}`,
            resource: perm.split('.')[0], // Extract resource from permission pattern
          }));
          setPermissions(perms);
        } else {
          setPermissions([]);
        }
      }
      
      setShowPermissionsModal(true);
    } catch (err: any) {
      setError(err.message || 'Failed to load permissions');
    }
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Never';
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  };

  const getRoleBadgeColor = (role: string) => {
    return ROLE_COLORS[role] || 'bg-gray-50 text-gray-700 border-gray-200 text-xs px-2 py-0.5 rounded-full';
  };

  const getRoleDisplayName = (roleKey: string) => {
    const role = roles.find(r => r.value === roleKey);
    return role ? role.label : roleKey.replace('_', ' ').toUpperCase();
  };

  const groupPermissionsByResource = (perms: any[]) => {
    const grouped: Record<string, any[]> = {
      notifications: [],
      ui: [],
      api: [],
      sdk: [],
    };

    perms.forEach(perm => {
      if (grouped[perm.resource]) {
        grouped[perm.resource].push(perm);
      }
    });

    return grouped;
  };

  return (
    <AppLayout>
      <div className="p-4 sm:p-6 lg:p-8">
        {/* Page Header */}
        <div className="mb-8">
          <h1 className="text-2xl font-bold text-gray-900 mb-1">Service Accounts</h1>
          <p className="text-sm text-gray-500">
            Manage service accounts and their role-based permissions
          </p>
        </div>

        {/* Error Message */}
        {error && (
          <div className="mb-6 p-4 bg-red-50 border border-red-200 text-red-800 rounded text-sm">
            {error}
          </div>
        )}

        {/* Create Button */}
        <div className="mb-6">
          <button
            onClick={() => setShowCreateModal(true)}
            className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
          >
            + New Service Account
          </button>
        </div>

        {/* Service Accounts List */}
        {loading ? (
          <div className="text-center py-12">
            <p className="text-gray-500">Loading service accounts...</p>
          </div>
        ) : serviceAccounts.length === 0 ? (
          <div className="text-center py-16 bg-white border border-gray-200 rounded-lg">
            <div className="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-4">
              <span className="text-2xl text-gray-400">SA</span>
            </div>
            <h3 className="text-lg font-semibold text-gray-900 mb-2">No service accounts yet</h3>
            <p className="text-sm text-gray-500 mb-6">
              Create a service account to manage API access with specific roles
            </p>
            <button
              onClick={() => setShowCreateModal(true)}
              className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
            >
              Create Your First Service Account
            </button>
          </div>
        ) : (
          <div className="bg-white border border-gray-200 rounded-lg overflow-x-auto">
            <table className="w-full min-w-[800px]">
              <thead className="border-b border-gray-200 bg-gray-50">
                <tr>
                  <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Name</th>
                  <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Role</th>
                  <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Status</th>
                  <th className="hidden md:table-cell px-4 sm:px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Last Used</th>
                  <th className="hidden lg:table-cell px-4 sm:px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Created</th>
                  <th className="px-4 sm:px-6 py-3 text-right text-xs font-semibold text-gray-600 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody>
                {serviceAccounts.map((sa, index) => (
                  <tr
                    key={sa.id}
                    className={`hover:bg-gray-50 transition-colors ${
                      index !== serviceAccounts.length - 1 ? 'border-b border-gray-200' : ''
                    }`}
                  >
                    <td className="px-4 sm:px-6 py-4">
                      <div>
                        <div className="font-medium text-gray-900">{sa.name}</div>
                        {sa.description && (
                          <div className="text-sm text-gray-500">{sa.description}</div>
                        )}
                      </div>
                    </td>
                    <td className="px-4 sm:px-6 py-4">
                      <span className={`px-2 sm:px-3 py-1 text-xs font-medium rounded-full border ${getRoleBadgeColor(sa.role || 'standard_service')}`}>
                        {getRoleDisplayName(sa.role || 'standard_service')}
                      </span>
                    </td>
                    <td className="px-4 sm:px-6 py-4">
                      <span className={`px-2 sm:px-3 py-1 text-xs font-medium rounded-full ${
                        sa.is_active 
                          ? 'bg-green-100 text-green-800' 
                          : 'bg-gray-100 text-gray-800'
                      }`}>
                        {sa.is_active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="hidden md:table-cell px-4 sm:px-6 py-4 text-sm text-gray-600">
                      {formatDate(sa.last_used_at)}
                    </td>
                    <td className="hidden lg:table-cell px-4 sm:px-6 py-4 text-sm text-gray-600">
                      {formatDate(sa.created_at)}
                    </td>
                    <td className="px-4 sm:px-6 py-4 text-right">
                      <div className="flex flex-col sm:flex-row gap-2 justify-end">
                        <button
                          onClick={() => handleViewPermissions(sa.role || 'standard_service')}
                          className="px-3 py-1 text-sm border border-gray-300 hover:bg-gray-100 transition-colors rounded"
                        >
                          Permissions
                        </button>
                        <button
                          onClick={() => handleToggleActive(sa.id, sa.is_active)}
                          className="px-3 py-1 text-sm border border-gray-300 hover:bg-gray-100 transition-colors rounded"
                        >
                          {sa.is_active ? 'Deactivate' : 'Activate'}
                        </button>
                        <button
                          onClick={() => handleDelete(sa.id)}
                          className="px-3 py-1 text-sm border border-red-600 text-red-600 hover:bg-red-600 hover:text-white transition-colors rounded"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Create Modal */}
        {showCreateModal && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
            <div className="bg-white rounded-lg shadow-xl p-8 max-w-md w-full mx-4">
              <h2 className="text-2xl font-bold mb-6">Create Service Account</h2>

              <form onSubmit={handleCreateServiceAccount}>
                <div className="mb-4">
                  <label htmlFor="name" className="block text-sm font-medium text-gray-700 mb-2">
                    Name <span className="text-red-600">*</span>
                  </label>
                  <input
                    id="name"
                    type="text"
                    required
                    value={createForm.name}
                    onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent"
                    placeholder="CI/CD Pipeline"
                  />
                </div>

                <div className="mb-4">
                  <label htmlFor="description" className="block text-sm font-medium text-gray-700 mb-2">
                    Description
                  </label>
                  <textarea
                    id="description"
                    value={createForm.description}
                    onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent"
                    placeholder="GitHub Actions deployment"
                    rows={3}
                  />
                </div>

                <div className="mb-6">
                  <label htmlFor="role" className="block text-sm font-medium text-gray-700 mb-2">
                    Role <span className="text-red-600">*</span>
                  </label>
                  <select
                    id="role"
                    value={createForm.role}
                    onChange={(e) => setCreateForm({ ...createForm, role: e.target.value })}
                    className="w-full px-4 py-3 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent bg-white"
                  >
                    {roles.map((role) => (
                      <option key={role.value} value={role.value}>
                        {role.label} - {role.description}
                      </option>
                    ))}
                  </select>
                  <p className="mt-2 text-xs text-gray-500">
                    Roles define what permissions this service account has for notifications, API endpoints, and resources
                  </p>
                </div>

                <div className="flex gap-4">
                  <button
                    type="button"
                    onClick={() => {
                      setShowCreateModal(false);
                      setCreateForm({ name: '', description: '', role: roles[0]?.value || 'standard_service' });
                    }}
                    className="flex-1 px-6 py-3 border border-gray-300 hover:bg-gray-100 transition-colors font-medium rounded"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={createMutation.isPending}
                    className="flex-1 px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors disabled:opacity-50 rounded"
                  >
                    {createMutation.isPending ? 'Creating...' : 'Create'}
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}

        {/* Permissions Modal */}
        {showPermissionsModal && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
            <div className="bg-white rounded-lg shadow-xl p-8 max-w-3xl w-full mx-4 max-h-[80vh] overflow-y-auto">
              <div className="flex justify-between items-center mb-6">
                <div>
                  <h2 className="text-2xl font-bold">
                    {getRoleDisplayName(selectedRole)} Role
                  </h2>
                  <p className="text-sm text-gray-600 mt-1">
                    {roles.find(r => r.value === selectedRole)?.description}
                  </p>
                </div>
                <button
                  onClick={() => setShowPermissionsModal(false)}
                  className="text-2xl font-bold hover:text-gray-600"
                >
                  ×
                </button>
              </div>

              {permissions.length === 0 ? (
                <p className="text-gray-500">No permissions found</p>
              ) : (
                <div className="space-y-6">
                  {(() => {
                    const grouped = groupPermissionsByResource(permissions);
                    return (
                      <>
                        {/* Notifications */}
                        {grouped.notifications.length > 0 && (
                          <div>
                            <div className="flex items-center gap-3 mb-3">
                              <div className="w-8 h-8 flex items-center justify-center bg-gray-200 rounded-full">
                                <span className="text-sm font-bold text-gray-700">N</span>
                              </div>
                              <h3 className="text-base font-semibold text-gray-900">Notifications ({grouped.notifications.length})</h3>
                            </div>
                            <div className="space-y-2 ml-11">
                              {grouped.notifications.map((perm) => (
                                <div key={perm.id} className="flex items-start gap-2">
                                  <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                    <span className="text-xs text-gray-700">✓</span>
                                  </div>
                                  <div>
                                    <p className="font-medium text-sm text-gray-900">{perm.name}</p>
                                    <p className="text-xs text-gray-500">{perm.description}</p>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        {/* Web UI */}
                        {grouped.ui.length > 0 && (
                          <div>
                            <div className="flex items-center gap-3 mb-3">
                              <div className="w-8 h-8 flex items-center justify-center bg-gray-200 rounded-full">
                                <span className="text-sm font-bold text-gray-700">UI</span>
                              </div>
                              <h3 className="text-base font-semibold text-gray-900">Web UI ({grouped.ui.length})</h3>
                            </div>
                            <div className="space-y-2 ml-11">
                              {grouped.ui.map((perm) => (
                                <div key={perm.id} className="flex items-start gap-2">
                                  <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                    <span className="text-xs text-gray-700">✓</span>
                                  </div>
                                  <div>
                                    <p className="font-medium text-sm text-gray-900">{perm.name}</p>
                                    <p className="text-xs text-gray-500">{perm.description}</p>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        {/* API/GraphQL */}
                        {grouped.api.length > 0 && (
                          <div>
                            <div className="flex items-center gap-3 mb-3">
                              <div className="w-8 h-8 flex items-center justify-center bg-gray-200 rounded-full">
                                <span className="text-sm font-bold text-gray-700">API</span>
                              </div>
                              <h3 className="text-base font-semibold text-gray-900">API/GraphQL ({grouped.api.length})</h3>
                            </div>
                            <div className="space-y-2 ml-11">
                              {grouped.api.map((perm) => (
                                <div key={perm.id} className="flex items-start gap-2">
                                  <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                    <span className="text-xs text-gray-700">✓</span>
                                  </div>
                                  <div>
                                    <p className="font-medium text-sm text-gray-900">{perm.name}</p>
                                    <p className="text-xs text-gray-500">{perm.description}</p>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        {/* SDK */}
                        {grouped.sdk.length > 0 && (
                          <div>
                            <div className="flex items-center gap-3 mb-3">
                              <div className="w-8 h-8 flex items-center justify-center bg-gray-200 rounded-full">
                                <span className="text-sm font-bold text-gray-700">SDK</span>
                              </div>
                              <h3 className="text-base font-semibold text-gray-900">SDK ({grouped.sdk.length})</h3>
                            </div>
                            <div className="space-y-2 ml-11">
                              {grouped.sdk.map((perm) => (
                                <div key={perm.id} className="flex items-start gap-2">
                                  <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                    <span className="text-xs text-gray-700">✓</span>
                                  </div>
                                  <div>
                                    <p className="font-medium text-sm text-gray-900">{perm.name}</p>
                                    <p className="text-xs text-gray-500">{perm.description}</p>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}
                      </>
                    );
                  })()}
                </div>
              )}

              <button
                onClick={() => setShowPermissionsModal(false)}
                className="w-full mt-6 px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors rounded"
              >
                Close
              </button>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}
