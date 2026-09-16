import { useState, useEffect } from 'react';
import { graphqlClient } from '~/lib/graphql';
import { Activity, BarChart2, Info } from 'lucide-react';

type MetricsRange = '7d' | '30d' | '90d';

export default function DeliveryHealthTab({ refKey, type }: { refKey: string; type: string }) {
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
        const result = await graphqlClient.getDeliveryHealthMetrics({ refKey, type, range });
        // deliveryHealth is returned as a JSON string by the backend — parse it
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

  const formatValue = (val: any, isPercentage?: boolean, isMs?: boolean) => {
    if (val === null || val === undefined) return '—';
    if (typeof val === 'number') {
      const formatted = Number.isInteger(val) ? val.toLocaleString() : val.toFixed(2);
      if (isPercentage) return `${formatted}%`;
      if (isMs) return `${formatted} ms`;
      return formatted;
    }
    return String(val);
  };

  const statCards = data ? [
    { label: 'Total Threads', value: data.total_thread_count?.value, isPercentage: false, description: data.total_thread_count?.description || 'Volume of activity for this entity over time. Simple count of all threads.' },
    { label: 'Success Rate', value: data.success_rate?.value, isPercentage: true, description: data.success_rate?.description || 'Proportion of threads that completed successfully.' },
    { label: 'Overall Failure Rate', value: data.overall_failure_rate?.value, isPercentage: true, description: data.overall_failure_rate?.description || 'Proportion of threads that failed or were cancelled. Represents delivery failures.' },
    { label: 'Error Rate', value: data.error_rate?.value, isPercentage: true, description: data.error_rate?.description || 'System errors specifically, separate from business logic failures. Signals infrastructure problems.' },
    { label: 'Recovery Rate', value: data.recovery_rate?.value, isPercentage: true, description: data.recovery_rate?.description || 'When delivery fails for this entity, how often do they eventually succeed on retry.' },
    { label: 'Step Failure Breadth', value: data.step_failure_breadth?.value, isPercentage: true, description: data.step_failure_breadth?.description || 'How widespread failures are across this entity\'s process. High ratio = failures spreading.' },
    { label: 'Avg Duration', value: data.average_thread_duration?.value, isMs: true, description: data.average_thread_duration?.description || 'How long delivery typically takes for this entity (completed threads only).' },
  ] : [];

  return (
    <div className="space-y-8">
      {/* Range selector & Notice */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-2">
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
        <div className="flex items-center gap-1.5 text-xs text-gray-500 italic">
          <Activity className="w-3.5 h-3.5" />
          <span>Delivery Health are computed asynchronously and may take up to 30 mins to update.</span>
        </div>
      </div>

      {isLoading && (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center text-gray-500 flex items-center justify-center gap-2">
          <Activity className="w-5 h-5 animate-pulse" /> Loading delivery health…
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-6 text-red-600 text-sm">
          {error}
        </div>
      )}

      {!isLoading && !error && !data && (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
          <BarChart2 className="w-12 h-12 text-gray-300 mb-3" />
          <h3 className="text-base font-medium text-gray-900">No data available yet</h3>
          <p className="text-sm text-gray-500 mt-1 max-w-sm">
            There is no activity data for this entity in the selected time range.
          </p>
        </div>
      )}

      {!isLoading && !error && data && (
        <div className="space-y-6">
          {/* Health Score Banner */}
          {data.health_score?.value !== undefined && (
            <div className={`rounded-xl border-2 p-6 transition-all ${
              data.health_score.status === 'Healthy' ? 'bg-emerald-50 border-emerald-500' :
              data.health_score.status === 'At Risk' ? 'bg-yellow-50 border-yellow-500' :
              data.health_score.status === 'Degraded' ? 'bg-orange-50 border-orange-500' :
              'bg-red-50 border-red-500'
            }`}>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className={`w-3 h-3 rounded-full ${
                    data.health_score.status === 'Healthy' ? 'bg-emerald-500' :
                    data.health_score.status === 'At Risk' ? 'bg-yellow-500' :
                    data.health_score.status === 'Degraded' ? 'bg-orange-500' :
                    'bg-red-500'
                  }`} />
                  <div>
                    <div className="flex items-center gap-3">
                      <h2 className={`text-2xl font-bold ${
                        data.health_score.status === 'Healthy' ? 'text-emerald-900' :
                        data.health_score.status === 'At Risk' ? 'text-yellow-900' :
                        data.health_score.status === 'Degraded' ? 'text-orange-900' :
                        'text-red-900'
                      }`}>
                        {data.health_score.status}
                      </h2>
                      <span className={`text-3xl font-black tabular-nums ${
                        data.health_score.status === 'Healthy' ? 'text-emerald-700' :
                        data.health_score.status === 'At Risk' ? 'text-yellow-700' :
                        data.health_score.status === 'Degraded' ? 'text-orange-700' :
                        'text-red-700'
                      }`}>
                        {Math.round(data.health_score.value)}/100
                      </span>
                      <span className={`text-sm font-medium px-2 py-1 rounded ${
                        data.health_score.trend === 'Stable' ? 'bg-blue-100 text-blue-700' :
                        data.health_score.trend === 'Improving' ? 'bg-green-100 text-green-700' :
                        data.health_score.trend === 'Declining' ? 'bg-red-100 text-red-700' :
                        'bg-gray-100 text-gray-700'
                      }`}>
                        {data.health_score.trend}
                      </span>
                    </div>
                    <p className={`text-xs mt-1 ${
                      data.health_score.status === 'Healthy' ? 'text-emerald-700' :
                      data.health_score.status === 'At Risk' ? 'text-yellow-700' :
                      data.health_score.status === 'Degraded' ? 'text-orange-700' :
                      'text-red-700'
                    }`}>
                      {data.health_score.description}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Scalar Cards */}
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            {statCards.map((stat, idx) => (
              <div key={idx} className="bg-white border border-gray-200 rounded-xl shadow-sm p-5 flex flex-col">
                <div className="flex items-center gap-1.5 mb-2">
                  <span className="text-[10px] font-bold text-gray-400 uppercase tracking-tight">{stat.label}</span>
                  <span title={stat.description}><Info className="w-3.5 h-3.5 text-gray-300 cursor-help hover:text-gray-500 transition-colors" /></span>
                </div>
                <span className="text-2xl font-bold text-gray-900 tabular-nums tracking-tight">
                  {formatValue(stat.value, stat.isPercentage, stat.isMs)}
                </span>
              </div>
            ))}
          </div>

          {/* Most Common Errors Table */}
          {data.most_common_errors?.value && (
            <div className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden max-w-full">
              <div className="px-5 py-3.5 border-b border-gray-100 bg-gray-50/80 flex items-center gap-1.5">
                <h2 className="text-[11px] font-black text-gray-800 uppercase tracking-[0.15em]">
                  Most Common Errors
                </h2>
                <span title={data.most_common_errors?.description || "Which error messages surface most frequently for this entity."}><Info className="w-3.5 h-3.5 text-gray-400 cursor-help hover:text-gray-600 transition-colors" /></span>
              </div>
              
              <div className="p-4">
                {Array.isArray(data.most_common_errors.value) && data.most_common_errors.value.length > 0 ? (
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm text-left">
                      <thead>
                        <tr className="border-b border-gray-100">
                          <th className="py-2 pr-4 text-[10px] font-bold text-gray-400 uppercase tracking-tighter">Error Message</th>
                          <th className="py-2 pr-4 text-[10px] font-bold text-gray-400 uppercase tracking-tighter text-right">Count</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-50">
                        {data.most_common_errors.value.map((errItem: any, idx: number) => (
                          <tr key={idx} className="hover:bg-gray-50/50 transition-colors">
                            <td className="py-3 pr-4 text-gray-900 font-mono text-[11px] break-all max-w-[500px]">
                              {errItem.error_message || '—'}
                            </td>
                            <td className="py-3 pr-4 text-gray-900 font-mono text-[11px] text-right">
                              {formatValue(errItem.count)}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p className="text-xs text-gray-400 italic text-center py-4">No errors recorded in this time range.</p>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
