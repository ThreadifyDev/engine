import { useState, useEffect, useRef, useMemo } from 'react';
import { useParams, useNavigate, useSearchParams } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api, type EntityProfile } from '~/lib/api';
import { graphqlClient, type Thread } from '~/lib/graphql';
import AppLayout from '~/components/AppLayout';
import { 
  UserCircle, Activity, 
  ChevronLeft, AlertTriangle,
  TrendingUp, TrendingDown, Calendar, Search, History as HistoryIcon, LayoutDashboard, BarChart2
} from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';

type TabType = 'overview' | 'history' | 'metrics';

export const meta: MetaFunction = ({ params }) => {
  return [
    { title: `Profile ${params.refKey} - Threadify` },
    { name: "description", content: "View entity profile metrics" },
  ];
};

export default function EntityProfileDetail() {
  const params = useParams();
  const navigate = useNavigate();
  const { type, refKey } = params;

  const [profile, setProfile] = useState<EntityProfile | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();
  const urlTab = searchParams.get('tab') as TabType | null;
  const activeTab: TabType = (urlTab === 'history' || urlTab === 'overview' || urlTab === 'metrics') ? urlTab : 'overview';

  useEffect(() => {
    if (!type || !refKey) {
      setError("Invalid profile parameters");
      setIsLoading(false);
      return;
    }
    
    fetchProfile(refKey, type);
  }, [type, refKey]);

  const fetchProfile = async (rKey: string, pType: string) => {
    try {
      setIsLoading(true);
      const res = await graphqlClient.getEntityProfile({ refKey: rKey, type: pType });
      if (res) {
        setProfile(res);
      } else {
        setError("Entity profile not found or has no active metrics yet.");
      }
    } catch (err: any) {
      console.error(err);
      setError(err.message || 'Entity profile not found or currently unindexed.');
    } finally {
      setIsLoading(false);
    }
  };

  const metrics = profile?.metrics || {
    deliveryHealthScore: 0,
    healthTrendSlope: 0,
    totalDeliveries: 0,
    completedSuccessfully: 0,
    validationViolations: 0,
    averageDeliveryTimeMs: 0,
    lastCalculatedAt: new Date().toISOString()
  };

  const memoizedOverviewTab = useMemo(() => {
    if (!profile) return null;
    return <OverviewTab profile={profile} metrics={metrics} />;
  }, [profile, metrics]);

  const memoizedMetricsTab = useMemo(() => {
    if (!refKey || !type) return null;
    return <MetricsTab refKey={refKey} type={type} />;
  }, [refKey, type]);

  const memoizedHistoryTab = useMemo(() => {
    if (!profile || !refKey) return null;
    return (
      <HistoryTab 
        profileId={profile.id}
        refValue={refKey} 
        navigate={navigate} 
      />
    );
  }, [profile?.id, refKey, navigate]);

  if (isLoading) {
    return (
      <AppLayout>
        <div className="flex h-screen items-center justify-center">
          <div className="flex items-center gap-2 text-gray-500">
            <Activity className="w-5 h-5 animate-pulse" />
            <span>Loading profile...</span>
          </div>
        </div>
      </AppLayout>
    );
  }

  if (error || !profile) {
    return (
      <AppLayout>
        <div className="p-8 px-4 sm:px-6 lg:px-8 max-w-4xl mx-auto mt-12">
          <button 
            onClick={() => navigate(type ? `/u/profiles/${encodeURIComponent(type)}` : '/u/profiles')}
            className="text-red-700 hover:text-red-800 mb-6 flex items-center gap-2 text-sm font-medium transition-colors"
          >
            <ChevronLeft className="w-4 h-4" /> Back to Profiles
          </button>
          
          <div className="bg-red-50 border border-red-100 rounded-xl p-8 text-center flex flex-col items-center">
            <AlertTriangle className="w-12 h-12 text-red-400 mb-4" />
            <h2 className="text-xl font-bold text-red-900 mb-2">Profile Not Found</h2>
            <p className="text-red-700">{error}</p>
            <p className="text-sm text-red-500 mt-4 max-w-md">
              The entity <strong>{refKey}</strong> of type <strong>{type}</strong> either doesn't exist, has no associated activity logged in Archival storage, or has not yet been processed by the Intelligence stream.
            </p>
          </div>
        </div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div className="p-8">
        {/* Back */}
        <button
          onClick={() => navigate(type ? `/u/profiles/${encodeURIComponent(type)}` : '/u/profiles')}
          className="text-gray-500 hover:text-gray-900 mb-6 flex items-center text-sm font-medium transition-colors"
        >
          ← Back to Profiles
        </button>

        {/* Identity header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg border border-gray-200 flex items-center justify-center bg-gray-50">
              <UserCircle className="w-6 h-6 text-gray-600" />
            </div>
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-3xl font-semibold text-gray-900">
                  {profile.name || refKey}
                </h1>
                {profile.profileType?.name && (
                  <span className="px-2 py-1 rounded bg-indigo-50 text-indigo-700 text-xs font-medium border border-indigo-100">
                    {profile.profileType.name}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap gap-2 mt-3">
                {profile.profileType?.type?.map((key: string) => (
                  <span 
                    key={key}
                    className={`px-2 py-0.5 rounded-full text-[10px] font-medium uppercase tracking-wider ${
                      key === refKey 
                        ? 'bg-blue-100 text-blue-700 border border-blue-200' 
                        : 'bg-gray-100 text-gray-600 border border-gray-200'
                    }`}
                  >
                    {key}
                  </span>
                ))}
                {!profile.profileType && (
                  <>
                    <span className="px-2 py-0.5 rounded bg-gray-100 text-gray-700 font-mono text-xs">{type}</span>
                    <span className="ml-2 text-gray-400 font-mono text-xs">{refKey}</span>
                  </>
                )}
              </div>
            </div>
          </div>

          <div className="text-right">
            <p className="text-xs text-gray-500 mb-1">Health Score</p>
            <div className="flex items-center gap-2">
              <span className="text-4xl font-semibold text-gray-900 tabular-nums">
                {metrics.deliveryHealthScore !== null ? Math.round(metrics.deliveryHealthScore) : '-'}
              </span>
              <div className={`flex items-center gap-1 text-sm font-medium ${metrics.healthTrendSlope !== null && metrics.healthTrendSlope >= 0 ? 'text-green-600' : 'text-red-600'}`}>
                {metrics.healthTrendSlope !== null && metrics.healthTrendSlope >= 0 ? <TrendingUp className="w-4 h-4" /> : <TrendingDown className="w-4 h-4" />}
                <span>{metrics.healthTrendSlope !== null ? Math.abs(metrics.healthTrendSlope).toFixed(1) : '-'}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Tabs */}
        <div className="border-b border-gray-200 mb-6">
          <nav className="-mb-px flex space-x-8">
            <button
              onClick={() => setSearchParams(prev => { prev.set('tab', 'overview'); return prev; })}
              className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors flex items-center gap-2 ${
                activeTab === 'overview'
                  ? 'border-gray-900 text-gray-900'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <LayoutDashboard className="w-4 h-4" />
              Overview
            </button>
            <button
              onClick={() => setSearchParams(prev => { prev.set('tab', 'history'); return prev; })}
              className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors flex items-center gap-2 ${
                activeTab === 'history'
                  ? 'border-gray-900 text-gray-900'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <HistoryIcon className="w-4 h-4" />
              History
            </button>
            <button
              onClick={() => setSearchParams(prev => { prev.set('tab', 'metrics'); return prev; })}
              className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors flex items-center gap-2 ${
                activeTab === 'metrics'
                  ? 'border-gray-900 text-gray-900'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <BarChart2 className="w-4 h-4" />
              Metrics
            </button>
          </nav>
        </div>
        <div className="mt-4">
          {activeTab === 'overview' && memoizedOverviewTab}
          {activeTab === 'metrics' && memoizedMetricsTab}
          {activeTab === 'history' && memoizedHistoryTab}
        </div>
      </div>
    </AppLayout>
  );
}

// --- Overview tab: the existing metrics + details ---

function OverviewTab({ profile, metrics }: { profile: EntityProfile; metrics: any }) {
  return (
    <>
      <div className="grid grid-cols-4 gap-8 py-6 border-b border-gray-200 mb-10">
        <div>
          <p className="text-xs text-gray-500 mb-1">Total Activities</p>
          <p className="text-sm text-gray-900 font-medium">{metrics.totalDeliveries}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">Successful</p>
          <p className="text-sm text-green-700 font-medium">{metrics.completedSuccessfully}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">Violations</p>
          <p className="text-sm text-red-600 font-medium">{metrics.validationViolations}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">Avg Duration</p>
          <p className="text-sm text-gray-900 font-medium">
            {metrics.averageDeliveryTimeMs > 0
              ? `${(metrics.averageDeliveryTimeMs / 1000).toFixed(1)}s`
              : 'N/A'}
          </p>
        </div>
      </div>

      <h2 className="text-lg font-semibold text-gray-900 mb-4">Profile Details</h2>
      <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
        <div className="divide-y divide-gray-200">
          <div className="px-6 py-4 hover:bg-gray-50">
            <dt className="text-sm font-medium text-gray-500 mb-1">Record ID</dt>
            <dd className="font-mono text-sm text-gray-900">{profile.id}</dd>
          </div>
          <div className="px-6 py-4 hover:bg-gray-50">
            <dt className="text-sm font-medium text-gray-500 mb-1">First Observed</dt>
            <dd className="text-sm text-gray-900">
              {new Date(profile.createdAt).toLocaleString()}
              <span className="text-gray-500 ml-2">
                ({formatDistanceToNow(new Date(profile.createdAt), { addSuffix: true })})
              </span>
            </dd>
          </div>
          <div className="px-6 py-4 hover:bg-gray-50">
            <dt className="text-sm font-medium text-gray-500 mb-1">Last Active</dt>
            <dd className="text-sm text-gray-900">
              {new Date(profile.lastActiveAt).toLocaleString()}
              <span className="text-gray-500 ml-2">
                ({formatDistanceToNow(new Date(profile.lastActiveAt), { addSuffix: true })})
              </span>
            </dd>
          </div>
          {metrics.lastCalculatedAt && (
            <div className="px-6 py-4 hover:bg-gray-50">
              <dt className="text-sm font-medium text-gray-500 mb-1">Metrics Last Calculated</dt>
              <dd className="text-sm text-gray-900">
                {new Date(metrics.lastCalculatedAt).toLocaleString()}
              </dd>
            </div>
          )}
          {profile.profileType?.metricsConfig && profile.profileType.metricsConfig.length > 0 && (
            <div className="px-6 py-4 hover:bg-gray-50">
              <dt className="text-sm font-medium text-gray-500 mb-2">Configured Metrics</dt>
              <dd className="text-sm text-gray-900">
                <ul className="list-disc pl-5 space-y-1">
                  {profile.profileType.metricsConfig.map((mc: any, i: number) => (
                    <li key={i}>
                      {mc.name ? (
                        <span>{mc.name} <span className="text-gray-500 text-xs">({mc.templateId})</span></span>
                      ) : (
                        <span>{mc.templateId}</span>
                      )}
                    </li>
                  ))}
                </ul>
              </dd>
            </div>
          )}
        </div>
      </div>
    </>
  );
}

// --- Metrics tab: computed metrics from metric templates ---

type MetricsRange = '7d' | '30d' | '90d';

function MetricsTab({ refKey, type }: { refKey: string; type: string }) {
  const [range, setRange] = useState<MetricsRange>('7d');
  const [data, setData] = useState<any>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setIsLoading(true);
        setError(null);
        setData(null);
        const result = await graphqlClient.getComputedMetrics({ refKey, type, range });
        // computedMetrics is returned as a JSON string by the backend — parse it
        const parsed = result ? (typeof result === 'string' ? JSON.parse(result) : result) : null;
        if (!cancelled) setData(parsed);
      } catch (err: any) {
        if (!cancelled) setError(err.message || 'Failed to load metrics');
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [refKey, type, range]);

  return (
    <div>
      {/* Range selector */}
      <div className="flex items-center gap-2 mb-6">
        {(['7d', '30d', '90d'] as MetricsRange[]).map(r => (
          <button
            key={r}
            onClick={() => setRange(r)}
            className={`px-3 py-1.5 text-sm font-medium rounded-lg transition-colors ${
              range === r
                ? 'bg-gray-900 text-white'
                : 'bg-white border border-gray-200 text-gray-600 hover:border-gray-300 hover:text-gray-900'
            }`}
          >
            {r}
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center text-gray-500 flex items-center justify-center gap-2">
          <Activity className="w-5 h-5 animate-pulse" /> Loading metrics…
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-6 text-red-600 text-sm">
          {error}
        </div>
      )}

      {!isLoading && !error && data === null && (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
          <BarChart2 className="w-12 h-12 text-gray-300 mb-3" />
          <h3 className="text-base font-medium text-gray-900">No metrics configured</h3>
          <p className="text-sm text-gray-500 mt-1 max-w-sm">
            Add metric templates to this entity profile type to start seeing computed results here.
          </p>
        </div>
      )}

      {!isLoading && !error && data !== null && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
          {Object.entries(data as Record<string, any>).map(([metricName, result]) => {
            const parts = metricName.split(' (');
            const mainName = parts[0];
            const status = parts.length > 1 ? parts[1].replace('STATUS: ', '').replace(')', '').toLowerCase() : null;
            
            const nameSplit = mainName.split(': ');
            let tagline = nameSplit[0];
            let header = nameSplit.length > 1 ? nameSplit[1] : tagline;
            
            // Clean up header from snake_case if necessary
            header = header.split('_').map(w => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ');
            tagline = tagline.split('_').map(w => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ');

            if (status) {
              tagline = `${tagline} (${status})`;
            }

            return (
              <div key={metricName} className="flex flex-col gap-2">
                <div className="px-1">
                  <p className="text-[10px] font-extrabold text-gray-400 uppercase tracking-[0.15em] mb-0.5">
                    {tagline}
                  </p>
                </div>
                <div className="bg-white border border-gray-200 rounded-xl overflow-hidden flex-1 flex flex-col shadow-sm hover:shadow-md transition-shadow">
                  <div className="px-5 py-3 border-b border-gray-50 bg-gray-50/50">
                    <h3 className="text-xs font-bold text-gray-900 uppercase tracking-tight">{header}</h3>
                  </div>
                  <div className="p-5 flex-1 flex flex-col justify-center">
                    {Array.isArray(result) ? (
                      result.length === 0 ? (
                        <p className="text-xs text-gray-400 italic text-center">No data</p>
                      ) : (
                        <div className="overflow-x-auto">
                          <table className="w-full text-sm">
                            <thead>
                              <tr className="border-b border-gray-100">
                                {Object.keys(result[0]).map(col => (
                                  <th key={col} className="text-left text-[10px] font-bold text-gray-400 pb-1 pr-4 font-mono uppercase">{col}</th>
                                ))}
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-50">
                              {result.slice(0, 5).map((row: any, i: number) => (
                                <tr key={i}>
                                  {Object.values(row).map((val: any, j: number) => (
                                    <td key={j} className="py-1.5 pr-4 text-gray-900 font-mono text-xs">{String(val ?? '—')}</td>
                                  ))}
                                </tr>
                              ))}
                              {result.length > 5 && (
                                <tr>
                                  <td colSpan={Object.keys(result[0]).length} className="pt-2 text-[10px] text-gray-400 text-center italic">
                                    + {result.length - 5} more rows
                                  </td>
                                </tr>
                              )}
                            </tbody>
                          </table>
                        </div>
                      )
                    ) : (
                      <div className="flex flex-col items-center justify-center py-4">
                        <span className="text-3xl font-bold text-gray-900 tabular-nums">
                          {typeof result === 'number' ? 
                            (Number.isInteger(result) ? result : result.toFixed(2)) : 
                            String(result ?? '—')}
                        </span>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// --- History tab: threads referencing (ref_type=type, ref_value=refKey) ---

function HistoryTab({
  profileId,
  refValue,
  navigate,
}: {
  profileId: string;
  refValue: string;
  navigate: (path: string) => void;
}) {
  const [threads, setThreads] = useState<Thread[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadedRef = useRef(false);

  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;

    (async () => {
      try {
        setIsLoading(true);
        setError(null);
        // Query by profileId (backend resolves the reference keys)
        const res = await graphqlClient.getEntityProfileHistory({
          profileID: profileId,
        });
        setThreads(res.threads || []);
        setTotalCount(res.totalCount || 0);
      } catch (err: any) {
        setError(err.message || 'Failed to load thread history');
      } finally {
        setIsLoading(false);
      }
    })();
  }, [profileId, refValue]);

  if (isLoading) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg p-12 text-center text-gray-500 flex items-center justify-center gap-2">
        <Activity className="w-5 h-5 animate-pulse" /> Loading thread history…
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-8 text-center text-red-600">
        {error}
      </div>
    );
  }

  if (threads.length === 0) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
        <Search className="w-12 h-12 text-gray-300 mb-3" />
        <h3 className="text-lg font-medium text-gray-900">No threads referenced this entity</h3>
        <p className="text-gray-500 mt-1 max-w-md">
          No threads have been recorded with <code className="font-mono text-xs bg-gray-100 px-1.5 py-0.5 rounded">ID={refValue}</code>.
        </p>
      </div>
    );
  }

  const getStatusBadge = (status: string) => {
    const base = 'text-xs px-1.5 py-0.5 rounded-md font-medium';
    const map: Record<string, string> = {
      active: `${base} bg-blue-50 text-blue-700`,
      completed: `${base} bg-green-50 text-green-700`,
      failed: `${base} bg-red-50 text-red-700`,
      cancelled: `${base} bg-gray-100 text-gray-700`,
    };
    return map[status] || `${base} bg-gray-100 text-gray-700`;
  };

  return (
    <div className="space-y-2">
      <div className="mb-3 text-xs text-gray-600">
        Found <span className="font-semibold text-gray-900">{totalCount}</span> thread
        {totalCount !== 1 ? 's' : ''} referencing this entity
      </div>

      {threads.map((thread) => {
        const refs = thread.refs
          ? typeof thread.refs === 'string'
            ? JSON.parse(thread.refs)
            : thread.refs
          : {};

        const threadIdSummary = thread.id.split('-').pop() || '';
        const threadTitle = thread.label 
          ? `${thread.label} (${threadIdSummary})` 
          : thread.contractName 
            ? `${thread.contractName} (${threadIdSummary})` 
            : threadIdSummary;

        return (
          <div
            key={thread.id}
            onClick={() => navigate(`/u/threads/${thread.id}`)}
            className="bg-white border border-gray-200 rounded-lg p-3 hover:shadow-sm hover:border-gray-300 transition-all cursor-pointer"
          >
            <div className="flex justify-between items-start">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1.5">
                  <h3 className="font-semibold text-sm text-gray-900 truncate">
                    {threadTitle}
                  </h3>
                  <span className={getStatusBadge(thread.status)}>{thread.status}</span>
                </div>
                <p className="text-xs text-gray-500 font-mono mb-2 truncate">
                  {thread.contractName && thread.contractVersion
                    ? `${thread.contractName} v${thread.contractVersion}`
                    : '-'}
                </p>
                <div className="flex items-center gap-3 text-xs text-gray-600">
                  {thread.startedAt && (
                    <div className="flex items-center gap-1">
                      <Calendar className="w-3 h-3" />
                      {formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}
                    </div>
                  )}
                  {Object.keys(refs).length > 0 && (
                    <div className="flex items-center gap-1.5">
                      {Object.entries(refs)
                        .slice(0, 2)
                        .map(([key, value]) => (
                          <span
                            key={key}
                            className="inline-flex items-center px-1.5 py-0.5 bg-blue-50 text-blue-700 text-xs rounded"
                          >
                            <span className="font-medium">{key}:</span>
                            <span className="ml-0.5">{String(value)}</span>
                          </span>
                        ))}
                      {Object.keys(refs).length > 2 && (
                        <span className="text-xs text-gray-500">
                          +{Object.keys(refs).length - 2} more
                        </span>
                      )}
                    </div>
                  )}
                </div>
              </div>
              <div className="ml-3 flex-shrink-0">
                <button className="px-3 py-1.5 text-xs font-medium text-gray-700 hover:text-gray-900 hover:bg-gray-50 rounded-md transition-colors">
                  View →
                </button>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
