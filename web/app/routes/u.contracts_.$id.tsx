import { useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ValidationError } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import YamlEditor from '~/components/YamlEditor';
import Alert from '~/components/Alert';


export default function ContractDetail() {
  const navigate = useNavigate();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [updateYaml, setUpdateYaml] = useState('');
  const [updating, setUpdating] = useState(false);
  const [updateError, setUpdateError] = useState('');
  const [updateErrorDetails, setUpdateErrorDetails] = useState<Array<{ field: string; message: string }>>([]);

  // Check authentication
  const token = api.isAuthenticated();
  if (!token) {
    navigate('/login');
    return null;
  }

  const { data: versionsResponse, isLoading, error } = useQuery({
    queryKey: ['contract', id, 'versions'],
    queryFn: () => api.getContractVersions(id!),
    enabled: !!id,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });

  const contract = versionsResponse ? {
    id: versionsResponse.contractId,
    name: versionsResponse.name,
    description: versionsResponse.description,
    latestVersion: versionsResponse.latestVersion,
    createdAt: versionsResponse.createdAt,
    updatedAt: versionsResponse.updatedAt,
    versions: versionsResponse.versions || []
  } : null;

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    setUpdating(true);
    setUpdateError('');
    setUpdateErrorDetails([]);

    try {
      await api.updateContract(id!, { yaml: updateYaml });
      // Refresh the contract data
      queryClient.invalidateQueries({ queryKey: ['contract', id, 'versions'] });
      setShowUpdateModal(false);
      setUpdateYaml('');
    } catch (err) {
      if (err instanceof ValidationError) {
        setUpdateError(err.message);
        setUpdateErrorDetails(err.details || []);
      } else {
        setUpdateError(err instanceof Error ? err.message : 'Failed to update contract');
      }
    } finally {
      setUpdating(false);
    }
  };

  if (isLoading) {
    return (
      <AppLayout>
        <div className="min-h-[60vh] flex items-center justify-center">
          <div className="text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-black"></div>
            <p className="mt-4 text-black">Loading contract...</p>
          </div>
        </div>
      </AppLayout>
    );
  }

  if (error) {
    return (
      <AppLayout>
        <div className="min-h-[60vh] flex items-center justify-center">
          <div className="text-center">
            <p className="text-red-600 mb-4">
              {error instanceof Error ? error.message : 'Failed to load contract'}
            </p>
            <button
              onClick={() => navigate('/u/contracts')}
              className="text-gray-500 hover:text-gray-700 font-medium transition-colors text-sm"
            >
              Back to Contracts
            </button>
          </div>
        </div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div className="overflow-auto">
        <div className="min-w-0 p-4 sm:p-6 lg:p-8">
          {/* Header */}
          <div className="mb-12">
            <button
              onClick={() => navigate('/u/contracts')}
              className="text-gray-600 hover:text-gray-900 mb-6 flex items-center text-sm"
            >
              ← Back to Contracts
            </button>
            <div className="flex items-center justify-between mb-6">
              <div>
                <h1 className="text-2xl font-bold text-gray-900 mb-2">
                  {contract?.name || id || 'Contract Details'}
                </h1>
                <p className="text-gray-600">
                  View contract versions and validation rules
                </p>
              </div>
              <button
                onClick={() => setShowUpdateModal(true)}
                className="px-4 py-2 bg-gray-900 text-white rounded hover:bg-gray-800 transition-colors text-sm font-medium"
              >
                Update Contract
              </button>
            </div>
            
            {/* Contract Metadata */}
            <div className="grid grid-cols-4 gap-8 py-6 border-b border-gray-200">
              <div>
                <p className="text-xs text-gray-500 mb-1">Latest Version</p>
                <p className="text-sm text-gray-900 font-medium">v{contract?.latestVersion || 'N/A'}</p>
              </div>
              <div>
                <p className="text-xs text-gray-500 mb-1">Total Versions</p>
                <p className="text-sm text-gray-900 font-medium">{contract?.versions?.length || 0}</p>
              </div>
              <div>
                <p className="text-xs text-gray-500 mb-1">Created</p>
                <p className="text-sm text-gray-900 font-medium">
                  {contract?.createdAt ? new Date(contract.createdAt).toLocaleDateString('en-US', { 
                    month: 'short',
                    day: 'numeric',
                    year: 'numeric'
                  }) : 'N/A'}
                </p>
              </div>
              <div>
                <p className="text-xs text-gray-500 mb-1">Description</p>
                <p className="text-sm text-gray-900 font-medium">
                  {contract?.description || 'No description'}
                </p>
              </div>
            </div>
          </div>

          {/* Versions Section */}
          {contract?.versions && contract.versions.length > 0 && (
            <div>
              <h2 className="text-lg font-semibold text-gray-900 mb-4">
                Versions
              </h2>
              <div className="space-y-0">
                {contract.versions.map((version: any) => (
                  <div
                    key={version.version}
                    onClick={() => navigate(`/u/contracts/${id}/versions/${version.version}`)}
                    className="flex items-center justify-between py-4 border-b border-gray-200 hover:bg-gray-50 cursor-pointer transition-colors px-2 -mx-2"
                  >
                    <div className="flex items-center gap-6">
                      <span className="font-medium text-gray-900">v{version.version}</span>
                      <span className="text-sm text-gray-500">
                        {version.createdAt ? new Date(version.createdAt).toLocaleDateString('en-US', {
                          month: 'short',
                          day: 'numeric',
                          year: 'numeric'
                        }) : 'N/A'}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Update Contract Modal */}
      {showUpdateModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white border-4 border-black p-8 max-w-2xl w-full max-h-[80vh] overflow-y-auto">
            <h2 className="text-2xl font-bold text-black mb-4" style={{ fontFamily: 'Block, sans-serif' }}>
              Update Contract - Create New Version
            </h2>
            <p className="text-gray-600 mb-4">
              Current version: v{contract?.latestVersion}. The new version will be v{(contract?.latestVersion || 0) + 1}.
            </p>
            <form onSubmit={handleUpdate}>
              <div className="mb-4">
                <label className="block text-black font-medium mb-2">
                  Contract source
                </label>
                <YamlEditor contractSource
                  value={updateYaml}
                  onChange={setUpdateYaml}
                  placeholder="Paste your updated Gherkin-style contract here..."
                  height="500px"
                />
              </div>
              {updateError && (
                <Alert 
                  type="error" 
                  message={updateError} 
                  details={updateErrorDetails} 
                  className="mb-4"
                />
              )}
                <div className="flex justify-end items-center gap-6 mt-2">
                  <button
                    type="button"
                    onClick={() => {
                      setShowUpdateModal(false);
                      setUpdateYaml('');
                      setUpdateError('');
                      setUpdateErrorDetails([]);
                    }}
                    className="text-red-700 hover:text-red-800 font-medium transition-colors text-sm"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={updating}
                    className="px-8 py-3 bg-black rounded-xl text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50"
                  >
                    {updating ? 'Updating...' : 'Create New Version'}
                  </button>
                </div>
            </form>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
