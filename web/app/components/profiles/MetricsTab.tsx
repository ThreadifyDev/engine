import { useState, useEffect, useMemo } from 'react';
import { graphqlClient } from '~/lib/graphql';
import { Activity, BarChart2, Calendar, CheckCircle2, Clock } from 'lucide-react';

type MetricsRange = '7d' | '30d' | '90d';

export default function MetricsTab({ refKey, type, hasMetricsConfig }: { refKey: string; type: string; hasMetricsConfig: boolean }) {
  const [range, setRange] = useState<MetricsRange>('7d');
  const [data, setData] = useState<any>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedTables, setExpandedTables] = useState<Record<string, boolean>>({});

  const toggleTable = (key: string) => {
    setExpandedTables(prev => ({
      ...prev,
      [key]: !prev[key]
    }));
  };

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
        <div className="flex flex-wrap items-start gap-6">
          {Object.entries(groupedData).map(([tagline, statuses]) => {
            // Find all unique scalar headers
            const scalarHeadersSet = new Set<string>();
            let hasMultipleStatuses = false;
            let statusCount = 0;
            
            Object.entries(statuses).forEach(([status, items]) => {
              if (status !== 'General') {
                hasMultipleStatuses = true;
              }
              statusCount++;
              items.forEach(item => {
                if (!Array.isArray(item.result) && !(typeof item.result === 'object' && item.result !== null)) {
                  scalarHeadersSet.add(item.header);
                }
              });
            });
            
            const scalarHeaders = Array.from(scalarHeadersSet);
            const showStatusColumn = hasMultipleStatuses || (!statuses['General'] && statusCount > 0);
            
            const hasScalars = scalarHeaders.length > 0;
            const hasOtherItems = Object.values(statuses).some(items => 
              items.some(i => Array.isArray(i.result) || (typeof i.result === 'object' && i.result !== null))
            );

            return (
              <div key={tagline} className="bg-white border border-gray-200 rounded-xl shadow-sm hover:shadow-md transition-all duration-200 overflow-hidden animate-in fade-in slide-in-from-bottom-2 duration-500 max-w-full">
                {/* Metric Name as Card Header */}
                <div className="px-5 py-3.5 border-b border-gray-100 bg-gray-50/80">
                  <h2 className="text-[11px] font-black text-gray-800 uppercase tracking-[0.15em]">
                    {tagline}
                  </h2>
                </div>

                <div className="flex flex-col">
                  {/* Scalar Metrics Table */}
                  {hasScalars && (
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm text-left whitespace-nowrap">
                        <thead>
                          <tr className="border-b border-gray-100 bg-white">
                            {showStatusColumn && (
                              <th className="py-3 px-5 text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
                                Group / Status
                              </th>
                            )}
                            {scalarHeaders.map(h => (
                              <th key={h} className="py-3 px-5 text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
                                {h}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-50">
                          {Object.entries(statuses).map(([status, items]) => {
                            const scalarItems = items.filter(i => !Array.isArray(i.result) && !(typeof i.result === 'object' && i.result !== null));
                            if (scalarItems.length === 0) return null;
                            
                            const itemsByHeader = new Map(scalarItems.map(i => [i.header, i]));
                            const firstItem = scalarItems[0];
                            const statusColor = firstItem?.statusColorRef?.toLowerCase() || '';

                            return (
                              <tr key={status} className="hover:bg-gray-50/50 transition-colors bg-white">
                                {showStatusColumn && (
                                  <td className="py-3 px-5">
                                    {status !== 'General' ? (
                                      <div className="flex items-center gap-2">
                                        <div className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${
                                          statusColor === 'completed' ? 'bg-emerald-500' :
                                          statusColor === 'active' ? 'bg-blue-500' : 'bg-gray-400'
                                        }`} />
                                        <span className="text-[11px] font-bold text-gray-600 uppercase tracking-tight">{status}</span>
                                      </div>
                                    ) : (
                                      <span className="text-[11px] font-bold text-gray-400 uppercase tracking-tight">GENERAL</span>
                                    )}
                                  </td>
                                )}
                                {scalarHeaders.map(h => {
                                  const item = itemsByHeader.get(h);
                                  return (
                                    <td key={h} className="py-3 px-5 text-gray-900 font-mono text-[13px]">
                                      {item ? formatValue(item.result) : '—'}
                                    </td>
                                  );
                                })}
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  )}

                  {/* Other Items (Arrays / Objects) */}
                  {hasOtherItems && (
                    <div className={`p-5 grid grid-cols-1 gap-5 ${hasScalars ? 'border-t border-gray-100 bg-gray-50/30' : 'bg-white'}`}>
                      {Object.entries(statuses).map(([status, items]) => {
                        const otherItems = items.filter(i => Array.isArray(i.result) || (typeof i.result === 'object' && i.result !== null));
                        return otherItems.map((item, idx) => {
                          const tableKey = `${tagline}-${status}-${idx}`;
                          const isExpanded = expandedTables[tableKey];
                          const displayData = Array.isArray(item.result) ? (isExpanded ? item.result : item.result.slice(0, 5)) : item.result;

                          return (
                          <div key={tableKey} className="bg-white border border-gray-200 rounded-lg overflow-hidden flex flex-col shadow-sm">
                            {(item.header.toUpperCase() !== tagline.toUpperCase() || status !== 'General') && (
                              <div className="px-4 py-2 border-b border-gray-100 bg-gray-50/50 flex items-center justify-between">
                                {item.header.toUpperCase() !== tagline.toUpperCase() && (
                                  <h3 className="text-[10px] font-bold text-gray-600 uppercase tracking-tight">{item.header}</h3>
                                )}
                                {status !== 'General' && (
                                  <span className={`text-[9px] font-bold text-gray-500 uppercase bg-white border border-gray-200 px-1.5 py-0.5 rounded ${item.header.toUpperCase() === tagline.toUpperCase() ? 'ml-0' : 'ml-auto'}`}>
                                    {status}
                                  </span>
                                )}
                              </div>
                            )}
                            <div className="p-4 flex-1 flex flex-col justify-center min-h-[60px]">
                              {Array.isArray(item.result) ? (
                                item.result.length === 0 ? (
                                  <p className="text-xs text-gray-400 italic text-center">No data recorded</p>
                                ) : (
                                  <div className="overflow-x-auto">
                                    <table className="w-full text-sm text-left">
                                      <thead>
                                        <tr className="border-b border-gray-100">
                                          {Object.keys(item.result[0]).map(col => (
                                            <th key={col} className="py-2 pr-4 text-[10px] font-bold text-gray-400 uppercase tracking-tighter whitespace-nowrap">{col}</th>
                                          ))}
                                        </tr>
                                      </thead>
                                      <tbody className="divide-y divide-gray-50">
                                        {displayData.map((row: any, i: number) => (
                                          <tr key={i} className="hover:bg-gray-50/50 transition-colors">
                                            {Object.values(row).map((val: any, j: number) => (
                                              <td key={j} className="py-2 pr-4 text-gray-900 font-mono text-[11px] whitespace-nowrap">
                                                {formatValue(val)}
                                              </td>
                                            ))}
                                          </tr>
                                        ))}
                                        {item.result.length > 5 && !isExpanded && (
                                          <tr>
                                            <td 
                                              colSpan={Object.keys(item.result[0]).length} 
                                              className="pt-3 pb-1 text-[10px] text-gray-400 text-center italic cursor-pointer hover:text-gray-600 transition-colors"
                                              onClick={() => toggleTable(tableKey)}
                                            >
                                              + {item.result.length - 5} more rows (click to expand)
                                            </td>
                                          </tr>
                                        )}
                                        {item.result.length > 5 && isExpanded && (
                                          <tr>
                                            <td 
                                              colSpan={Object.keys(item.result[0]).length} 
                                              className="pt-3 pb-1 text-[10px] text-gray-400 text-center italic cursor-pointer hover:text-gray-600 transition-colors"
                                              onClick={() => toggleTable(tableKey)}
                                            >
                                              Show less
                                            </td>
                                          </tr>
                                        )}
                                      </tbody>
                                    </table>
                                  </div>
                                )
                              ) : (
                                <div className="flex flex-row flex-wrap gap-4 justify-center">
                                  {Object.entries(item.result as Record<string, any>).map(([key, val]) => (
                                    <div key={key} className="flex flex-col items-center justify-center">
                                      <span className="text-[10px] font-bold text-gray-400 uppercase tracking-tight mb-1">{key}</span>
                                      <span className="text-lg font-bold text-gray-900 tabular-nums tracking-tight">
                                        {formatValue(val)}
                                      </span>
                                    </div>
                                  ))}
                                </div>
                              )}
                            </div>
                          </div>
                        );
                      });
                    })}
                  </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
