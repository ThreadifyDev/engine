import { TabBar } from '~/components/TabBar';
import { useState, useEffect, useMemo } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router';
import { type EntityProfile } from '~/lib/api';
import { graphqlClient } from '~/lib/graphql';
import AppLayout from '~/components/AppLayout';
import {
  Activity,
  ChevronLeft, AlertTriangle,
  History as HistoryIcon, LayoutDashboard, BarChart2
} from 'lucide-react';

import OverviewTab from '~/components/profiles/OverviewTab';
import MetricsTab from '~/components/profiles/MetricsTab';
import DeliveryHealthTab from '~/components/profiles/DeliveryHealthTab';
import HistoryTab from '~/components/profiles/HistoryTab';

type TabType = 'overview' | 'history' | 'metrics' | 'delivery-health';

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
        <header className="mb-6 min-w-0">
          <h1 className="break-words text-2xl font-semibold leading-snug tracking-tight text-gray-900">
            {profile.name || refKey}
          </h1>
          {profile.profileType?.name && (
            <p className="mt-1 text-sm text-gray-500">{profile.profileType.name}</p>
          )}
          <details className="group mt-3 text-sm text-gray-500">
            <summary className="w-fit cursor-pointer rounded hover:text-gray-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-gray-400">
              Reference details
            </summary>
            <dl className="mt-3 space-y-3 border-l border-gray-200 pl-4 text-xs">
              <div>
                <dt className="mb-1 font-medium text-gray-600">Reference</dt>
                <dd className="break-all font-mono text-gray-700">{profile.refKey}</dd>
              </div>
              {!!profile.profileType?.type?.length && (
                <div>
                  <dt className="mb-1 font-medium text-gray-600">Identifier fields</dt>
                  <dd className="break-words text-gray-700">{profile.profileType.type.join(', ')}</dd>
                </div>
              )}
            </dl>
          </details>
        </header>

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
