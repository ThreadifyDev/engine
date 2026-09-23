import { useState } from 'react';

type Day = { date: string; completed: number; failed: number; total: number };
export function parseDeliveryTrend(value: unknown): Day[] | null {
  if (!Array.isArray(value) || !value.length || value.length > 90) return null;
  const days: Day[] = [];
  for (const row of value) {
    if (!row || typeof row.date !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(row.date)) return null;
    const timestamp = Date.parse(`${row.date}T00:00:00Z`);
    if (!Number.isFinite(timestamp) || new Date(timestamp).toISOString().slice(0, 10) !== row.date) return null;
    if (days.length && timestamp - Date.parse(`${days[days.length - 1].date}T00:00:00Z`) !== 86400000) return null;
    if (![row.completed, row.failed, row.total].every(n => Number.isSafeInteger(n) && n >= 0)) return null;
    if (row.completed + row.failed > row.total) return null;
    days.push({ date: row.date, completed: row.completed, failed: row.failed, total: row.total });
  }
  return days;
}

const labelDate = (date: string) => new Date(`${date}T00:00:00Z`).toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });

export default function DeliveryTrendChart({ data }: { data: unknown }) {
  const days = parseDeliveryTrend(data);
  const [selected, setSelected] = useState<number | null>(null);
  if (!days) return <p className="rounded-xl bg-stone-50 p-6 text-sm text-stone-500">Daily outcome data is unavailable. Try again after the metrics refresh.</p>;
  const completed = days.reduce((n, d) => n + d.completed, 0);
  const failed = days.reduce((n, d) => n + d.failed, 0);
  const total = days.reduce((n, d) => n + d.total, 0);
  const max = Math.max(2, ...days.flatMap(d => [d.completed, d.failed]));
  const step = Math.max(1, Math.ceil(max / 4));
  const ceiling = Math.ceil(max / step) * step;
  const x = (i: number) => 50 + i / Math.max(1, days.length - 1) * 610;
  const y = (n: number) => 242 - n / ceiling * 200;
  const ticks = Array.from({ length: ceiling / step + 1 }, (_, i) => i * step);
  const indices = [...new Set([0, Math.floor((days.length - 1) / 3), Math.floor(2 * (days.length - 1) / 3), days.length - 1])];
  const active = days[Math.min(selected ?? days.length - 1, days.length - 1)];

  return <figure aria-label="Completed and failed deliveries over time" className="rounded-xl border border-stone-100 bg-stone-50/50 p-4 sm:p-6">
    <figcaption className="flex flex-wrap items-start justify-between gap-4">
      <div><p className="text-sm font-semibold text-stone-900">Delivery outcomes over time</p><p className="mt-1 text-xs text-stone-500">{labelDate(days[0].date)} – {labelDate(days[days.length - 1].date)} · UTC</p></div>
      <div className="flex flex-wrap gap-4 text-xs">
        <span className="flex items-center gap-2"><span aria-hidden="true" className="w-5 border-t-[3px] border-emerald-600" />Completed <strong>{completed}</strong></span>
        <span className="flex items-center gap-2"><span aria-hidden="true" className="w-5 border-t-[3px] border-dashed border-rose-500" />Failed / cancelled <strong>{failed}</strong></span>
      </div>
    </figcaption>
    <svg viewBox="0 0 710 290" className="mt-4 w-full overflow-visible" role="img" aria-label={`Line graph: ${completed} completed and ${failed} failed or cancelled threads. X axis: UTC creation date. Y axis: thread count.`}>
      <text x="50" y="20" fontSize="11" fill="#78716c">Threads</text>
      {ticks.map(tick => <g key={tick}><line x1="50" x2="660" y1={y(tick)} y2={y(tick)} stroke="#e7e5e4" /><text x="36" y={y(tick) + 4} textAnchor="end" fontSize="11" fill="#78716c">{tick}</text></g>)}
      {indices.map(i => <text key={i} x={x(i)} y="268" textAnchor={i === 0 ? 'start' : i === days.length - 1 ? 'end' : 'middle'} fontSize="11" fill="#78716c">{labelDate(days[i].date)}</text>)}
      <polyline points={days.map((d, i) => `${x(i)},${y(d.completed)}`).join(' ')} fill="none" stroke="#059669" strokeWidth="3" strokeLinejoin="round" />
      <polyline points={days.map((d, i) => `${x(i)},${y(d.failed)}`).join(' ')} fill="none" stroke="#f43f5e" strokeWidth="2.5" strokeDasharray="7 5" strokeLinejoin="round" />
      {days.map((d, i) => <g key={d.date} tabIndex={0} role="button" aria-label={`${labelDate(d.date)}: ${d.completed} completed, ${d.failed} failed or cancelled, ${d.total} total`} onFocus={() => setSelected(i)} onMouseEnter={() => setSelected(i)} onClick={() => setSelected(i)} onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setSelected(i); } }} className="cursor-crosshair outline-none">
        <rect x={x(i) - 610 / days.length / 2} y="32" width={610 / days.length} height="216" fill="transparent" />
        <circle cx={x(i)} cy={y(d.completed)} r={d.completed || selected === i ? 4 : 2} fill="#059669" />
        <circle cx={x(i)} cy={y(d.failed)} r={d.failed || selected === i ? 4 : 2} fill="white" stroke="#f43f5e" strokeWidth="1.5" />
      </g>)}
    </svg>
    <div className="flex flex-wrap gap-x-5 gap-y-1 rounded-lg border border-stone-200 bg-white px-3 py-2 text-xs tabular-nums" aria-live="polite"><span className="font-medium text-stone-700">{labelDate(active.date)}</span><span className="text-emerald-700">Completed: {active.completed}</span><span className="text-rose-600">Failed / cancelled: {active.failed}</span><span className="text-stone-500">Total: {active.total}</span></div>
    <p className="mt-4 text-xs leading-5 text-stone-500">Current outcomes of {total} threads, grouped by the day they were created. Today is partial; active threads are included in totals but not in either outcome line.{failed === 0 && ' No failed or cancelled threads in this period; the dashed red line remains at zero.'}</p>
  </figure>;
}
