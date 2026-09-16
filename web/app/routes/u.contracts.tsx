import { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router';
import { Trash2, Search, X, ChevronLeft, ChevronRight } from 'lucide-react';
import { api, ValidationError } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import YamlEditor from '~/components/YamlEditor';

const PAGE_SIZE = 20;


export default function Contracts() {
  const navigate = useNavigate();
  const [contracts, setContracts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState({ name: '', yaml: '' });
  const [uploading, setUploading] = useState(false);
  const hasFetched = useRef(false);

  useEffect(() => {
    // Check authentication
    const token = api.isAuthenticated();
    if (!token) {
      navigate('/login');
      return;
    }

    fetchContracts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [offset]);

  const fetchContracts = async () => {
    try {
      setLoading(true);
      const response = await api.getAllContracts({
        limit: PAGE_SIZE,
        offset: offset
      });
      setContracts(response.contracts || []);
      setTotal(response.total || 0);
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
      setError(err instanceof ValidationError && err.details?.length
        ? err.details.map(detail => `${detail.field}: ${detail.message}`).join('\n')
        : err instanceof Error ? err.message : 'Failed to upload contract');
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
    <AppLayout>
      <div className="min-w-0 p-4 sm:p-6 lg:p-8">
        <div className="flex flex-wrap justify-between items-center gap-4 mb-8">
          <div>
            <h2 className="text-2xl font-bold mb-2">Contracts</h2>
            <p className="text-gray-600">
              Enforce your service-delivery workflow as contracts
            </p>
          </div>
          <button
            onClick={() => setShowUploadModal(true)}
            className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
          >
            Upload Contract
          </button>
        </div>

        {error && (
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg mb-6">
            {error}
          </div>
        )}

        {loading ? (
          <div className="text-center py-12">
            <p className="text-gray-600">Loading contracts...</p>
          </div>
        ) : contracts.length === 0 ? (
          <div className="border border-gray-200 rounded-lg p-12 text-center bg-white shadow-sm">
            <h3 className="text-xl font-bold text-gray-900 mb-2">No contracts yet</h3>
            <p className="text-gray-600 mb-6">
              Upload your first contract to start validating workflows
            </p>
            <button
              onClick={() => setShowUploadModal(true)}
              className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
            >
              Upload Contract
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4">
            {contracts.map((contract) => (
              <div
                key={contract.id}
                onClick={() => navigate(`/u/contracts/${contract.id}`)}
                className="bg-white border-b border-gray-200 py-5 px-2 sm:px-4 hover:bg-gray-50 transition-colors cursor-pointer"
              >
                <div className="flex min-w-0 justify-between items-start gap-3">
                  <div className="min-w-0 flex-1">
                    <h3 className="text-base font-semibold mb-2 [overflow-wrap:anywhere]">{contract.name}</h3>
                    <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-gray-600">
                      <span>Version: v{contract.latestVersion || 1}</span>
                      <span>
                        Created: {
                          contract.createdAt 
                            ? new Date(contract.createdAt).toLocaleDateString()
                            : 'N/A'
                        }
                      </span>
                    </div>
                  </div>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDelete(contract.id);
                    }}
                    className="shrink-0 p-2 text-red-500 hover:text-red-700 transition-colors"
                    title="Delete contract"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        {total > PAGE_SIZE && (
          <div className="mt-8 flex items-center justify-between border-t border-gray-200 pt-6">
            <div className="text-sm text-gray-500">
              Showing {offset + 1} to {Math.min(offset + PAGE_SIZE, total)} of {total} results
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                disabled={offset === 0}
                className="p-2 border border-gray-200 rounded hover:bg-gray-50 disabled:opacity-50 transition-colors"
              >
                <ChevronLeft className="w-4 h-4" />
              </button>
              <button
                onClick={() => setOffset(offset + PAGE_SIZE)}
                disabled={offset + PAGE_SIZE >= total}
                className="p-2 border border-gray-200 rounded hover:bg-gray-50 disabled:opacity-50 transition-colors"
              >
                <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}

        {/* Upload Modal */}
        {showUploadModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50 backdrop-blur-sm transition-all">
          <div className="bg-white rounded-2xl shadow-2xl max-w-3xl w-full max-h-[90vh] overflow-y-auto border-2 border-black">
            <div className="border-b-2 border-black p-6 flex justify-between items-center bg-white">
              <h2 className="text-xl font-bold text-gray-900">Upload Contract</h2>
              <button
                onClick={() => setShowUploadModal(false)}
                className="text-2xl font-bold hover:text-gray-600"
              >
                ×
              </button>
            </div>

            <form onSubmit={handleUpload} className="p-6">
              {error && <p role="alert" className="mb-4 whitespace-pre-wrap text-sm text-red-700">{error}</p>}
              <div className="mb-6">
                <label className="block text-sm font-medium mb-2">
                  Contract source <span className="text-red-600">*</span>
                </label>
                {!uploadForm.yaml && (
                  <button type="button" className="text-sm underline mb-2"
                    onClick={() => setUploadForm({ ...uploadForm, yaml: `Feature: payment_processing
Version: 1
Description: Record valid payments.

Rule: Validate a payment
  When step "charge" is submitted
  Then owner must be "payment_processor"
  And content "amount" must be a number greater than 0
  And content "currency" must be one of "GBP", "USD", "EUR"
  And this step is an entry point
  And this step is terminal
` })}>
                    Start with a Gherkin example
                  </button>
                )}
                <YamlEditor contractSource
                  value={uploadForm.yaml}
                  onChange={(value) => setUploadForm({ ...uploadForm, yaml: value })}
                  placeholder="Paste your Gherkin-style contract here..."
                  height="500px"
                />
                <p className="text-sm text-gray-600 mt-2">
                  Write Gherkin-style rules for steps, prerequisites, and content checks. YAML remains accepted temporarily.
                </p>
              </div>

              <div className="flex justify-end items-center gap-6 mt-2">
                <button
                  type="button"
                  onClick={() => setShowUploadModal(false)}
                  className="text-red-700 hover:text-red-800 font-medium transition-colors text-sm"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={uploading}
                  className="px-8 py-3 bg-black rounded-xl text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {uploading ? 'Uploading...' : 'Upload Contract'}
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
