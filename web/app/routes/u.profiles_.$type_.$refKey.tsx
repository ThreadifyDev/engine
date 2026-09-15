import { TabBar } from '~/components/TabBar';
import { useState, useEffect, useRef, useMemo } from 'react';
import { useParams, useNavigate, useSearchParams } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api, type EntityProfile } from '~/lib/api';
import { graphqlClient, type Thread } from '~/lib/graphql';
import AppLayout from '~/components/AppLayout';
import { 
  UserCircle, Activity, 
  ChevronLeft, AlertTriangle,
  History as HistoryIcon, LayoutDashboard, BarChart2
} from 'lucide-react';

import OverviewTab from '~/components/profiles/OverviewTab';
import MetricsTab from '~/components/profiles/MetricsTab';
import DeliveryHealthTab from '~/components/profiles/DeliveryHealthTab';
import HistoryTab from '~/components/profiles/HistoryTab';

type TabType = 'overview' | 'history' | 'metrics' | 'delivery-health';

const IDENTIFIER_BADGE_STYLES = [
  'border-emerald-200 bg-emerald-50 text-emerald-800',
  'border-sky-200 bg-sky-50 text-sky-800',
  'border-amber-200 bg-amber-50 text-amber-800',
  'border-rose-200 bg-rose-50 text-rose-800',
  'border-cyan-200 bg-cyan-50 text-cyan-800',
] as const;

function getIdentifierBadgeStyle(identifier: string) {
  const colorIndex = Array.from(identifier).reduce((hash, character) => hash + character.charCodeAt(0), 0)
    % IDENTIFIER_BADGE_STYLES.length;
  return IDENTIFIER_BADGE_STYLES[colorIndex];
}

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
  const activeTab: TabType = (urlTab === 'history' || urlTab === 'overview' || urlTab === 'metrics' || urlTab === 'delivery-health') ? urlTab : 'overview';

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

  const memoizedDeliveryHealthTab = useMemo(() => {
    if (!refKey || !type) return null;
    return <DeliveryHealthTab refKey={refKey} type={type} />;
  }, [refKey, type]);

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
        <div className="flex min-h-[calc(100vh-73px)] items-center justify-center px-4 lg:min-h-screen">
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
        <div className="p-4 sm:p-6 lg:p-8">
          <button
            onClick={() => navigate(type ? `/u/profiles/${encodeURIComponent(type)}` : '/u/profiles')}
            className="text-red-700 hover:text-red-800 mb-6 flex items-center gap-2 text-sm font-medium transition-colors"
          >
            <ChevronLeft className="w-4 h-4" /> Back to Profiles
          </button>

          <div className="bg-red-50 border border-red-100 rounded-xl p-5 text-center flex flex-col items-center sm:p-8">
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
      <div className="max-w-full overflow-hidden p-4 sm:p-6 lg:p-8">
        {/* Back */}
        <button
          onClick={() => navigate(type ? `/u/profiles/${encodeURIComponent(type)}` : '/u/profiles')}
          className="mb-4 flex items-center text-sm font-medium text-gray-600 transition-colors hover:text-gray-900 sm:mb-6"
        >
          ← Back to Profiles
        </button>

        {/* Identity header */}
        <div className="mb-6 flex items-start">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-purple-200 bg-purple-50">
              <UserCircle className="h-6 w-6 text-purple-700" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:gap-3">
                <h1 className="break-words text-2xl font-semibold leading-tight text-gray-900 sm:text-3xl">
                  {profile.name || refKey}
                </h1>
                {profile.profileType?.name && (
                  <span className="max-w-full rounded border border-purple-300 bg-purple-100 px-2 py-1 text-xs font-semibold text-purple-900">
                    {profile.profileType.name}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap gap-2 mt-3">
                <span
                  className="inline-flex max-w-full flex-wrap items-center gap-1 rounded border border-blue-200 bg-blue-50 px-2 py-1 text-xs"
                  title="Entity reference value"
                >
                  <span className="font-semibold text-blue-700">Reference:</span>
                  <code className="break-all font-medium text-blue-950">{profile.refKey}</code>
                </span>
                {profile.profileType?.type?.map((key: string) => (
                  <span
                    key={key}
                    className={`inline-flex max-w-full items-center rounded border px-2 py-1 text-xs ${getIdentifierBadgeStyle(key)}`}
                    title="Configured identifier type"
                  >
                    <span className="break-all font-semibold">{key}</span>
                  </span>
                ))}
              </div>
            </div>
          </div>

        </div>

        <TabBar label="Entity profile" value={activeTab} onChange={tab => setSearchParams(prev => { prev.set('tab', tab); return prev; })} panelId="profile-panel" className="mb-6"
          items={[
            {value:'overview',label:<><LayoutDashboard className="h-4 w-4" />Overview</>},
            {value:'delivery-health',label:<><BarChart2 className="h-4 w-4" />Delivery Health</>},
            {value:'metrics',label:<><BarChart2 className="h-4 w-4" />Metrics</>},
            {value:'history',label:<><HistoryIcon className="h-4 w-4" />History</>}
          ]} />

        <div id="profile-panel" role="tabpanel" aria-label="Entity profile content" className="min-w-0 mt-4">
          {activeTab === 'overview' && memoizedOverviewTab}
          {activeTab === 'delivery-health' && memoizedDeliveryHealthTab}
          {activeTab === 'metrics' && memoizedMetricsTab}
          {activeTab === 'history' && memoizedHistoryTab}
        </div>
      </div>
    </AppLayout>
  );
}
