import { useState, useEffect, useMemo } from 'react';
import { graphqlClient } from '~/lib/graphql';
import { Activity, AlertTriangle, BarChart2, CheckCircle2, Clock, RefreshCw, ShieldAlert } from 'lucide-react';

type MetricsRange = '7d' | '30d' | '90d';

const METRIC_THEMES = {
  positive: {
    accent: 'border-t-emerald-400',
    icon: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
  },
  negative: {
    accent: 'border-t-rose-400',
    icon: 'bg-rose-50 text-rose-600 ring-rose-100',
  },
  warning: {
    accent: 'border-t-amber-400',
    icon: 'bg-amber-50 text-amber-700 ring-amber-100',
  },
  activity: {
    accent: 'border-t-sky-400',
    icon: 'bg-sky-50 text-sky-600 ring-sky-100',
  },
  neutral: {
    accent: 'border-t-violet-400',
    icon: 'bg-violet-50 text-violet-600 ring-violet-100',
  },
} as const;

function getMetricTheme(name: string) {
  const normalized = name.toLowerCase();
  if (/failure|failed|error|violation/.test(normalized)) return METRIC_THEMES.negative;
  if (/completion|success|completed/.test(normalized)) return METRIC_THEMES.positive;
  if (/retry|duration|time|latency/.test(normalized)) return METRIC_THEMES.warning;
  if (/activity|volume|count|execution/.test(normalized)) return METRIC_THEMES.activity;
  return METRIC_THEMES.neutral;
}

function getMetricIcon(name: string) {
  const normalized = name.toLowerCase();
  if (/violation|error/.test(normalized)) return ShieldAlert;
  if (/failure|failed/.test(normalized)) return AlertTriangle;
  if (/completion|success/.test(normalized)) return CheckCircle2;
  if (/retry/.test(normalized)) return RefreshCw;
  if (/duration|time|latency/.test(normalized)) return Clock;
  return BarChart2;
}

function getStatusStyle(status: string) {
  const normalized = status.replace(/^status:\s*/i, '').toLowerCase();
  if (/completed|success|passed/.test(normalized)) return 'border-emerald-200 bg-emerald-50 text-emerald-700';
  if (/failed|error|critical|violated/.test(normalized)) return 'border-rose-200 bg-rose-50 text-rose-700';
  if (/active|running|progress/.test(normalized)) return 'border-sky-200 bg-sky-50 text-sky-700';
  if (/warning|pending|retry/.test(normalized)) return 'border-amber-200 bg-amber-50 text-amber-700';
  return 'border-gray-200 bg-gray-50 text-gray-600';
}

