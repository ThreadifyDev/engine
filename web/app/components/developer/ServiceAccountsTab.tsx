import { useState, useEffect } from 'react';
import { Check } from 'lucide-react';
import { api } from '~/lib/api';
import { useServiceAccountRoles } from '~/hooks/useRoles';
import { useServiceAccounts, useCreateServiceAccount, useToggleServiceAccount, useDeleteServiceAccount } from '~/hooks/useServiceAccounts';

const ROLE_COLORS: Record<string, string> = {
  standard_service: 'bg-blue-50 text-blue-700 border-blue-200 text-xs px-2 py-0.5 rounded-full',
  reader: 'bg-gray-50 text-gray-700 border-gray-200 text-xs px-2 py-0.5 rounded-full',
  member: 'bg-purple-50 text-purple-700 border-purple-200 text-xs px-2 py-0.5 rounded-full',
};

export function ServiceAccountsTab() {
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

  // Set default role when roles are loaded
  useEffect(() => {
    if (roles.length > 0 && !createForm.role) {
      const defaultRole = roles.find(r => r.value === 'standard_service')?.value || roles[0].value;
      setCreateForm({ name: '', description: '', role: defaultRole });
    }

    // Listen for create button click from parent
    const handleCreate = () => setShowCreateModal(true);
    window.addEventListener('create-service-account', handleCreate);
    return () => window.removeEventListener('create-service-account', handleCreate);
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
    <div>
      {/* Error Message */}
      {error && (
        <div role="alert" className="mb-6 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800">
          {error}
        </div>
      )}

      {/* Service Accounts List */}
      {loading ? (
        <div role="status" className="space-y-3 rounded-2xl border border-stone-200 bg-white p-6">
          {[0, 1, 2].map(item => <div key={item} className="h-14 animate-pulse rounded-lg bg-stone-100" />)}
          <span className="sr-only">Loading service accounts</span>
        </div>
      ) : serviceAccounts.length === 0 ? (
        <div className="rounded-2xl border border-stone-200 bg-white px-6 py-16 text-center shadow-sm shadow-stone-200/40">
          <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-stone-100">
            <span className="text-sm font-semibold text-stone-500">SA</span>
          </div>
          <h3 className="mb-2 text-base font-semibold text-stone-900">No service accounts yet</h3>
          <p className="mb-6 text-sm text-stone-500">
            Create a service account to manage API access with specific roles
          </p>
          <button
            onClick={() => setShowCreateModal(true)}
            className="rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white hover:bg-stone-800"
          >
            Create Your First Service Account
          </button>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
          <table className="w-full min-w-[800px]">
            <thead className="border-b border-stone-200 bg-stone-50">
              <tr>
                <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">Name</th>
                <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">Role</th>
                <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">Status</th>
                <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">Last Used</th>
                <th className="px-4 sm:px-6 py-3 text-left text-xs font-semibold uppercase tracking-wide text-stone-500">Created</th>
                <th className="px-4 sm:px-6 py-3 text-right text-xs font-semibold uppercase tracking-wide text-stone-500">Actions</th>
              </tr>
            </thead>
            <tbody>
              {serviceAccounts.map((sa, index) => (
                <tr
                  key={sa.id}
                  className={`hover:bg-stone-50 transition-colors ${
                    index !== serviceAccounts.length - 1 ? 'border-b border-stone-100' : ''
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
                  <td className="px-4 sm:px-6 py-4 text-sm text-gray-600">
                    {formatDate(sa.last_used_at)}
                  </td>
                  <td className="px-4 sm:px-6 py-4 text-sm text-gray-600">
                    {formatDate(sa.created_at)}
                  </td>
                  <td className="px-4 sm:px-6 py-4 text-right">
                    <div className="flex flex-col sm:flex-row gap-4 justify-end items-center">
                      <button
                        onClick={() => handleViewPermissions(sa.role || 'standard_service')}
                        className="rounded-lg border border-stone-200 px-3 py-1.5 text-sm font-medium text-stone-700 transition-colors hover:bg-stone-50"
                      >
                        Permissions
                      </button>
                      <button
                        onClick={() => handleToggleActive(sa.id, sa.is_active)}
                        className="rounded-lg border border-stone-200 px-3 py-1.5 text-sm font-medium text-stone-700 transition-colors hover:bg-stone-50"
                      >
                        {sa.is_active ? 'Deactivate' : 'Activate'}
                      </button>
                      <button
                        onClick={() => handleDelete(sa.id)}
                        className="text-red-700 hover:text-red-800 font-medium transition-colors text-sm"
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
        <div role="dialog" aria-modal="true" aria-label="Create service account" className="fixed inset-0 z-[100] flex items-center justify-center bg-stone-950/50 p-4">
          <div className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-2xl border border-stone-200 bg-white p-6 shadow-2xl sm:p-8">
            <h2 className="mb-6 text-xl font-semibold tracking-tight text-stone-900">Create Service Account</h2>

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
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
                  className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100 bg-white"
                >
                  {(roles || []).map((role) => (
                    <option key={role.value} value={role.value}>
                      {role.label} - {role.description}
                    </option>
                  ))}
                </select>
                <p className="mt-2 text-xs text-gray-500">
                  Roles define what permissions this service account has for notifications, API endpoints, and resources
                </p>
              </div>

              <div className="flex justify-end items-center gap-6 mt-2">
                <button
                  type="button"
                  onClick={() => {
                    setShowCreateModal(false);
                    setCreateForm({ name: '', description: '', role: roles[0]?.value || 'standard_service' });
                  }}
                  className="text-red-500 hover:text-red-700 font-medium transition-colors text-sm"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createMutation.isPending}
                  className="px-8 py-3 bg-black rounded-xl text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50"
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
                            {grouped.notifications.map((perm, idx) => (
                              <div key={idx} className="flex items-start gap-2">
                                <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                  <Check className="w-3 h-3 text-gray-700" />
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
                            {grouped.ui.map((perm, idx) => (
                              <div key={idx} className="flex items-start gap-2">
                                <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                  <Check className="w-3 h-3 text-gray-700" />
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
                            {grouped.api.map((perm, idx) => (
                              <div key={idx} className="flex items-start gap-2">
                                <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                  <Check className="w-3 h-3 text-gray-700" />
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
                            {grouped.sdk.map((perm, idx) => (
                              <div key={idx} className="flex items-start gap-2">
                                <div className="w-4 h-4 flex items-center justify-center bg-gray-100 rounded-sm mt-0.5">
                                  <Check className="w-3 h-3 text-gray-700" />
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
  );
}
