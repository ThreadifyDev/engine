import { useState, useEffect, useMemo } from 'react';
import { graphqlClient } from '~/lib/graphql';
import { Activity, BarChart2, Calendar, CheckCircle2, Clock } from 'lucide-react';

type MetricsRange = '7d' | '30d' | '90d';

export default function MetricsTab({ refKey, type, hasMetricsConfig }: { refKey: string; type: string; hasMetricsConfig: boolean }) {
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

  const formatValue = (val: any) => {
    if (val === null || val === undefined) return '—';
    if (typeof val === 'number') {
      return Number.isInteger(val) ? val.toLocaleString() : val.toFixed(2);
    }
    if (typeof val === 'string') {
      // ISO Date Check: e.g. 2026-05-02T00:00:00Z
      if (val.length >= 10 && /^\d{4}-\d{2}-\d{2}/.test(val)) {
        const d = new Date(val);
        if (!isNaN(d.getTime())) {
          // If it's exactly at midnight, treat as a date only
          if (val.includes('T00:00:00')) {
            return d.toLocaleDateString(undefined, { 
              month: 'short', 
              day: 'numeric', 
              year: 'numeric' 
            });
          }
          return d.toLocaleString(undefined, { 
            month: 'short', 
            day: 'numeric', 
            hour: '2-digit', 
            minute: '2-digit' 
          });
        }
      }
    }
    return String(val);
  };

  const groupedData = useMemo(() => {
    if (!data) return null;
    const groups: Record<string, Record<string, any[]>> = {};
    
    Object.entries(data as Record<string, any>).forEach(([metricName, result]) => {
      // Backend format is typically: "BaseTagline: Header (Status)"
      // e.g. "OUTCOME RATE: MATCHED THREADS (STATUS: ACTIVE)"
      const parts = metricName.split(' (');
      const mainName = parts[0]; 
      const statusRaw = parts.length > 1 ? parts[1].replace(')', '') : null;
      
      const nameSplit = mainName.split(':');
      let baseTagline = nameSplit[0].trim();
      let header = (nameSplit.length > 1 ? nameSplit.slice(1).join(':') : baseTagline).trim();
      
      // Clean up strings
      baseTagline = baseTagline.split('_').map(w => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ');
      header = header.split('_').map(w => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ');
      
      const statusKey = statusRaw || 'General';
      let statusColorRef = statusKey.replace(/^STATUS:\s*/i, '');
      
      if (!groups[baseTagline]) {
        groups[baseTagline] = {};
      }
      
      if (!groups[baseTagline][statusKey]) {
        groups[baseTagline][statusKey] = [];
      }
      
      groups[baseTagline][statusKey].push({
        metricName,
        header,
        result,
        status: statusRaw,
        statusColorRef
      });
    });
    
    return groups;
  }, [data]);

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
          <span>Metrics are computed asynchronously and may take up to 30 mins to update.</span>
        </div>
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

      {!isLoading && !error && data === null && !hasMetricsConfig && (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
          <BarChart2 className="w-12 h-12 text-gray-300 mb-3" />
          <h3 className="text-base font-medium text-gray-900">No metrics configured</h3>
          <p className="text-sm text-gray-500 mt-1 max-w-sm">
            Add metric templates to this entity profile type to start seeing computed results here.
          </p>
        </div>
      )}

      {!isLoading && !error && data === null && hasMetricsConfig && (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
          <Activity className="w-12 h-12 text-gray-300 mb-3" />
          <h3 className="text-base font-medium text-gray-900">No data available yet</h3>
          <p className="text-sm text-gray-500 mt-1 max-w-sm">
            There is no activity data for this entity in the selected time range.
          </p>
        </div>
      )}

      {!isLoading && !error && groupedData && Object.keys(groupedData).length > 0 && (
        <div className="space-y-12">
          {Object.entries(groupedData).map(([tagline, statuses]) => (
            <div key={tagline} className="animate-in fade-in slide-in-from-bottom-2 duration-500">
              <div className="flex items-center gap-4 mb-6">
                <h2 className="text-xs font-black text-gray-900 uppercase tracking-[0.2em]">
                  {tagline}
                </h2>
                <div className="h-px bg-gray-100 flex-1" />
              </div>
              
              <div className="space-y-8">
                {Object.entries(statuses).map(([status, items]) => (
                  <div key={status} className="space-y-4">
                    {status !== 'General' && (
                      <div className="flex items-center gap-2 px-1">
                        <div className={`w-1.5 h-1.5 rounded-full ${
                          items[0].statusColorRef.toLowerCase() === 'completed' ? 'bg-emerald-500' : 
                          items[0].statusColorRef.toLowerCase() === 'active' ? 'bg-blue-500' : 'bg-gray-400'
                        }`} />
                        <span className="text-[10px] font-bold text-gray-400 uppercase tracking-widest">
                          {status}
                        </span>
                      </div>
                    )}
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                      {items.map((item, idx) => (
                        <div key={idx} className="bg-white border border-gray-200 rounded-xl overflow-hidden flex flex-col shadow-sm hover:shadow-md transition-all duration-200 group">
                          <div className="px-5 py-3 border-b border-gray-50 bg-gray-50/30 group-hover:bg-gray-50/50 transition-colors">
                            <h3 className="text-[10px] font-bold text-gray-600 uppercase tracking-tight">{item.header}</h3>
                          </div>
                          <div className="p-5 flex-1 flex flex-col justify-center min-h-[120px]">
                            {Array.isArray(item.result) ? (
                              item.result.length === 0 ? (
                                <p className="text-xs text-gray-400 italic text-center">No data recorded</p>
                              ) : (
                                <div className="overflow-x-auto">
                                  <table className="w-full text-sm">
                                    <thead>
                                      <tr className="border-b border-gray-100">
                                        {Object.keys(item.result[0]).map(col => (
                                          <th key={col} className="text-left text-[10px] font-bold text-gray-400 pb-2 pr-4 font-mono uppercase tracking-tighter">{col}</th>
                                        ))}
                                      </tr>
                                    </thead>
                                    <tbody className="divide-y divide-gray-50">
                                      {item.result.slice(0, 5).map((row: any, i: number) => (
                                        <tr key={i} className="hover:bg-gray-50/50 transition-colors">
                                          {Object.values(row).map((val: any, j: number) => (
                                            <td key={j} className="py-2 pr-4 text-gray-900 font-mono text-[11px] whitespace-nowrap">
                                              {formatValue(val)}
                                            </td>
                                          ))}
                                        </tr>
                                      ))}
                                      {item.result.length > 5 && (
                                        <tr>
                                          <td colSpan={Object.keys(item.result[0]).length} className="pt-3 text-[10px] text-gray-400 text-center italic">
                                            + {item.result.length - 5} more rows
                                          </td>
                                        </tr>
                                      )}
                                    </tbody>
                                  </table>
                                </div>
                              )
                            ) : (
                              <div className="flex flex-col items-center justify-center py-2">
                                <span className="text-4xl font-bold text-gray-900 tabular-nums tracking-tight">
                                  {formatValue(item.result)}
                                </span>
                              </div>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
