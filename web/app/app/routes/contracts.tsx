import { useState, useEffect, useRef } from 'react';
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import SideNav from '~/components/SideNav';

export default function Contracts() {
  const navigate = useNavigate();
  const [contracts, setContracts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState({ name: '', yaml: '' });
  const [uploading, setUploading] = useState(false);
  const hasFetched = useRef(false);

  useEffect(() => {
    // Check authentication
    const token = api.getStoredToken();
    if (!token) {
      navigate('/auth/login');
      return;
    }

    // Prevent double fetch in React strict mode
    if (hasFetched.current) return;
    hasFetched.current = true;

    fetchContracts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Empty dependency array - only run once on mount

  const fetchContracts = async () => {
    try {
      setLoading(true);
      const response = await api.getAllContracts();
      setContracts(response.contracts || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load contracts');
    } finally {
      setLoading(false);
    }
  };

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault();
    setUploading(true);
    setError('');

    try {
      await api.createContract(uploadForm);
      setShowUploadModal(false);
      setUploadForm({ name: '', yaml: '' });
      fetchContracts();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload contract');
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this contract?')) return;

    try {
      await api.deleteContract(id);
      fetchContracts();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete contract');
    }
  };

  return (
    <div className="min-h-screen bg-white flex">
      <SideNav />

      {/* Main Content */}
      <div className="flex-1 ml-64 p-8">
        <div className="flex justify-between items-center mb-8">
          <div>
            <h2 className="text-2xl font-bold mb-2">Contracts</h2>
            <p className="text-gray-600">
              Manage your workflow contracts and validation rules
            </p>
          </div>
          <button
            onClick={() => setShowUploadModal(true)}
            className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium"
          >
            Upload Contract
          </button>
        </div>

        {error && (
          <div className="bg-black text-white px-4 py-3 mb-6">
            {error}
          </div>
        )}

        {loading ? (
          <div className="text-center py-12">
            <p className="text-gray-600">Loading contracts...</p>
          </div>
        ) : contracts.length === 0 ? (
          <div className="border-4 border-black p-12 text-center">
            <h3 className="text-xl font-bold mb-2">No contracts yet</h3>
            <p className="text-gray-600 mb-6">
              Upload your first contract to start validating workflows
            </p>
            <button
              onClick={() => setShowUploadModal(true)}
              className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium"
            >
              Upload Contract
            </button>
          </div>
        ) : (
          <div className="grid gap-4">
            {contracts.map((contract) => (
              <div
                key={contract.id}
                className="border-4 border-black p-6 hover:bg-gray-50 transition-colors"
              >
                <div className="flex justify-between items-start">
                  <div className="flex-1">
                    <h3 className="text-xl font-bold mb-2">{contract.name}</h3>
                    <div className="flex gap-4 text-sm text-gray-600">
                      <span>Version: {contract.latest_version || 'v1'}</span>
                      <span>
                        Created: {
                          contract.created_at 
                            ? new Date(contract.created_at).toLocaleDateString()
                            : 'N/A'
                        }
                      </span>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={() => navigate(`/contracts/${contract.id}`)}
                      className="px-4 py-2 border-2 border-black hover:bg-black hover:text-white transition-colors font-medium"
                    >
                      View
                    </button>
                    <button
                      onClick={() => handleDelete(contract.id)}
                      className="px-4 py-2 border-2 border-black hover:bg-black hover:text-white transition-colors font-medium"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Upload Modal */}
      {showUploadModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50">
          <div className="bg-white border-4 border-black max-w-3xl w-full max-h-[90vh] overflow-y-auto">
            <div className="border-b-4 border-black p-6 flex justify-between items-center">
              <h2 className="text-2xl font-bold">Upload Contract</h2>
              <button
                onClick={() => setShowUploadModal(false)}
                className="text-2xl font-bold hover:text-gray-600"
              >
                ×
              </button>
            </div>

            <form onSubmit={handleUpload} className="p-6">
              <div className="mb-6">
                <label className="block text-sm font-medium mb-2">
                  Contract Name <span className="text-red-600">*</span>
                </label>
                <input
                  type="text"
                  required
                  value={uploadForm.name}
                  onChange={(e) => setUploadForm({ ...uploadForm, name: e.target.value })}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black"
                  placeholder="e.g., order_fulfillment"
                />
              </div>

              <div className="mb-6">
                <label className="block text-sm font-medium mb-2">
                  YAML Contract <span className="text-red-600">*</span>
                </label>
                <textarea
                  required
                  value={uploadForm.yaml}
                  onChange={(e) => setUploadForm({ ...uploadForm, yaml: e.target.value })}
                  className="w-full px-4 py-3 border-2 border-black focus:outline-none focus:ring-2 focus:ring-black font-mono text-sm"
                  rows={20}
                  placeholder="Paste your YAML contract here..."
                />
                <p className="text-sm text-gray-600 mt-2">
                  Define your workflow steps, transitions, and validation rules in YAML format
                </p>
              </div>

              <div className="flex gap-4">
                <button
                  type="submit"
                  disabled={uploading}
                  className="flex-1 px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {uploading ? 'Uploading...' : 'Upload Contract'}
                </button>
                <button
                  type="button"
                  onClick={() => setShowUploadModal(false)}
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
  );
}