function humanizeLabel(value: string) {
  return value
    .replace(/^status:\s*/i, '')
    .replace(/[_:]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

export default function MetricsTab({ refKey, type, hasMetricsConfig, initialRange = '7d', selectedRange, onRangeChange }: { refKey: string; type: string; hasMetricsConfig: boolean; initialRange?: MetricsRange; selectedRange?: MetricsRange; onRangeChange?: (range: MetricsRange) => void }) {
  const [localRange, setLocalRange] = useState<MetricsRange>(initialRange);
  const range = selectedRange ?? localRange;
  const setRange = onRangeChange ?? setLocalRange;
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
      if (metricName === '__metricResults') return;
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
    <div className="min-w-0 space-y-5 sm:space-y-6">
      {/* Range selector & Notice */}
      <div className="flex flex-col gap-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3 sm:flex-row sm:items-center sm:justify-between sm:p-4">
        <div className="grid grid-cols-3 rounded-lg bg-gray-200/70 p-1 sm:inline-grid">
          {(['7d', '30d', '90d'] as MetricsRange[]).map(r => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={`min-w-0 rounded-md px-4 py-1.5 text-sm font-semibold transition-all ${
                range === r
                  ? 'bg-white text-gray-950 shadow-sm ring-1 ring-black/5'
                  : 'text-gray-500 hover:text-gray-900'
              }`}
            >
              {r}
            </button>
          ))}
        </div>
        <div className="flex min-w-0 items-start gap-2 text-xs leading-5 text-gray-500 sm:max-w-sm sm:items-center sm:text-right">
          <Activity className="mt-0.5 h-4 w-4 flex-none text-gray-400 sm:mt-0" />
          <span>Metrics may take up to 30 minutes to reflect new activity.</span>
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
        <div className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2 xl:gap-5">
          {Object.entries(groupedData).map(([tagline, statuses]) => {
            const theme = getMetricTheme(tagline);
            const MetricIcon = getMetricIcon(tagline);
            const hasScalars = Object.values(statuses).some(items =>
              items.some(item => !Array.isArray(item.result) && !(typeof item.result === 'object' && item.result !== null))
            );
            const hasOtherItems = Object.values(statuses).some(items => 
              items.some(i => Array.isArray(i.result) || (typeof i.result === 'object' && i.result !== null))
            );

            return (
              <article
                key={tagline}
                className={`flex h-full min-w-0 flex-col overflow-hidden rounded-2xl border border-gray-200 border-t-[3px] bg-white shadow-sm transition-shadow hover:shadow-md ${theme.accent}`}
              >
                <div className="flex min-h-[64px] items-center gap-3 border-b border-gray-100 bg-gray-50/60 px-4 py-3 sm:px-5">
                  <div className={`flex h-9 w-9 flex-none items-center justify-center rounded-lg ring-1 ${theme.icon}`}>
                    <MetricIcon className="h-[18px] w-[18px]" />
                  </div>
                  <h2 className="min-w-0 text-sm font-semibold leading-5 text-gray-900">
                    {tagline}
                  </h2>
                </div>

                <div className="flex min-h-0 flex-1 flex-col">
                  {hasScalars && (
                    <div className="flex flex-1 flex-col justify-center space-y-4 p-4 sm:p-5">
                      {Object.entries(statuses).map(([status, items]) => {
                        const scalarItems = items.filter(item =>
                          !Array.isArray(item.result) && !(typeof item.result === 'object' && item.result !== null)
                        );
                        if (scalarItems.length === 0) return null;

                        return (
                          <section key={status} className="space-y-2.5">
                            {status !== 'General' && (
                              <span className={`inline-flex rounded-full border px-2 py-0.5 text-[10px] font-semibold capitalize ${getStatusStyle(status)}`}>
                                {humanizeLabel(status)}
                              </span>
                            )}
                            <dl className="grid grid-cols-2 gap-2 sm:[grid-template-columns:repeat(auto-fit,minmax(110px,1fr))]">
                              {scalarItems.map(item => (
                                <div key={item.metricName} className="min-w-0 rounded-xl border border-gray-100 bg-gray-50/70 p-3">
                                  <dt className="break-words text-[10px] font-semibold uppercase leading-4 tracking-wide text-gray-500">
                                    {humanizeLabel(item.header)}
                                  </dt>
                                  <dd className="mt-1 break-words font-mono text-xl font-semibold tabular-nums text-gray-950">
                                    {formatValue(item.result)}
                                  </dd>
                                </div>
                              ))}
                            </dl>
                          </section>
                        );
                      })}
                    </div>
                  )}

                  {hasOtherItems && (
                    <div className={`grid grid-cols-1 gap-4 p-4 sm:p-5 ${hasScalars ? 'border-t border-gray-100 bg-gray-50/30' : ''}`}>
                      {Object.entries(statuses).map(([status, items]) => {
                        const otherItems = items.filter(i => Array.isArray(i.result) || (typeof i.result === 'object' && i.result !== null));
                        return otherItems.map((item, idx) => {
                          const tableKey = `${tagline}-${status}-${idx}`;
                          const isExpanded = expandedTables[tableKey];
                          const displayData = Array.isArray(item.result) ? (isExpanded ? item.result : item.result.slice(0, 5)) : item.result;

                          return (
                          <section key={tableKey} className="flex min-w-0 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white">
                            {(item.header.toUpperCase() !== tagline.toUpperCase() || status !== 'General') && (
                              <div className="flex min-w-0 items-center justify-between gap-2 border-b border-gray-100 bg-gray-50/60 px-3 py-2.5">
                                {item.header.toUpperCase() !== tagline.toUpperCase() && (
                                  <h3 className="min-w-0 break-words text-xs font-semibold text-gray-700">{item.header}</h3>
                                )}
                                {status !== 'General' && (
                                  <span className={`ml-auto inline-flex flex-none rounded-full border px-2 py-0.5 text-[10px] font-semibold capitalize ${getStatusStyle(status)}`}>
                                    {humanizeLabel(status)}
                                  </span>
                                )}
                              </div>
                            )}
                            <div className="flex min-h-[88px] min-w-0 flex-1 flex-col justify-center p-3 sm:p-4">
                              {Array.isArray(item.result) ? (
                                item.result.length === 0 ? (
                                  <div className="flex flex-col items-center justify-center gap-2 py-3 text-center">
                                    <Activity className="h-5 w-5 text-gray-300" />
                                    <p className="text-xs text-gray-400">No data recorded</p>
                                  </div>
                                ) : (
                                  <>
                                    <div className="space-y-2 sm:hidden">
                                      {displayData.map((row: any, rowIndex: number) => (
                                        <dl key={rowIndex} className="divide-y divide-gray-100 rounded-lg border border-gray-100 bg-gray-50/50 px-3">
                                          {Object.entries(row).map(([column, value]) => (
                                            <div key={column} className="flex min-w-0 items-start justify-between gap-4 py-2">
                                              <dt className="min-w-0 break-words text-[10px] font-semibold uppercase leading-4 tracking-wide text-gray-500">
                                                {humanizeLabel(column)}
                                              </dt>
                                              <dd className="max-w-[60%] break-words text-right font-mono text-xs font-semibold text-gray-900">
                                                {formatValue(value)}
                                              </dd>
                                            </div>
                                          ))}
                                        </dl>
                                      ))}
                                    </div>
                                    <div className="hidden min-w-0 overflow-x-auto sm:block">
                                    <table className="w-full text-left text-sm">
                                      <thead>
                                        <tr className="border-b border-gray-100">
                                          {Object.keys(item.result[0]).map(col => (
                                            <th key={col} className="whitespace-nowrap px-2 py-2 text-[10px] font-semibold uppercase tracking-wide text-gray-500 first:pl-0 last:pr-0">
                                              {humanizeLabel(col)}
                                            </th>
                                          ))}
                                        </tr>
                                      </thead>
                                      <tbody className="divide-y divide-gray-50">
                                        {displayData.map((row: any, i: number) => (
                                          <tr key={i} className="transition-colors hover:bg-gray-50/70">
                                            {Object.values(row).map((val: any, j: number) => (
                                              <td key={j} className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-gray-900 first:pl-0 last:pr-0">
                                                {formatValue(val)}
                                              </td>
                                            ))}
                                          </tr>
                                        ))}
                                      </tbody>
                                    </table>
                                    </div>
                                    {item.result.length > 5 && (
                                      <button
                                        type="button"
                                        onClick={() => toggleTable(tableKey)}
                                        className="mt-3 w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900"
                                      >
                                        {isExpanded ? 'Show less' : `Show ${item.result.length - 5} more`}
                                      </button>
                                    )}
                                  </>
                                )
                              ) : (
                                <dl className="grid grid-cols-2 gap-2 sm:[grid-template-columns:repeat(auto-fit,minmax(110px,1fr))]">
                                  {Object.entries(item.result as Record<string, any>).map(([key, val]) => (
                                    <div key={key} className="min-w-0 rounded-lg bg-gray-50 p-3 text-center">
                                      <dt className="break-words text-[10px] font-semibold uppercase leading-4 tracking-wide text-gray-500">{humanizeLabel(key)}</dt>
                                      <dd className="mt-1 break-words font-mono text-lg font-semibold tabular-nums text-gray-950">
                                        {formatValue(val)}
                                      </dd>
                                    </div>
                                  ))}
                                </dl>
                              )}
                            </div>
                          </section>
                        );
                      });
                    })}
                  </div>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}
    </div>
  );
}
