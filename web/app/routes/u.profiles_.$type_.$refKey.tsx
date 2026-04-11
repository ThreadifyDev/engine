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
      <div className="p-8 px-4 sm:px-6 lg:px-8 max-w-7xl mx-auto">
        <button 
          onClick={() => navigate('/u/profiles')}
          className="text-gray-500 hover:text-black mb-6 flex items-center gap-2 text-sm font-medium transition-colors"
        >
          <ChevronLeft className="w-4 h-4" /> Back to Search
        </button>

        {/* Header Block */}
        <div className="bg-gradient-to-br from-gray-900 to-black rounded-2xl p-8 mb-8 text-white shadow-lg relative overflow-hidden">
          <div className="absolute top-0 right-0 w-64 h-64 bg-white opacity-5 rounded-full blur-3xl -translate-y-1/2 translate-x-1/3"></div>
          <div className="flex items-start justify-between relative z-10">
            <div>
              <div className="flex items-center gap-3 mb-2">
                <div className="w-10 h-10 rounded-lg outline outline-1 outline-white/20 flex items-center justify-center bg-white/10 backdrop-blur">
                  <UserCircle className="w-6 h-6 text-white" />
                </div>
                <div>
                  <h1 className="text-3xl font-bold">{profile.name || refKey}</h1>
                  <p className="text-gray-400 text-sm font-mono flex items-center gap-2 mt-1">
                    <span className="px-2 py-0.5 rounded bg-white/10 text-white/90">{type}</span>
                    <span className="select-all opacity-80">{refKey}</span>
                  </p>
                </div>
              </div>
            </div>
            
            <div className="text-right flex flex-col items-end">
              <p className="text-sm font-medium text-gray-400 mb-1 tracking-wide uppercase">Health Score</p>
              <div className="flex items-end gap-3">
                <span className="text-5xl font-black tabular-nums">{Math.round(metrics.deliveryHealthScore)}</span>
                <div className={`flex items-center gap-1 font-bold ${metrics.healthTrendSlope >= 0 ? 'text-green-400' : 'text-red-400'} pb-1`}>
                  {metrics.healthTrendSlope >= 0 ? <TrendingUp className="w-5 h-5" /> : <TrendingDown className="w-5 h-5" />}
                  <span>{Math.abs(metrics.healthTrendSlope).toFixed(1)}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Overview Stats */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
          <div className="bg-white p-5 rounded-xl border border-gray-200 shadow-sm">
            <div className="flex items-center gap-2 text-gray-500 mb-2 font-medium">
              <Archive className="w-4 h-4" /> Total Activities
            </div>
            <div className="text-3xl font-black text-gray-900">{metrics.totalDeliveries}</div>
          </div>
          
          <div className="bg-white p-5 rounded-xl border border-gray-200 shadow-sm">
            <div className="flex items-center gap-2 text-green-600 mb-2 font-medium">
              <CheckCircle2 className="w-4 h-4" /> Successful
            </div>
            <div className="text-3xl font-black text-green-700">{metrics.completedSuccessfully}</div>
          </div>

          <div className="bg-white p-5 rounded-xl border border-gray-200 shadow-sm">
            <div className="flex items-center gap-2 text-red-500 mb-2 font-medium">
              <AlertTriangle className="w-4 h-4" /> Violations
            </div>
            <div className="text-3xl font-black text-red-600">{metrics.validationViolations}</div>
          </div>

          <div className="bg-white p-5 rounded-xl border border-gray-200 shadow-sm">
            <div className="flex items-center gap-2 text-blue-500 mb-2 font-medium">
              <Clock className="w-4 h-4" /> Avg Duration
            </div>
            <div className="text-3xl font-black text-blue-600">
              {metrics.averageDeliveryTimeMs > 0 
                ? `${(metrics.averageDeliveryTimeMs / 1000).toFixed(1)}s` 
                : 'N/A'
              }
            </div>
          </div>
        </div>

        {/* Deep Dive Information */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <div className="bg-white border border-gray-200 rounded-xl overflow-hidden shadow-sm">
            <div className="px-6 py-4 border-b border-gray-200 bg-gray-50">
              <h3 className="font-bold text-gray-900 flex items-center gap-2">
                <Target className="w-5 h-5 text-gray-500" /> Profiling Metadata
              </h3>
            </div>
            <div className="p-6">
              <dl className="space-y-4">
                <div>
                  <dt className="text-sm font-medium text-gray-500">Record ID</dt>
                  <dd className="mt-1 font-mono text-sm text-gray-900">{profile.id}</dd>
                </div>
                <div>
                  <dt className="text-sm font-medium text-gray-500">First Observed</dt>
                  <dd className="mt-1 text-sm text-gray-900">
                    {new Date(profile.createdAt).toLocaleString()} 
                    <span className="text-gray-400 ml-2">({formatDistanceToNow(new Date(profile.createdAt), {addSuffix: true})})</span>
                  </dd>
                </div>
                <div>
                  <dt className="text-sm font-medium text-gray-500">Last Actively Tracked</dt>
                  <dd className="mt-1 text-sm text-gray-900">
                    {new Date(profile.lastActiveAt).toLocaleString()}
                    <span className="text-gray-400 ml-2">({formatDistanceToNow(new Date(profile.lastActiveAt), {addSuffix: true})})</span>
                  </dd>
                </div>
                <div>
                  <dt className="text-sm font-medium text-gray-500">Metric Calculation Epoch</dt>
                  <dd className="mt-1 text-sm text-gray-900">
                    {new Date(metrics.lastCalculatedAt).toLocaleString()}
                  </dd>
                </div>
              </dl>
            </div>
          </div>

          <div className="bg-white border border-gray-200 rounded-xl overflow-hidden shadow-sm flex flex-col justify-center items-center p-12 text-center text-gray-500">
            <LayoutDashboard className="w-12 h-12 text-gray-300 mb-4" />
            <h3 className="text-lg font-bold text-gray-900 mb-2">Extended Timeline</h3>
            <p className="text-sm max-w-sm">
              In-depth visualization of historical thread runs and validation logs for this entity will be presented here in future iterations.
            </p>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
