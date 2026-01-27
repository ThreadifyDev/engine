import { useState } from 'react';
import { useNavigate, useParams } from '@remix-run/react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '~/lib/api';
import SideNav from '~/components/SideNav';
import YamlEditor from '~/components/YamlEditor';

export default function ContractDetail() {
  const navigate = useNavigate();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [updateYaml, setUpdateYaml] = useState('');
  const [updating, setUpdating] = useState(false);
  const [updateError, setUpdateError] = useState('');

  // Check authentication
  const token = api.getStoredToken();
  if (!token) {
    navigate('/auth/login');
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

    try {
      await api.updateContract(id!, { yaml: updateYaml });
      // Refresh the contract data
      queryClient.invalidateQueries({ queryKey: ['contract', id, 'versions'] });
      setShowUpdateModal(false);
      setUpdateYaml('');
    } catch (err) {
      setUpdateError(err instanceof Error ? err.message : 'Failed to update contract');
    } finally {
      setUpdating(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex h-screen bg-white">
        <SideNav />
        <div className="flex-1 flex items-center justify-center ml-64">
          <div className="text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-black"></div>
            <p className="mt-4 text-black">Loading contract...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-screen bg-white">
        <SideNav />
        <div className="flex-1 flex items-center justify-center ml-64">
          <div className="text-center">
            <p className="text-red-600 mb-4">
              {error instanceof Error ? error.message : 'Failed to load contract'}
            </p>
            <button
              onClick={() => navigate('/contracts')}
              className="px-4 py-2 bg-black text-white hover:bg-gray-800"
            >
              Back to Contracts
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-screen bg-white">
      <SideNav />
      <div className="flex-1 overflow-auto ml-64">
        <div className="p-8">
          {/* Header */}
          <div className="mb-8">
            <button
              onClick={() => navigate('/contracts')}
              className="text-black hover:underline mb-4 flex items-center"
            >
              ← Back to Contracts
            </button>
            <div className="flex items-center justify-between mb-4">
              <h1 className="text-4xl font-bold text-black" style={{ fontFamily: 'Block, sans-serif' }}>
                {contract?.name || id || 'Contract Details'}
              </h1>
              <button
                onClick={() => setShowUpdateModal(true)}
                className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium"
              >
                Update Contract
              </button>
            </div>
            
            {/* Contract Metadata */}
            <div className="grid grid-cols-2 gap-4 bg-gray-50 border-2 border-black p-6">
              <div>
                <p className="text-sm text-gray-600 mb-1">Description</p>
                <p className="text-black font-medium">
                  {contract?.description || 'No description provided'}
                </p>
              </div>
              <div>
                <p className="text-sm text-gray-600 mb-1">Created</p>
                <p className="text-black font-medium">
                  {contract?.createdAt ? new Date(contract.createdAt).toLocaleDateString('en-US', { 
                    year: 'numeric', 
                    month: 'long', 
                    day: 'numeric' 
                  }) : 'N/A'}
                </p>
              </div>
              <div>
                <p className="text-sm text-gray-600 mb-1">Latest Version</p>
                <p className="text-black font-medium">v{contract?.latestVersion || 'N/A'}</p>
              </div>
              <div>
                <p className="text-sm text-gray-600 mb-1">Total Versions</p>
                <p className="text-black font-medium">{contract?.versions?.length || 0}</p>
              </div>
            </div>
          </div>

          {/* Versions Section */}
          {contract?.versions && contract.versions.length > 0 && (
            <div>
              <h2 className="text-2xl font-bold text-black mb-4" style={{ fontFamily: 'Block, sans-serif' }}>
                Versions ({contract.versions.length})
              </h2>
              <div className="border-2 border-black">
                {contract.versions.map((version: any, index: number) => (
                  <div
                    key={version.version}
                    className={`flex items-center justify-between p-4 hover:bg-gray-50 transition-colors ${
                      index !== contract.versions.length - 1 ? 'border-b-2 border-black' : ''
                    }`}
                  >
                    <div className="flex items-center gap-4">
                      <span className="font-bold text-black text-lg">v{version.version}</span>
                      <span className="text-sm text-gray-600">
                        {version.createdAt ? new Date(version.createdAt).toLocaleDateString('en-US', {
                          month: 'short',
                          day: 'numeric',
                          year: 'numeric'
                        }) : 'N/A'}
                      </span>
                    </div>
                    <button
                      onClick={() => navigate(`/contracts/${id}/versions/${version.version}`)}
                      className="px-4 py-2 border-2 border-black hover:bg-black hover:text-white transition-colors text-sm font-medium"
                    >
                      View
                    </button>
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
                  Contract YAML
                </label>
                <YamlEditor
                  value={updateYaml}
                  onChange={setUpdateYaml}
                  placeholder="Paste your updated contract YAML here..."
                  height="500px"
                />
              </div>
              {updateError && (
                <div className="mb-4 p-3 bg-red-50 border-2 border-red-600 text-red-600">
                  {updateError}
                </div>
              )}
              <div className="flex gap-4">
                <button
                  type="submit"
                  disabled={updating}
                  className="flex-1 px-6 py-3 bg-black text-white hover:bg-gray-800 disabled:bg-gray-400 transition-colors font-medium"
                >
                  {updating ? 'Updating...' : 'Create New Version'}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setShowUpdateModal(false);
                    setUpdateYaml('');
                    setUpdateError('');
                  }}
                  className="flex-1 px-6 py-3 border-2 border-black hover:bg-gray-100 transition-colors font-medium"
                >
                  Cancel
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
