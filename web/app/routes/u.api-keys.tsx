import { useState, useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { Key, Plus, Copy, Check, Trash2, Eye, EyeOff, X } from 'lucide-react';
import { api, ValidationError } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import { useServiceAccountRoles } from '~/hooks/useRoles';
import Alert, { isCreditError } from '~/components/Alert';

export const meta: MetaFunction = () => {
  return [
    { title: "API Keys - Threadify" },
    { name: "description", content: "Manage your API keys" },
  ];
};

export default function APIKeys() {
  const navigate = useNavigate();
  const { roles, isLoading: rolesLoading } = useServiceAccountRoles();
  const [apiKeys, setApiKeys] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [expiresIn, setExpiresIn] = useState<string>('never');
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [copySuccess, setCopySuccess] = useState(false);
  const [createServiceAccount, setCreateServiceAccount] = useState(true);
  const [serviceAccountRole, setServiceAccountRole] = useState('standard_service');
  const [serviceAccounts, setServiceAccounts] = useState<any[]>([]);
  const [selectedServiceAccountId, setSelectedServiceAccountId] = useState<string>('');

  // Set default role when roles are loaded
  useEffect(() => {
    if (roles.length > 0 && serviceAccountRole === 'standard_service') {
      const defaultRole = roles.find(r => r.value === 'standard_service')?.value || roles[0].value;
      setServiceAccountRole(defaultRole);
    }
  }, [roles, serviceAccountRole]);

  useEffect(() => {
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }
    fetchAPIKeys();
    fetchServiceAccounts();
  }, [navigate]);

  const fetchServiceAccounts = async () => {
    try {
      const response = await api.listServiceAccounts();
      setServiceAccounts(response.service_accounts || []);
    } catch (err) {
      console.error('Failed to load service accounts:', err);
    }
  };

  const fetchAPIKeys = async () => {
    try {
      setLoading(true);
      const response = await api.listAPIKeys();
      setApiKeys(response.api_keys || []);
    } catch (err) {
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({ message: err instanceof Error ? err.message : 'Failed to load API keys' });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleCreateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setCreating(true);

    try {
      const expiresInDays = expiresIn === 'never' ? undefined : parseInt(expiresIn);
      const response = await api.createAPIKey({
        name: newKeyName,
        expires_in: expiresInDays,
        service_account_id: !createServiceAccount && selectedServiceAccountId ? selectedServiceAccountId : undefined,
        create_service_account: createServiceAccount,
        service_account_role: createServiceAccount ? serviceAccountRole : undefined,
      });

      setCreatedKey(response.key);
      setNewKeyName('');
      setExpiresIn('never');
      setCreateServiceAccount(true);
      setSelectedServiceAccountId('');
      setServiceAccountRole('developer');
      await fetchAPIKeys();
    } catch (err) {
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({ message: err instanceof Error ? err.message : 'Failed to create API key' });
      }
    } finally {
      setCreating(false);
    }
  };

  const handleRevokeKey = async (keyId: string, keyName: string) => {
    if (!confirm(`Are you sure you want to revoke "${keyName}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.revokeAPIKey(keyId);
      await fetchAPIKeys();
    } catch (err) {
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({ message: err instanceof Error ? err.message : 'Failed to revoke API key' });
      }
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopySuccess(true);
    setTimeout(() => setCopySuccess(false), 2000);
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Never';
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  };

  return (
    <AppLayout>
      <div className="p-8">
        {/* Page Header */}
        <div className="mb-8">
          <h1 className="text-2xl font-bold mb-2">API Keys</h1>
          <p className="text-gray-600">
            Manage your API keys for authenticating with the Threadify API
          </p>
        </div>

        {/* Copy Success Notification */}
        {copySuccess && (
          <div className="fixed top-4 right-4 bg-green-600 text-white px-6 py-3 rounded shadow-lg z-50 flex items-center gap-2">
            <Check className="w-4 h-4" /> Copied to clipboard!
          </div>
        )}

        {/* Error Message */}
        {error && (
          <Alert
            type="error"
            message={error.message}
            details={error.details}
            className="mb-6"
            action={
              isCreditError(error.message)
                ? { label: 'Go to Billing', onClick: () => navigate('/u/settings?tab=billing'), variant: 'primary' }
                : undefined
            }
          />
        )}

        {/* Create Button */}
        <div className="mb-6">
          <button
            onClick={() => setShowCreateModal(true)}
            className="px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors rounded"
          >
            + Create New API Key
          </button>
        </div>

        {/* API Keys List */}
        {loading ? (
          <div className="text-center py-12">
            <p className="text-gray-500">Loading API keys...</p>
          </div>
        ) : apiKeys.length === 0 ? (
          <div className="text-center py-16 border border-gray-200 rounded">
            <div className="flex justify-center mb-4">
              <div className="rounded-full bg-gray-50 p-4">
                <Key className="w-12 h-12 text-gray-400 stroke-[1.5]" />
              </div>
            </div>
            <h3 className="text-xl font-bold mb-2">No API keys yet</h3>
            <p className="text-gray-600 mb-6">
              Create your first API key to start using the Threadify API
            </p>
            <button
              onClick={() => setShowCreateModal(true)}
              className="px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors rounded"
            >
              Create Your First API Key
            </button>
          </div>
        ) : (
          <div className="border border-gray-200 rounded overflow-hidden">
            <table className="w-full">
              <thead className="border-b border-gray-200 bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-sm font-semibold text-gray-700">Name</th>
                  <th className="px-6 py-3 text-left text-sm font-semibold text-gray-700">Key</th>
                  <th className="px-6 py-3 text-left text-sm font-semibold text-gray-700">Last Used</th>
                  <th className="px-6 py-3 text-left text-sm font-semibold text-gray-700">Expires</th>
                  <th className="px-6 py-3 text-left text-sm font-semibold text-gray-700">Created</th>
                  <th className="px-6 py-3 text-right text-sm font-semibold text-gray-700">Actions</th>
                </tr>
              </thead>
              <tbody>
                {apiKeys.map((key, index) => (
                  <tr
                    key={key.id}
                    className={`hover:bg-gray-50 transition-colors ${
                      index !== apiKeys.length - 1 ? 'border-b border-gray-200' : ''
                    }`}
                  >
                    <td className="px-6 py-4 font-medium text-gray-900">{key.name}</td>
                    <td className="px-6 py-4">
                      <code className="text-sm bg-gray-100 px-2 py-1 rounded font-mono text-gray-700">
                        {key.key_prefix}...
                      </code>
                    </td>
                    <td className="px-6 py-4 text-sm text-gray-600">
                      {formatDate(key.last_used_at)}
                    </td>
                    <td className="px-6 py-4 text-sm text-gray-600">
                      {formatDate(key.expires_at)}
                    </td>
                    <td className="px-6 py-4 text-sm text-gray-600">
                      {formatDate(key.created_at)}
                    </td>
                    <td className="px-6 py-4 text-right">
                      <button
                        onClick={() => handleRevokeKey(key.id, key.name)}
                        className="px-4 py-2 border border-red-600 text-red-600 hover:bg-red-600 hover:text-white transition-colors text-sm font-medium rounded"
                      >
                        Revoke
                      </button>
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
              <h2 className="text-2xl font-bold mb-6">
                Create API Key
              </h2>

              {createdKey ? (
                <div>
                  <div className="mb-6 p-4 border border-green-600 bg-green-50 rounded">
                    <p className="text-green-800 font-semibold mb-2 flex items-center gap-1">
                      <Check className="w-4 h-4" /> API Key Created!
                    </p>
                    <p className="text-sm text-green-700 mb-4">
                      Copy this key now. You won't be able to see it again.
                    </p>
                    <div className="flex gap-2">
                      <code className="flex-1 bg-white border border-gray-300 px-3 py-2 text-sm break-all rounded font-mono">
                        {createdKey}
                      </code>
                      <button
                        onClick={() => copyToClipboard(createdKey)}
                        className="px-4 py-2 bg-black text-white hover:bg-gray-800 transition-colors whitespace-nowrap rounded"
                      >
                        Copy
                      </button>
                    </div>
                  </div>
                  <button
                    onClick={() => {
                      setCreatedKey(null);
                      setShowCreateModal(false);
                    }}
                    className="w-full px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors rounded"
                  >
                    Done
                  </button>
                </div>
              ) : (
                <form onSubmit={handleCreateKey}>
                  <div className="mb-4">
                    <label htmlFor="name" className="block text-sm font-medium text-gray-700 mb-2">
                      Key Name <span className="text-red-600">*</span>
                    </label>
                    <input
                      id="name"
                      type="text"
                      required
                      value={newKeyName}
                      onChange={(e) => setNewKeyName(e.target.value)}
                      className="w-full px-4 py-3 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent"
                      placeholder="Production API Key"
                    />
                  </div>

                  <div className="mb-4">
                    <label htmlFor="expires" className="block text-sm font-medium text-gray-700 mb-2">
                      Expires In
                    </label>
                    <select
                      id="expires"
                      value={expiresIn}
                      onChange={(e) => setExpiresIn(e.target.value)}
                      className="w-full px-4 py-3 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent bg-white"
                    >
                      <option value="never">Never</option>
                      <option value="30">30 days</option>
                      <option value="90">90 days</option>
                      <option value="180">180 days</option>
                      <option value="365">1 year</option>
                    </select>
                  </div>

                  <div className="mb-4 p-4 border border-gray-200 rounded bg-gray-50">
                    <div className="flex items-center mb-3">
                      <input
                        type="checkbox"
                        id="createServiceAccount"
                        checked={createServiceAccount}
                        onChange={(e) => setCreateServiceAccount(e.target.checked)}
                        className="w-4 h-4 text-black border-gray-300 rounded focus:ring-black"
                      />
                      <label htmlFor="createServiceAccount" className="ml-2 text-sm font-medium text-gray-700">
                        Create new service account
                      </label>
                    </div>
                    <p className="text-xs text-gray-600 mb-3">
                      Service accounts provide role-based access control for your API keys
                    </p>
                    
                    {!createServiceAccount && (
                      <div className="mb-3">
                        <label htmlFor="existingServiceAccount" className="block text-sm font-medium text-gray-700 mb-2">
                          Select Existing Service Account
                        </label>
                        <select
                          id="existingServiceAccount"
                          value={selectedServiceAccountId}
                          onChange={(e) => setSelectedServiceAccountId(e.target.value)}
                          className="w-full px-4 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent bg-white text-sm"
                          required={!createServiceAccount}
                        >
                          <option value="">Select a service account...</option>
                          {serviceAccounts.map((sa) => (
                            <option key={sa.id} value={sa.id}>
                              {sa.name} {!sa.is_active ? '(Inactive)' : ''}
                            </option>
                          ))}
                        </select>
                      </div>
                    )}
                    
                    {createServiceAccount && (
                      <div>
                        <label htmlFor="role" className="block text-sm font-medium text-gray-700 mb-2">
                          Service Account Role
                        </label>
                        <select
                          id="role"
                          value={serviceAccountRole}
                          onChange={(e) => setServiceAccountRole(e.target.value)}
                          className="w-full px-4 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent bg-white text-sm"
                          disabled={rolesLoading}
                        >
                          {rolesLoading ? (
                            <option>Loading roles...</option>
                          ) : roles.length > 0 ? (
                            roles.map((role) => (
                              <option key={role.value} value={role.value}>
                                {role.label} - {role.description}
                              </option>
                            ))
                          ) : (
                            <option value="standard_service">Standard Service - Default role</option>
                          )}
                        </select>
                      </div>
                    )}
                  </div>

                  <div className="flex gap-4">
                    <button
                      type="button"
                      onClick={() => {
                        setShowCreateModal(false);
                        setNewKeyName('');
                        setExpiresIn('never');
                      }}
                      className="flex-1 px-6 py-3 border border-gray-300 hover:bg-gray-100 transition-colors font-medium rounded"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      disabled={creating}
                      className="flex-1 px-6 py-3 bg-black text-white font-medium hover:bg-gray-800 transition-colors disabled:opacity-50 rounded"
                    >
                      {creating ? 'Creating...' : 'Create Key'}
                    </button>
                  </div>
                </form>
              )}
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}
