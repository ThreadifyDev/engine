import { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api, type EntityProfile } from '~/lib/api';
import { graphqlClient, type Thread } from '~/lib/graphql';
import AppLayout from '~/components/AppLayout';
import { 
  UserCircle, Activity, 
  ChevronLeft, AlertTriangle,
  TrendingUp, TrendingDown, Calendar, Search, History as HistoryIcon, LayoutDashboard
} from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';

type TabType = 'overview' | 'history';

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
  const [activeTab, setActiveTab] = useState<TabType>('overview');

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
            className="text-gray-500 hover:text-black mb-6 flex items-center gap-2 text-sm font-medium transition-colors"
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

  // Safely extract metrics
  const metrics = profile.metrics || {
    deliveryHealthScore: 0,
    healthTrendSlope: 0,
    totalDeliveries: 0,
    completedSuccessfully: 0,
    validationViolations: 0,
    averageDeliveryTimeMs: 0,
    lastCalculatedAt: new Date().toISOString()
  };

  return (
    <AppLayout>
      <div className="p-8">
        {/* Back */}
        <button
          onClick={() => navigate(type ? `/u/profiles/${encodeURIComponent(type)}` : '/u/profiles')}
          className="text-gray-600 hover:text-gray-900 mb-6 flex items-center text-sm"
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
              <h1 className="text-3xl font-semibold text-gray-900">
                {profile.name || refKey}
              </h1>
              <p className="text-sm text-gray-500 font-mono mt-1">
                <span className="px-2 py-0.5 rounded bg-gray-100 text-gray-700">{type}</span>
                <span className="ml-2 text-gray-400">{refKey}</span>
              </p>
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
              onClick={() => setActiveTab('overview')}
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
              onClick={() => setActiveTab('history')}
              className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors flex items-center gap-2 ${
                activeTab === 'history'
                  ? 'border-gray-900 text-gray-900'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <HistoryIcon className="w-4 h-4" />
              History
            </button>
          </nav>
        </div>

        {activeTab === 'overview' ? (
          <OverviewTab profile={profile} metrics={metrics} />
        ) : (
          <HistoryTab refKey={refKey!} refValue={refKey!} typeKey={type!} navigate={navigate} />
        )}
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
        </div>
      </div>
    </>
  );
}

// --- History tab: threads referencing (ref_type=type, ref_value=refKey) ---

function HistoryTab({
  typeKey,
  refValue,
  navigate,
}: {
  refKey: string;
  refValue: string;
  typeKey: string;
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
        // Strict query — refKey=typeKey, refValue=refValue
        const res = await graphqlClient.getThreadsByRef({
          refKey: typeKey,
          refValue,
        });
        setThreads(res.threads || []);
        setTotalCount(res.totalCount || 0);
      } catch (err: any) {
        setError(err.message || 'Failed to load thread history');
      } finally {
        setIsLoading(false);
      }
    })();
  }, [typeKey, refValue]);

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
          No threads have been recorded with <code className="font-mono text-xs bg-gray-100 px-1.5 py-0.5 rounded">{typeKey}={refValue}</code>.
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

        return (
          <div
            key={thread.id}
            onClick={() => navigate(`/u/threads/${thread.id}`)}
            className="bg-white border border-gray-200 rounded-lg p-3 hover:shadow-sm hover:border-gray-300 transition-all cursor-pointer"
          >
            <div className="flex justify-between items-start">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1.5">
                  <h3 className="font-semibold text-sm text-gray-900 truncate">{thread.id}</h3>
                  <span className={getStatusBadge(thread.status)}>{thread.status}</span>
                </div>
                <p className="text-xs text-gray-500 font-mono mb-2 truncate">
                  {thread.contractName && thread.contractVersion
                    ? `${thread.contractName} v${thread.contractVersion}`
                    : thread.contractName || '-'}
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
