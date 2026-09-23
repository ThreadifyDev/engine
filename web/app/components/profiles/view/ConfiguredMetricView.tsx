import { useQuery } from '@tanstack/react-query';
import { graphqlClient } from '~/lib/graphql';
import type { ProfileView } from './profile-view';

const textValue = (value: unknown) => value == null ? '—' : typeof value === 'object' ? JSON.stringify(value) : String(value);
type Row = Record<string, unknown>;

export default function ConfiguredMetricView({ metricId, display, refKey, type, range, companyId }: { companyId: string; metricId: string; display: NonNullable<ProfileView['blocks'][number]['display']>; refKey: string; type: string; range: string }) {
  // All metric blocks share one authenticated request for the current entity.
  const query = useQuery({ queryKey: ['profile-metric-presentations', companyId, type, refKey, range], queryFn: () => graphqlClient.getComputedMetrics({ refKey, type, range }), staleTime: 30000 });
  if (query.isPending) return <p className="text-sm text-gray-400">Loading metric…</p>;
  if (query.isError) return <p role="alert" className="text-sm text-red-600">Could not load this metric.</p>;
  let data;
  try { data = typeof query.data === 'string' ? JSON.parse(query.data) : query.data; }
  catch { return <p role="alert" className="text-sm text-red-600">The metric returned invalid data.</p>; }
  const rows = data?.__metricResults?.[metricId] as Row[] | undefined;
  if (!Array.isArray(rows) || rows.some(row => !row || typeof row !== 'object' || Array.isArray(row))) return <p className="text-sm text-gray-500">This metric is unavailable or has been removed from the profile configuration.</p>;
  if (!rows.length) return <p className="text-sm text-gray-500">No results for this metric in the selected period.</p>;
  const columns = [...new Set(rows.flatMap(row => Object.keys(row)))];
  if (display === 'card' && rows.length === 1) return <dl className="grid grid-cols-2 gap-4">{Object.entries(rows[0]).map(([key, value]) => <div key={key}><dt className="text-xs text-gray-500">{key.replaceAll('_', ' ')}</dt><dd className="mt-2 break-words text-2xl font-semibold tabular-nums">{textValue(value)}</dd></div>)}</dl>;
  if (display === 'line' || display === 'bar') {
    const category = columns.find(key => rows.some(row => typeof row[key] === 'string'));
    const values = columns.filter(key => rows.some(row => typeof row[key] === 'number') && rows.every(row => row[key] == null || typeof row[key] === 'number'));
    if (category && values.length && rows.length <= 100) return <MetricChart rows={rows} category={category} values={values} kind={display} />;
  }
  return <div className="overflow-auto">
    {display !== 'table' && <p className="mb-3 text-xs text-gray-500">These results are best shown as a table. Choose a grouped metric for charts or a single result for a card.</p>}
    <table className="w-full text-left text-xs"><thead><tr>{columns.map(key => <th key={key} className="border-b px-3 py-2 font-medium text-gray-500">{key.replaceAll('_', ' ')}</th>)}</tr></thead><tbody>{rows.map((row, index) => <tr key={index}>{columns.map(key => <td key={key} className="border-b border-gray-100 px-3 py-2 tabular-nums">{textValue(row[key])}</td>)}</tr>)}</tbody></table>
  </div>;
}

function MetricChart({ rows, category, values, kind }: { rows: Row[]; category: string; values: string[]; kind: 'line' | 'bar' }) {
  const colors = ['#059669', '#e11d48', '#0284c7', '#7c3aed', '#d97706'];
  const numbers = rows.flatMap(row => values.map(key => row[key]).filter((v): v is number => typeof v === 'number' && Number.isFinite(v)));
  const min = Math.min(0, ...numbers), max = Math.max(1, ...numbers), spread = max - min;
  const x = (i: number) => 55 + (i + 0.5) / rows.length * 590;
  const y = (value: number) => 225 - (value - min) / spread * 180;
  const label = (row: Row) => /^\d{4}-\d{2}-\d{2}T/.test(String(row[category])) ? String(row[category]).slice(0, 10) : textValue(row[category]);
  return <figure aria-label={`${kind} chart of configured metric`}>
    <figcaption className="mb-3 flex flex-wrap gap-4 text-xs">{values.map((key, i) => <span key={key} style={{ color: colors[i % colors.length] }}>{key.replaceAll('_', ' ')}</span>)}</figcaption>
    <svg viewBox="0 0 700 280" className="w-full" role="img" aria-label={`${values.join(', ')} by ${category}`}>
      {[0, 1, 2, 3, 4].map(i => { const value = min + spread * i / 4; return <g key={i}><line x1="55" x2="645" y1={y(value)} y2={y(value)} stroke="#e5e7eb" /><text x="45" y={y(value) + 4} textAnchor="end" fontSize="11" fill="#6b7280">{Number(value.toFixed(2))}</text></g>; })}
      {values.map((key, series) => <g key={key} fill={colors[series % colors.length]} stroke={colors[series % colors.length]}>
        {kind === 'line' && <path d={rows.map((row, i) => typeof row[key] === 'number' && Number.isFinite(row[key]) ? `${i === 0 || typeof rows[i - 1][key] !== 'number' ? 'M' : 'L'}${x(i)},${y(row[key] as number)}` : '').join(' ')} fill="none" strokeWidth="2" />}
        {rows.map((row, i) => {
          const value = row[key]; if (typeof value !== 'number' || !Number.isFinite(value)) return null;
          const width = Math.min(30, 500 / rows.length / values.length);
          return kind === 'line' ? <circle key={i} cx={x(i)} cy={y(value)} r="3"><title>{label(row)} · {key}: {value}</title></circle> : <rect key={i} x={x(i) + (series - values.length / 2) * width} y={Math.min(y(value), y(0))} width={width - 1} height={Math.max(1, Math.abs(y(value) - y(0)))}><title>{label(row)} · {key}: {value}</title></rect>;
        })}
      </g>)}
      {rows.map((row, i) => i % Math.max(1, Math.ceil(rows.length / 5)) === 0 ? <text key={i} x={x(i)} y="250" textAnchor="middle" fontSize="10" fill="#6b7280">{label(row).slice(0, 16)}</text> : null)}
    </svg>
    <details className="mt-2 text-xs text-gray-500"><summary className="cursor-pointer">View values</summary><ul className="mt-2 max-h-48 overflow-auto">{rows.map((row, i) => <li key={i}>{label(row)}: {values.map(key => `${key}: ${textValue(row[key])}`).join(' · ')}</li>)}</ul></details>
  </figure>;
}
