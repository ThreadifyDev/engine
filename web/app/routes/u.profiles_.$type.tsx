import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams, Link } from '@remix-run/react';
import type { MetaFunction } from '@remix-run/node';
import AppLayout from '~/components/AppLayout';
import { api } from '~/lib/api';
import type { EntityProfileType, MetricsTemplateResponse } from '~/lib/api';
import { graphqlClient, type EntityProfileListItem } from '~/lib/graphql';
import { ChevronLeft, ChevronRight, UserCircle, Search, X, Activity, Loader2, Settings, Edit2 } from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';
import { ProfileTypeModal } from '~/components/profiles/ProfileTypeModal';

export const meta: MetaFunction = ({ params }) => [
  { title: `${params.type ?? 'Profiles'} · Entity Profiles — Threadify` },
  { name: 'description', content: 'Entity profiles under this profile type' },
];

const PAGE_SIZE = 20;

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
    const token = api.getStoredToken();
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
            template_id: m.templateId,
            name: m.name,
            parameters: m.parameters || {}
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
      <div className="p-8">
        {/* Edit Modal */}
        {profileType && (
          <ProfileTypeModal
            isOpen={isEditModalOpen}
            onClose={() => setIsEditModalOpen(false)}
            mode="edit"
            initialData={profileType}
            metricsTemplates={metricsTemplates}
            onRefresh={fetchProfiles}
          />
        )}

        {/* Back link */}
        <div className="mb-4">
          <Link
            to="/u/profiles"
            className="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-gray-900 transition-colors"
          >
            <ChevronLeft className="w-4 h-4" /> Back to Profile Types
          </Link>
        </div>

        {/* Header */}
        <div className="mb-8">
          <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-4">
            <div>
              <div className="flex items-center gap-3 mb-2">
                <h2 className="text-3xl font-bold font-mono tracking-tight text-gray-900 truncate">
                  {profileType?.name || type}
                </h2>
                {profileType && (
                  <button
                    onClick={() => setIsEditModalOpen(true)}
                    className="p-1.5 text-gray-400 hover:text-gray-900 hover:bg-gray-100 rounded-md transition-colors"
                    title="Edit Profile Type"
                  >
                    <Edit2 className="w-4 h-4" />
                  </button>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[11px] font-medium text-gray-400 uppercase tracking-wider">Refs:</span>
                {profileType?.type?.map((key: string) => (
                  <span
                    key={key}
                    className="px-2 py-0.5 bg-gray-100 text-gray-600 border border-gray-200/60 rounded text-[11px] font-mono"
                  >
                    {key}
                  </span>
                ))}
              </div>
            </div>
            
            {/* Explicit search */}
            <div className="w-full sm:w-64 shrink-0">
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
            <p className="text-gray-600 mb-6 max-w-3xl text-sm leading-relaxed">
              {profileType.description}
            </p>
          )}
          
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 py-4 border-t border-b border-gray-100 mb-6">
            <div className="flex-1">
              {profileType?.metrics && profileType.metrics.length > 0 ? (
                <div className="flex items-center gap-2.5 flex-wrap">
                  <span className="text-[11px] font-medium text-gray-400 uppercase tracking-wider">Metrics</span>
                  <div className="w-px h-3 bg-gray-200 mx-1"></div>
                  {profileType.metrics.map((mc: any, idx: number) => (
                    <span key={idx} className="inline-flex items-center gap-1.5 px-2 py-1 bg-indigo-50 text-indigo-700 border border-indigo-100/50 rounded-md text-[11px] font-medium shadow-sm shadow-indigo-100/20">
                      <Settings className="w-3.5 h-3.5 opacity-60" />
                      {mc.name || mc.template_id}
                    </span>
                  ))}
                </div>
              ) : (
                <span className="text-[11px] font-medium text-gray-400 italic">No metrics configured</span>
              )}
            </div>
            
            <p className="text-sm font-medium text-gray-500 shrink-0">
              {total} {total === 1 ? 'profile' : 'profiles'} tracked
            </p>
          </div>
        </div>

        {/* Content */}
        {isLoading && items.length === 0 ? (
          <div className="bg-white border border-gray-200 rounded-lg p-12 text-center text-gray-500 flex items-center justify-center gap-2">
            <Activity className="w-5 h-5 animate-pulse" /> Loading profiles…
          </div>
        ) : error ? (
          <div className="bg-red-50 border border-red-200 rounded-lg p-8 text-center text-red-600">
            {error}
          </div>
        ) : items.length === 0 ? (
          <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
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
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {items.map((p) => (
              <ProfileCard key={p.id} type={type!} profile={p} />
            ))}
          </div>
        )}

        {/* Pagination */}
        {total > PAGE_SIZE && (
          <div className="mt-8 flex items-center justify-between">
            <p className="text-sm text-gray-500">
              Page {page} of {totalPages}
            </p>
            <div className="flex items-center gap-2">
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
    <div className="flex items-center gap-2 w-full sm:w-auto">
      <div className="relative flex-1 sm:w-80 max-w-full">
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
        className="px-4 py-2 bg-gray-900 text-white text-sm font-medium rounded-lg hover:bg-gray-800 transition-colors whitespace-nowrap"
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

      <div className="p-5">
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-full bg-gradient-to-br from-gray-700 to-gray-900 text-white flex items-center justify-center font-semibold text-sm tracking-wide shrink-0">
            {initials}
          </div>

          <div className="min-w-0 flex-1">
            <h3 className="font-semibold text-gray-900 truncate">{displayName}</h3>
            {showRefKey && (
              <code className="text-xs font-mono text-gray-500 truncate block">{p.refKey}</code>
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
