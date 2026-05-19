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

import OverviewTab from '~/components/profiles/OverviewTab';
import MetricsTab from '~/components/profiles/MetricsTab';
import HistoryTab from '~/components/profiles/HistoryTab';

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
    const hasMetricsConfig = ((profile as any)?.profileType?.metricsConfig?.length ?? 0) > 0;
    return <MetricsTab refKey={refKey} type={type} hasMetricsConfig={hasMetricsConfig} />;
  }, [refKey, type, (profile as any)?.profileType?.metricsConfig?.length]);

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
                {profile.profileType?.type?.filter((k: string) => k !== type).map((key: string) => (
                  <span 
                    key={key}
                    className="inline-flex items-center gap-1 px-1.5 py-0.5 bg-gray-50 border border-gray-200 rounded text-xs opacity-60"
                    title="Missing value for this identifier type"
                  >
                    <span className="font-medium text-gray-500">{key}</span>
                  </span>
                ))}
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
