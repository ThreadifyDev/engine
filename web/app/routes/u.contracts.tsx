import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Trash2, Plus, ChevronLeft, ChevronRight } from 'lucide-react';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import ContractDraftEditor from '~/components/contracts/ContractDraftEditor';
import { useAgent } from '~/components/agent/agent-context';

const PAGE_SIZE = 20;


export default function Contracts() {
  const navigate = useNavigate();
  const [contracts, setContracts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const { contractDraft, editContractDraft, setContractEditorOpen } = useAgent();
  const showEditor = contractDraft.open;
  const setShowEditor = setContractEditorOpen;
  const uploadForm = { name: '', yaml: contractDraft.source };
  const setUploadForm = (value: { name: string; yaml: string }) => editContractDraft(value.yaml);

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

  const handleUpload = async () => {
    await api.createContract(uploadForm);
    setShowEditor(false);
    setUploadForm({ name: '', yaml: '' });
    await fetchContracts();
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
        {showEditor ? <ContractDraftEditor onSave={handleUpload} /> : (<>

        <div className="flex flex-wrap justify-between items-center gap-4 mb-8">
          <div>
            <h2 className="text-2xl font-bold mb-2">Contracts</h2>
            <p className="text-gray-600">
              Enforce your service-delivery workflow as contracts
            </p>
          </div>
          <button
            onClick={() => setShowEditor(true)}
            className="inline-flex items-center gap-2 rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700"
          >
            <Plus className="h-4 w-4" /> New contract
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
              onClick={() => setShowEditor(true)}
              className="inline-flex items-center gap-2 rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700"
            >
              <Plus className="h-4 w-4" /> New contract
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

        </>)}
      </div>
    </AppLayout>
  );
}
