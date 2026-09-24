import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { ArrowLeft, CalendarDays, GitBranch, Layers3 } from 'lucide-react';
import { load } from 'js-yaml';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import AgentToggleButton from '~/components/agent/AgentToggleButton';
import ContractSourceView from '~/components/contracts/ContractSourceView';

type IncludedContract = { name: string; version: number };

function includedContracts(source: string): IncludedContract[] {
  if (/^\s*Feature:/m.test(source)) {
    return Array.from(source.matchAll(/^\s*Include:\s*([A-Za-z0-9_]+):([1-9]\d*)\s*$/gm), match => ({
      name: match[1], version: Number(match[2]),
    }));
  }
  try {
    const parsed = load(source) as { includes?: unknown } | null;
    if (Array.isArray(parsed?.includes)) {
      return parsed.includes.filter((entry: any) => typeof entry?.name === 'string' && Number.isInteger(entry?.version));
    }
  } catch { /* Unrecognized source has no readable Include declarations. */ }
  return [];
}

function displayDate(value?: string) {
  if (!value) return 'Date unavailable';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? 'Date unavailable'
    : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

export default function ContractVersionDetail() {
  const navigate = useNavigate();
  const { id, version } = useParams();
  const [versionData, setVersionData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!api.isAuthenticated()) {
      navigate('/login');
      return;
    }
    if (!id || !version) return;
    let active = true;
    setLoading(true);
    setError('');
    api.getContractVersion(id, version)
      .then(response => { if (active) setVersionData(response); })
      .catch(cause => { if (active) setError(cause instanceof Error ? cause.message : 'Failed to load contract version'); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [id, navigate, version]);

  const source = versionData?.source ?? versionData?.yamlContent ?? '';
  const includes = includedContracts(source);
  const validation = versionData?.graph?.graph?.validation;

  return (
    <AppLayout hideDesktopHeader>
      <div className="min-h-screen bg-[#f8f8f6] px-4 py-7 sm:px-7 sm:py-10 lg:px-10">
        <div className="mx-auto max-w-6xl">
          <Link to={'/u/contracts/' + id} className="mb-8 inline-flex items-center gap-2 text-sm font-medium text-stone-500 hover:text-stone-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-emerald-700">
            <ArrowLeft className="h-4 w-4" /> Contract overview
          </Link>

          {loading ? (
            <div role="status" className="space-y-4">
              <div className="h-28 animate-pulse rounded-2xl bg-stone-200" />
              <div className="h-96 animate-pulse rounded-2xl bg-stone-200" />
              <span className="sr-only">Loading contract version</span>
            </div>
          ) : error ? (
            <div role="alert" className="rounded-xl border border-red-200 bg-white p-6 text-sm text-red-700">{error}</div>
          ) : (
            <>
              <header className="mb-8 flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0">
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Published version</p>
                  <h1 className="break-words text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">{versionData?.contractName || 'Contract'}</h1>
                  <div className="mt-4 flex flex-wrap items-center gap-2 text-xs text-stone-600">
                    <span className="inline-flex items-center gap-1.5 rounded-md border border-emerald-100 bg-emerald-50 px-2.5 py-1.5 font-semibold text-emerald-800"><GitBranch className="h-3.5 w-3.5" /> Version {version}</span>
                    <span className="inline-flex items-center gap-1.5 rounded-md border border-stone-200 bg-white px-2.5 py-1.5"><CalendarDays className="h-3.5 w-3.5" /> Published {displayDate(versionData?.createdAt)}</span>
                  </div>
                </div>
                <AgentToggleButton />
              </header>

              <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_260px]">
                <div className="min-w-0">
                  <ContractSourceView source={source} />
                </div>
                <aside className="space-y-4">
                  <section aria-labelledby="composition-heading" className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm shadow-stone-200/40">
                    <div className="mb-4 flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-50 text-emerald-700"><Layers3 className="h-4 w-4" /></div>
                    <h2 id="composition-heading" className="text-sm font-semibold text-stone-900">Composition</h2>
                    <p className="mt-1 text-xs leading-5 text-stone-500">{includes.length ? 'This version includes these published modules.' : 'This version defines its rules directly.'}</p>
                    {includes.length > 0 && (
                      <ul className="mt-4 space-y-2">
                        {includes.map((item, index) => (
                          <li key={item.name + ':' + item.version + ':' + index} className="flex min-w-0 items-center gap-2 rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 text-xs">
                            <span className="min-w-0 flex-1 break-all font-medium text-stone-800">{item.name}</span>
                            <span className="shrink-0 rounded bg-white px-1.5 py-0.5 font-semibold text-emerald-700">v{item.version}</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                  {validation?.MaxDuration && <section className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm shadow-stone-200/40">
                    <h2 className="text-sm font-semibold text-stone-900">Execution limit</h2>
                    <p className="mt-2 text-xs text-stone-500">Maximum duration <span className="font-semibold text-stone-800">{validation.MaxDuration}</span></p>
                  </section>}
                </aside>
              </div>
            </>
          )}
        </div>
      </div>
    </AppLayout>
  );
}
