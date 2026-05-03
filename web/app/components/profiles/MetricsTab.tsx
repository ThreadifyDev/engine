import { useState, useEffect } from 'react';
import { graphqlClient } from '~/lib/graphql';
import { Activity, BarChart2 } from 'lucide-react';

type MetricsRange = '7d' | '30d' | '90d';

export default function MetricsTab({ refKey, type }: { refKey: string; type: string }) {
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
