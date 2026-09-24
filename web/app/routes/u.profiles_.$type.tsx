import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams, Link } from 'react-router';
import AppLayout from '~/components/AppLayout';
import { api } from '~/lib/api';
import type { EntityProfileType, MetricsTemplateResponse } from '~/lib/api';
import { graphqlClient, type EntityProfileListItem } from '~/lib/graphql';
import { ChevronLeft, ChevronRight, UserCircle, Search, X, Activity, Loader2, Settings, Edit2, SlidersHorizontal } from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';
import { ProfileTypeModal } from '~/components/profiles/ProfileTypeModal';


const PAGE_SIZE = 20;

const metricBadgeStyles = [
  'border-emerald-200 bg-emerald-50 text-emerald-700',
  'border-rose-200 bg-rose-50 text-rose-700',
  'border-amber-200 bg-amber-50 text-amber-700',
  'border-sky-200 bg-sky-50 text-sky-700',
  'border-violet-200 bg-violet-50 text-violet-700',
];

export default function EntityProfilesByType() {
  const { type } = useParams();
  const navigate = useNavigate();

  const [items, setItems] = useState<EntityProfileListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [profileType, setProfileType] = useState<EntityProfileType | null>(null);
  
  // Modal & Edit State
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [metricsTemplates, setMetricsTemplates] = useState<MetricsTemplateResponse[]>([]);
  
  const [offset, setOffset] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Search: two values — `search` is what the user types, `committed` is what
  // gets sent to the backend (debounced).
  const [search, setSearch] = useState('');
  const [committed, setCommitted] = useState('');

  useEffect(() => {
    const token = api.isAuthenticated();
    if (!token) navigate('/login');
  }, [navigate]);

  useEffect(() => {
    api.listMetricsTemplates()
      .then(res => setMetricsTemplates(res.data))
      .catch(console.error);
  }, []);

  // Only commit search when user triggers it
  const handleSearch = () => {
    setCommitted(search.trim());
    setOffset(0);
  };

  const handleClear = () => {
    setSearch('');
    setCommitted('');
    setOffset(0);
  };

  const fetchProfiles = async () => {
    if (!type) return;
    try {
      setIsLoading(true);
      setError(null);
      const res = await graphqlClient.getEntityProfilesByType({
        type,
        search: committed || undefined,
        limit: PAGE_SIZE,
        offset,
      });

      setItems(res.items || []);
      setTotal(res.totalCount || 0);

      if (res.profileType) {
        setProfileType({
          ...res.profileType,
          metrics: res.profileType.metricsConfig?.map((m: any) => ({
            id: m.id,
            template_id: m.templateId,
            name: m.name,
            parameters: m.parameters || {},
            custom_definition: m.customDefinition ? {
              name: m.customDefinition.name,
              target: m.customDefinition.target,
              step_name: m.customDefinition.stepName,
              operation: m.customDefinition.operation,
              field: m.customDefinition.field,
              filters: m.customDefinition.filters || [],
              group_by: m.customDefinition.groupBy,
              granularity: m.customDefinition.granularity,
              visualisation: m.customDefinition.visualisation,
            } : undefined,
          })) || []
        } as EntityProfileType);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load entity profiles');
    } finally {
      setIsLoading(false);
    }
  };

  // Fetch from backend via GraphQL whenever type/offset/committed change
  useEffect(() => {
    fetchProfiles();
  }, [type, offset, committed]);

  const page = Math.floor(offset / PAGE_SIZE) + 1;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <AppLayout>
      <div className="min-h-screen overflow-hidden bg-[#f8f8f6] px-4 py-7 sm:px-7 sm:py-10 lg:px-10">
        <div className="mx-auto max-w-6xl">
        {/* Edit Modal */}
        {profileType && (
          <ProfileTypeModal
            isOpen={isEditModalOpen}
            onClose={() => setIsEditModalOpen(false)}
            mode="edit"
            initialData={profileType}
            metricsTemplates={metricsTemplates}
            onRefresh={fetchProfiles}
            persistedTypes={profileType?.type || []}
          />
        )}

        {/* Back link */}
        <div className="mb-6">
          <Link
            to="/u/profiles"
            className="inline-flex items-center gap-1 text-sm font-medium text-stone-500 transition-colors hover:text-emerald-700"
          >
            <ChevronLeft className="w-4 h-4" /> Back to Entity Profile Types
          </Link>
        </div>

        {/* Header */}
        <div className="mb-7 rounded-2xl border border-stone-200 bg-white p-6 shadow-sm sm:p-8">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Entity profiles / {type}</p>
          <div className="mb-4 flex min-w-0 flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
            <div className="min-w-0 flex-1">
              <div className="mb-3 flex min-w-0 items-start gap-2 sm:items-center sm:gap-3">
                <h1 className="min-w-0 break-words text-3xl font-semibold leading-tight tracking-tight text-stone-950 sm:text-4xl">
                  {profileType?.name || type}
                </h1>
                {profileType && (
                  <button
                    onClick={() => navigate(`/u/profile-views/${encodeURIComponent(type!)}?tab=data`)}
                    className="mt-0.5 shrink-0 rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-900 sm:mt-0"
                    title="Edit Profile Type"
                    aria-label="Edit profile type"
                  >
                    <Edit2 className="w-4 h-4" />
                  </button>
                )}
                {profileType && (
                  <Link
                    to={`/u/profile-views/${encodeURIComponent(type!)}`}
                    className="mt-0.5 shrink-0 rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500 sm:mt-0"
                    title="Customize view"
                    aria-label="Customize view"
                  >
                    <SlidersHorizontal className="w-4 h-4" />
                  </Link>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[11px] font-medium text-gray-400 uppercase tracking-wider">Refs:</span>
                {profileType?.type?.map((key: string) => (
                  <span
                    key={key}
                    className="max-w-full break-all rounded border border-gray-200/60 bg-gray-100 px-2 py-0.5 font-mono text-[11px] text-gray-600"
                  >
                    {key}
                  </span>
                ))}
              </div>
            </div>

            {/* Explicit search */}
            <div className="w-full min-w-0 lg:w-[28rem] lg:shrink-0">
              <InlineSearch
                value={search}
                onChange={setSearch}
                onSearch={handleSearch}
                onClear={handleClear}
                isLoading={isLoading && committed !== ''}
                placeholder="Search profiles…"
              />
            </div>
          </div>
          
          {profileType?.description && (
            <p className="mb-5 max-w-3xl break-words text-sm leading-relaxed text-gray-600 sm:mb-6">
              {profileType.description}
            </p>
          )}
          
          {!!profileType?.metrics?.length && (
            <div className="flex min-w-0 flex-wrap items-center gap-2" aria-label="Configured metrics">
              <span className="text-xs text-gray-500">Metrics</span>
              {profileType.metrics.map((metric, index) => (
                <span key={metric.id || index} className={`inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium ${metricBadgeStyles[index % metricBadgeStyles.length]}`}>
                  <Settings className="h-3 w-3 shrink-0 opacity-70" />
                  <span className="min-w-0 break-words">{metric.name || metric.custom_definition?.name || metric.template_id}</span>
                </span>
              ))}
            </div>
          )}
        </div>

        {!isLoading && !error && <div className="mb-3 flex items-center gap-2">
          <h3 className="text-sm font-semibold text-gray-800">{committed ? 'Matching profiles' : 'Profiles'}</h3>
          <span className="text-xs tabular-nums text-gray-400">{total}</span>
        </div>}

        {/* Content */}
        {isLoading && items.length === 0 ? (
          <div className="flex items-center justify-center gap-2 rounded-2xl border border-stone-200 bg-white shadow-sm p-8 text-center text-gray-500 sm:p-12">
            <Activity className="w-5 h-5 animate-pulse" /> Loading profiles…
          </div>
        ) : error ? (
          <div className="rounded-2xl border border-red-200 bg-red-50 p-6 text-center text-red-600 sm:p-8">
            {error}
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center rounded-2xl border border-stone-200 bg-white shadow-sm p-8 text-center sm:p-12">
            <UserCircle className="w-12 h-12 text-gray-300 mb-3" />
            <h3 className="text-lg font-medium text-gray-900">
              {committed ? 'No matches' : 'No profiles yet'}
            </h3>
            <p className="text-gray-500 mt-1 max-w-md">
              {committed
                ? `No profiles match "${committed}".`
                : 'Profiles appear here automatically as threads reference entities of this type.'}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {items.map((p) => (
              <ProfileCard key={p.id} type={type!} profile={p} />
            ))}
          </div>
        )}

        {/* Pagination */}
        {total > PAGE_SIZE && (
          <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-gray-500">
              Page {page} of {totalPages}
            </p>
            <div className="grid grid-cols-2 gap-2 sm:flex sm:items-center">
              <button
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                disabled={offset === 0}
                className="px-3 py-2 border border-gray-300 rounded-md text-sm flex items-center gap-1 disabled:opacity-40 disabled:cursor-not-allowed hover:bg-gray-50"
              >
                <ChevronLeft className="w-4 h-4" /> Prev
              </button>
              <button
                onClick={() => setOffset(offset + PAGE_SIZE)}
                disabled={offset + PAGE_SIZE >= total}
                className="px-3 py-2 border border-gray-300 rounded-md text-sm flex items-center gap-1 disabled:opacity-40 disabled:cursor-not-allowed hover:bg-gray-50"
              >
                Next <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}
        </div>
      </div>
    </AppLayout>
  );
}

// --- Inline search component — icon + input + clear X ---

function InlineSearch({
  value,
  onChange,
  onSearch,
  onClear,
  isLoading,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  onSearch: () => void;
  onClear: () => void;
  isLoading?: boolean;
  placeholder?: string;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  
  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      onSearch();
    }
  };

  return (
    <div className="grid w-full min-w-0 grid-cols-1 gap-2 min-[420px]:grid-cols-[minmax(0,1fr)_auto]">
      <div className="relative min-w-0">
        <div className="absolute inset-y-0 left-3 flex items-center text-gray-400 pointer-events-none">
          {isLoading ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <Search className="w-4 h-4" />
          )}
        </div>
        <input
          ref={inputRef}
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          className="w-full pl-10 pr-9 py-2 text-sm border border-gray-200 rounded-lg bg-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
        />
        {value && (
          <button
            type="button"
            onClick={() => {
              onClear();
              inputRef.current?.focus();
            }}
            className="absolute inset-y-0 right-2 flex items-center text-gray-400 hover:text-gray-700 px-1"
            aria-label="Clear search"
          >
            <X className="w-4 h-4" />
          </button>
        )}
      </div>
      <button
        onClick={onSearch}
        className="w-full whitespace-nowrap rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 min-[420px]:w-auto"
      >
        Search
      </button>
    </div>
  );
}

// --- Contact-card style profile tile with lightweight metrics ---

function ProfileCard({ type, profile }: { type: string; profile: EntityProfileListItem }) {
  const navigate = useNavigate();
  const p = profile;

  const source = (p.name || p.refKey || '?').trim();
  const initials = useMemo(
    () =>
      source
        .split(/[\s_\-]+/)
        .filter(Boolean)
        .slice(0, 2)
        .map((s) => s[0]?.toUpperCase())
        .join('') ||
      source[0]?.toUpperCase() ||
      '?',
    [source]
  );

  const displayName = p.name || p.refKey;
  const showRefKey = Boolean(p.name) && p.name !== p.refKey;

  const health = p.metrics?.deliveryHealthScore ?? null;
  const healthPct = health !== null ? Math.round(health) : null;
  const healthBand =
    healthPct === null
      ? 'bg-gray-100 text-gray-500 border-gray-200'
      : healthPct >= 80
      ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
      : healthPct >= 50
      ? 'bg-amber-50 text-amber-700 border-amber-200'
      : 'bg-red-50 text-red-700 border-red-200';

  const totalDeliveries = p.metrics?.totalDeliveries ?? 0;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() =>
        navigate(`/u/profiles/${encodeURIComponent(type)}/${encodeURIComponent(p.refKey)}`)
      }
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          navigate(`/u/profiles/${encodeURIComponent(type)}/${encodeURIComponent(p.refKey)}`);
        }
      }}
      className="bg-white border border-gray-200 rounded-xl shadow-sm hover:shadow-md hover:border-gray-300 transition-all cursor-pointer overflow-hidden"
    >
      <div className="h-1.5 bg-gradient-to-r from-gray-900 to-gray-700" />

      <div className="p-4 sm:p-5">
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-full bg-gradient-to-br from-gray-700 to-gray-900 text-white flex items-center justify-center font-semibold text-sm tracking-wide shrink-0">
            {initials}
          </div>

          <div className="min-w-0 flex-1">
            <h3 className="break-words font-semibold text-gray-900">{displayName}</h3>
            {showRefKey && (
              <code className="block break-all font-mono text-xs text-gray-500">{p.refKey}</code>
            )}
          </div>
        </div>

        <div className="mt-4 pt-3 border-t border-gray-100 flex items-center justify-between text-xs text-gray-500">
          <span>
            Active{' '}
            {p.lastActiveAt
              ? formatDistanceToNow(new Date(p.lastActiveAt), { addSuffix: true })
              : '—'}
          </span>
        </div>
      </div>
    </div>
  );
}
