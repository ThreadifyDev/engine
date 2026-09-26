import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, CalendarDays, FileText, GitBranch, Layers3, Plus, X } from 'lucide-react';
import { api, ValidationError } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import ContractEditor from '~/components/ContractEditor';

type ContractVersion = { version: number; createdAt?: string };

function displayDate(value?: string) {
  if (!value) return 'Date unavailable';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? 'Date unavailable'
    : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

function prepareNextVersion(source: string, nextVersion: number, name: string) {
  if (/^\s*Feature:/m.test(source)) {
    return /^\s*Version:/m.test(source)
      ? source.replace(/^(\s*Version:\s*)\d+/m, (_match, prefix: string) => prefix + nextVersion)
      : source.replace(/^(\s*Feature:[^\n]*\n)/m, '$1Version: ' + nextVersion + '\n');
  }
  return `Feature: ${name}\nVersion: ${nextVersion}\n\n# Add your Rule blocks here.\n`;
}

export default function ContractDetail() {
  const navigate = useNavigate();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [updateSource, setUpdateSource] = useState('');
  const [loadingSource, setLoadingSource] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [updateError, setUpdateError] = useState('');
  const [updateErrorDetails, setUpdateErrorDetails] = useState<Array<{ field: string; message: string }>>([]);

  useEffect(() => {
    if (!api.isAuthenticated()) navigate('/login');
  }, [navigate]);

  const { data: contract, isLoading, error } = useQuery({
    queryKey: ['contract', id, 'versions'],
    queryFn: () => api.getContractVersions(id!),
    enabled: !!id && api.isAuthenticated(),
    staleTime: 5 * 60 * 1000,
  });
  const versions: ContractVersion[] = contract?.versions || [];
  const latestVersion = contract?.latestVersion || 0;

  const openUpdate = async () => {
    if (!id || !latestVersion) return;
    setShowUpdateModal(true);
    setLoadingSource(true);
    setUpdateError('');
    setUpdateErrorDetails([]);
    try {
      const current = await api.getContractVersion(id, String(latestVersion));
      const source = current?.source ?? '';
      setUpdateSource(prepareNextVersion(source, latestVersion + 1, contract?.name || 'workflow'));
    } catch (cause) {
      setUpdateError(cause instanceof Error ? cause.message : 'Could not load the current source. You can paste a new version below.');
    } finally {
      setLoadingSource(false);
    }
  };

  const closeUpdate = () => {
    setShowUpdateModal(false);
    setUpdateSource('');
    setUpdateError('');
    setUpdateErrorDetails([]);
  };

  const handleUpdate = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!id || !updateSource.trim() || updating) return;
    setUpdating(true);
    setUpdateError('');
    setUpdateErrorDetails([]);
    try {
      await api.updateContract(id, { source: updateSource });
      await queryClient.invalidateQueries({ queryKey: ['contract', id, 'versions'] });
      closeUpdate();
    } catch (cause) {
      if (cause instanceof ValidationError) {
        setUpdateError(cause.message);
        setUpdateErrorDetails(cause.details || []);
      } else {
        setUpdateError(cause instanceof Error ? cause.message : 'Failed to create a new version');
      }
    } finally {
      setUpdating(false);
    }
  };

  return (
    <AppLayout>
      <div className="min-h-screen bg-[#f8f8f6] px-4 py-7 sm:px-7 sm:py-10 lg:px-10">
        <div className="mx-auto max-w-6xl">
          <Link to="/u/contracts" className="mb-8 inline-flex items-center gap-2 text-sm font-medium text-stone-500 hover:text-stone-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-emerald-700">
            <ArrowLeft className="h-4 w-4" /> All contracts
          </Link>

          {isLoading ? (
            <div role="status" className="space-y-4">
              <div className="h-28 animate-pulse rounded-2xl bg-stone-200" />
              <div className="h-72 animate-pulse rounded-2xl bg-stone-200" />
              <span className="sr-only">Loading contract</span>
            </div>
          ) : error ? (
            <div role="alert" className="rounded-xl border border-red-200 bg-white p-6 text-sm text-red-700">
              {error instanceof Error ? error.message : 'Failed to load contract'}
            </div>
          ) : (
            <>
              <header className="mb-8 flex flex-wrap items-start justify-between gap-5">
                <div className="min-w-0">
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Contract overview</p>
                  <h1 className="break-words text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">{contract?.name || id}</h1>
                  <p className="mt-3 max-w-2xl text-sm leading-6 text-stone-500">{contract?.description && contract.description !== contract.name ? contract.description : 'Versioned rules for this workflow.'}</p>
                </div>
                <div className="flex items-center gap-2">
                  <button type="button" onClick={openUpdate} disabled={!latestVersion}
                    className="inline-flex items-center gap-2 rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white shadow-sm hover:bg-stone-800 disabled:opacity-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700">
                    <Plus className="h-4 w-4" /> New version
                  </button>
                </div>
              </header>

              <section aria-label="Contract information" className="mb-8 grid gap-px overflow-hidden rounded-2xl border border-stone-200 bg-stone-200 shadow-sm shadow-stone-200/40 sm:grid-cols-3">
                <div className="bg-white p-5 sm:p-6">
                  <div className="mb-5 flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-50 text-emerald-700"><GitBranch className="h-4 w-4" /></div>
                  <p className="text-xs text-stone-500">Latest version</p>
                  <p className="mt-1 text-xl font-semibold tracking-tight text-stone-900">v{latestVersion || '—'}</p>
                </div>
                <div className="bg-white p-5 sm:p-6">
                  <div className="mb-5 flex h-9 w-9 items-center justify-center rounded-lg bg-stone-100 text-stone-600"><Layers3 className="h-4 w-4" /></div>
                  <p className="text-xs text-stone-500">Published versions</p>
                  <p className="mt-1 text-xl font-semibold tracking-tight text-stone-900">{versions.length}</p>
                </div>
                <div className="bg-white p-5 sm:p-6">
                  <div className="mb-5 flex h-9 w-9 items-center justify-center rounded-lg bg-stone-100 text-stone-600"><CalendarDays className="h-4 w-4" /></div>
                  <p className="text-xs text-stone-500">Created</p>
                  <p className="mt-1 text-xl font-semibold tracking-tight text-stone-900">{displayDate(contract?.createdAt)}</p>
                </div>
              </section>

              <section aria-labelledby="versions-heading" className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
                <div className="flex items-center justify-between border-b border-stone-200 px-5 py-4 sm:px-6">
                  <div>
                    <h2 id="versions-heading" className="text-sm font-semibold text-stone-900">Version history</h2>
                    <p className="mt-1 text-xs text-stone-500">Each published version keeps its own contract source and rules.</p>
                  </div>
                  <span className="rounded-md bg-stone-100 px-2 py-1 text-xs font-medium text-stone-600">{versions.length} total</span>
                </div>
                {versions.length ? (
                  <ul className="divide-y divide-stone-100">
                    {versions.map(version => (
                      <li key={version.version}>
                        <Link to={'/u/contracts/' + id + '/versions/' + version.version}
                          className="group flex items-center gap-4 px-5 py-4 transition-colors hover:bg-stone-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-inset focus-visible:outline-emerald-700 sm:px-6">
                          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-stone-200 bg-stone-50 text-stone-600"><FileText className="h-4 w-4" /></span>
                          <span className="min-w-0 flex-1">
                            <span className="flex flex-wrap items-center gap-2 text-sm font-semibold text-stone-900">Version {version.version}
                              {version.version === latestVersion && <span className="rounded-md bg-emerald-50 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-700">Latest</span>}
                            </span>
                            <span className="mt-1 block text-xs text-stone-500">Published {displayDate(version.createdAt)}</span>
                          </span>
                          <ArrowRight className="h-4 w-4 shrink-0 text-stone-400 transition-transform group-hover:translate-x-0.5 group-hover:text-stone-700" />
                        </Link>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="px-6 py-10 text-sm text-stone-500">No versions are available for this contract.</p>
                )}
              </section>
            </>
          )}
        </div>
      </div>

      {showUpdateModal && (
        <div role="dialog" aria-modal="true" aria-labelledby="update-contract-title" className="fixed inset-0 z-[100] flex items-center justify-center bg-stone-950/50 p-4">
          <div className="flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-2xl">
            <div className="flex items-start justify-between gap-4 border-b border-stone-200 px-5 py-4 sm:px-6">
              <div>
                <p className="mb-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-emerald-700">New version</p>
                <h2 id="update-contract-title" className="text-lg font-semibold text-stone-900">Update {contract?.name}</h2>
                <p className="mt-1 text-xs text-stone-500">Review the source for v{latestVersion + 1}, then publish it.</p>
              </div>
              <button type="button" onClick={closeUpdate} disabled={updating} aria-label="Close update dialog" className="rounded-lg p-2 text-stone-500 hover:bg-stone-100"><X className="h-4 w-4" /></button>
            </div>
            <form onSubmit={handleUpdate} className="min-h-0 overflow-y-auto">
              <div className="px-5 py-5 sm:px-6">
                <label className="mb-2 block text-xs font-semibold text-stone-700">Contract source</label>
                {loadingSource ? <div role="status" className="flex h-72 items-center justify-center rounded-xl bg-stone-50 text-sm text-stone-500">Loading current source…</div> :
                  <ContractEditor appearance="soft" value={updateSource} onChange={setUpdateSource} placeholder="Paste the next contract version here…" height="min(48vh, 480px)" />}
                {updateError && <div role="alert" className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                  <p>{updateError}</p>
                  {updateErrorDetails.map((detail, index) => <p key={index} className="mt-1">{detail.field}: {detail.message}</p>)}
                </div>}
              </div>
              <div className="flex items-center justify-end gap-3 border-t border-stone-200 bg-stone-50 px-5 py-4 sm:px-6">
                <button type="button" onClick={closeUpdate} disabled={updating} className="rounded-lg border border-stone-200 bg-white px-4 py-2.5 text-sm font-medium text-stone-700 hover:bg-stone-50">Cancel</button>
                <button type="submit" disabled={loadingSource || updating || !updateSource.trim()} className="rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white hover:bg-stone-800 disabled:opacity-50">{updating ? 'Publishing…' : 'Publish v' + (latestVersion + 1)}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
