import { useState, useEffect } from 'react';
import { useParams, useNavigate } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api, type EntityProfile } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import { 
  UserCircle, Activity, Archive, LayoutDashboard, 
  ChevronLeft, AlertTriangle, CheckCircle2,
  Clock, TrendingUp, TrendingDown, Target
} from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';

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
      const res = await api.getEntityProfile(rKey, pType);
      if (res && res.data) {
        setProfile(res.data);
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
            onClick={() => navigate('/u/profiles')}
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

  const getScoreColor = (score: number) => {
    if (score >= 90) return "text-green-600 bg-green-50 border-green-200";
    if (score >= 70) return "text-amber-600 bg-amber-50 border-amber-200";
    return "text-red-600 bg-red-50 border-red-200";
  };

  const scoreStyles = getScoreColor(metrics.deliveryHealthScore);

  return (
    <AppLayout>
      <div className="p-8 max-w-7xl mx-auto">
        {/* Header */}
        <div className="mb-12">
          <button
            onClick={() => navigate('/u/profiles')}
            className="text-gray-600 hover:text-gray-900 mb-6 flex items-center text-sm"
          >
            ← Back to Profiles
          </button>
          
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
                  {Math.round(metrics.deliveryHealthScore)}
                </span>
                <div className={`flex items-center gap-1 text-sm font-medium ${metrics.healthTrendSlope >= 0 ? 'text-green-600' : 'text-red-600'}`}>
                  {metrics.healthTrendSlope >= 0 ? <TrendingUp className="w-4 h-4" /> : <TrendingDown className="w-4 h-4" />}
                  <span>{Math.abs(metrics.healthTrendSlope).toFixed(1)}</span>
                </div>
              </div>
            </div>
          </div>

          {/* Metadata Grid */}
          <div className="grid grid-cols-4 gap-8 py-6 border-b border-gray-200">
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
                  : 'N/A'
                }
              </p>
            </div>
          </div>
        </div>

        {/* Details Section */}
        <div>
          <h2 className="text-lg font-semibold text-gray-900 mb-4">
            Profile Details
          </h2>
          
          <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
            <div className="divide-y divide-gray-200">
              <div className="px-6 py-4 hover:bg-gray-50">
                <dt className="text-sm font-medium text-gray-500 mb-1">Record ID</dt>
                <dd className="font-mono text-sm text-gray-900">{profile.id}</dd>
              </div>
              <div className="px-6 py-4 hover:bg-gray-50">
                <dt className="text-sm font-medium text-gray-500 mb-1">First Observed</dt>
                <dd className="text-sm text-gray-900">
                  {new Date(profile.createdAt).toLocaleDateString('en-US', { 
                    month: 'short',
                    day: 'numeric',
                    year: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                  })}
                  <span className="text-gray-500 ml-2">({formatDistanceToNow(new Date(profile.createdAt), {addSuffix: true})})</span>
                </dd>
              </div>
              <div className="px-6 py-4 hover:bg-gray-50">
                <dt className="text-sm font-medium text-gray-500 mb-1">Last Active</dt>
                <dd className="text-sm text-gray-900">
                  {new Date(profile.lastActiveAt).toLocaleDateString('en-US', { 
                    month: 'short',
                    day: 'numeric',
                    year: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                  })}
                  <span className="text-gray-500 ml-2">({formatDistanceToNow(new Date(profile.lastActiveAt), {addSuffix: true})})</span>
                </dd>
              </div>
              <div className="px-6 py-4 hover:bg-gray-50">
                <dt className="text-sm font-medium text-gray-500 mb-1">Metrics Last Calculated</dt>
                <dd className="text-sm text-gray-900">
                  {new Date(metrics.lastCalculatedAt).toLocaleDateString('en-US', { 
                    month: 'short',
                    day: 'numeric',
                    year: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                  })}
                </dd>
              </div>
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
