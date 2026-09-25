import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { ArrowRight, FileText, Layers3, Plus, Search, Trash2 } from 'lucide-react';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import ContractDraftEditor from '~/components/contracts/ContractDraftEditor';
import { useAgent } from '~/components/agent/agent-context';

const PAGE_SIZE = 20;

type ContractSummary = {
  id: string;
  name: string;
  description?: string;
  latestVersion?: number;
  createdAt?: string;
};

function displayDate(value?: string) {
  if (!value) return 'Date unavailable';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? 'Date unavailable'
    : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

export default function Contracts() {
  const navigate = useNavigate();
  const [contracts, setContracts] = useState<ContractSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [refresh, setRefresh] = useState(0);
  const { contractDraft, editContractDraft, setContractEditorOpen } = useAgent();

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    if (!api.isAuthenticated()) {
      navigate('/login');
      return;
    }
    let active = true;
    setLoading(true);
    setError('');
    api.getAllContracts({ search: debouncedSearch || undefined, limit: PAGE_SIZE, offset })
      .then(response => {
        if (!active) return;
        setContracts(response.contracts || []);
        setTotal(response.total || 0);
      })
      .catch(cause => {
        if (active) setError(cause instanceof Error ? cause.message : 'Failed to load contracts');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [debouncedSearch, navigate, offset, refresh]);

  const handleCreate = async () => {
    await api.createContract({ name: '', yaml: contractDraft.source });
    setContractEditorOpen(false);
    editContractDraft('');
    setOffset(0);
    setRefresh(value => value + 1);
  };

  const handleDelete = async (contract: ContractSummary) => {
    if (!window.confirm('Delete contract "' + contract.name + '"?')) return;
    try {
      await api.deleteContract(contract.id);
      if (contracts.length === 1 && offset > 0) setOffset(Math.max(0, offset - PAGE_SIZE));
      setRefresh(value => value + 1);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete contract');
    }
  };

  return (
    <AppLayout>
      <div className="min-h-screen bg-[#f8f8f6] px-4 py-7 sm:px-7 sm:py-10 lg:px-10">
        <div className="mx-auto max-w-6xl">
          {contractDraft.open ? <ContractDraftEditor onSave={handleCreate} /> : (
            <>
              <header className="mb-9 flex flex-wrap items-start justify-between gap-5">
                <div>
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Workflow library</p>
                  <h1 className="text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">Contracts</h1>
                  <p className="mt-3 max-w-xl text-sm leading-6 text-stone-500">Define workflow rules, compose reusable contracts, and publish versions your threads can rely on.</p>
                </div>
                <div className="flex items-center gap-2">
                  <button type="button" onClick={() => setContractEditorOpen(true)}
                    className="inline-flex items-center gap-2 rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-stone-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700">
                    <Plus className="h-4 w-4" /> New contract
                  </button>
                </div>
              </header>

              <section aria-label="Contract library" className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
                <div className="flex flex-wrap items-center justify-between gap-4 border-b border-stone-200 px-5 py-4 sm:px-6">
                  <div className="flex items-center gap-3">
                    <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-50 text-emerald-700"><Layers3 className="h-4 w-4" /></span>
                    <div>
                      <h2 className="text-sm font-semibold text-stone-900">Published contracts</h2>
                      <p className="text-xs text-stone-500">{total} {debouncedSearch ? (total === 1 ? 'matching contract' : 'matching contracts') : (total === 1 ? 'contract in this workspace' : 'contracts in this workspace')}</p>
                    </div>
                  </div>
                  <label className="relative block w-full sm:w-72">
                    <span className="sr-only">Search contracts</span>
                    <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-stone-400" />
                    <input value={search} onChange={event => { setSearch(event.target.value); setOffset(0); }}
                      placeholder="Search contracts"
                      className="w-full rounded-lg border border-stone-200 bg-stone-50 py-2.5 pl-9 pr-3 text-sm text-stone-900 placeholder:text-stone-400 focus:border-emerald-600 focus:bg-white focus:outline-none focus:ring-2 focus:ring-emerald-100" />
                  </label>
                </div>

                {error && <div role="alert" className="border-b border-red-100 bg-red-50 px-5 py-3 text-sm text-red-700 sm:px-6">{error}</div>}

                {loading ? (
                  <div role="status" className="space-y-3 p-6">
                    {[0, 1, 2].map(item => <div key={item} className="h-20 animate-pulse rounded-xl bg-stone-100" />)}
                    <span className="sr-only">Loading contracts</span>
                  </div>
                ) : contracts.length === 0 ? (
                  <div className="flex flex-col items-center px-6 py-16 text-center">
                    <span className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-stone-100 text-stone-500"><FileText className="h-6 w-6" /></span>
                    <h3 className="text-base font-semibold text-stone-900">{debouncedSearch ? 'No matching contracts' : 'Your library is ready'}</h3>
                    <p className="mt-2 max-w-sm text-sm leading-6 text-stone-500">{debouncedSearch ? 'Try a different name or clear your search.' : 'Create a contract to define the steps, owners, and rules for a workflow.'}</p>
                    {debouncedSearch ? (
                      <button type="button" onClick={() => setSearch('')} className="mt-5 text-sm font-medium text-emerald-700 hover:text-emerald-800">Clear search</button>
                    ) : (
                      <button type="button" onClick={() => setContractEditorOpen(true)} className="mt-6 inline-flex items-center gap-2 rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white hover:bg-stone-800"><Plus className="h-4 w-4" /> Create contract</button>
                    )}
                  </div>
                ) : (
                  <ul className="divide-y divide-stone-100">
                    {contracts.map(contract => (
                      <li key={contract.id} className="group flex items-center gap-2 px-3 py-2 transition-colors hover:bg-stone-50 sm:px-4">
                        <Link to={'/u/contracts/' + contract.id} className="flex min-w-0 flex-1 items-center gap-4 rounded-lg px-2 py-3 focus-visible:outline focus-visible:outline-2 focus-visible:outline-emerald-700 sm:px-3">
                          <span className="hidden h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-stone-200 bg-white text-stone-500 sm:flex"><FileText className="h-4 w-4" /></span>
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-sm font-semibold text-stone-900">{contract.name}</span>
                            <span className="mt-1 block truncate text-xs text-stone-500">{contract.description && contract.description !== contract.name ? contract.description : 'Published workflow contract'}</span>
                          </span>
                          <span className="hidden text-xs text-stone-500 md:block">Created {displayDate(contract.createdAt)}</span>
                          <span className="shrink-0 rounded-md border border-emerald-100 bg-emerald-50 px-2 py-1 text-xs font-semibold text-emerald-800">v{contract.latestVersion || 1}</span>
                          <ArrowRight className="h-4 w-4 shrink-0 text-stone-400 transition-transform group-hover:translate-x-0.5 group-hover:text-stone-700" />
                        </Link>
                        <button type="button" onClick={() => handleDelete(contract)} aria-label={'Delete ' + contract.name}
                          className="rounded-lg p-2 text-stone-400 hover:bg-red-50 hover:text-red-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-red-600">
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </li>
                    ))}
                  </ul>
                )}

                {!loading && total > PAGE_SIZE && (
                  <div className="flex items-center justify-between border-t border-stone-200 px-5 py-4 text-xs text-stone-500 sm:px-6">
                    <span>Showing {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}</span>
                    <div className="flex gap-2">
                      <button type="button" onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))} disabled={offset === 0} aria-label="Previous page" className="rounded-lg border border-stone-200 p-2 text-stone-700 hover:bg-stone-50 disabled:opacity-40">Previous</button>
                      <button type="button" onClick={() => setOffset(offset + PAGE_SIZE)} disabled={offset + PAGE_SIZE >= total} aria-label="Next page" className="rounded-lg border border-stone-200 p-2 text-stone-700 hover:bg-stone-50 disabled:opacity-40">Next</button>
                    </div>
                  </div>
                )}
              </section>
            </>
          )}
        </div>
      </div>
    </AppLayout>
  );
}
