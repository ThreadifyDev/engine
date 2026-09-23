const rates = [
  { key: 'success_rate', label: 'Completion', color: 'bg-emerald-500', explanation: 'Threads that eventually completed, including recovered runs.' },
  { key: 'overall_failure_rate', label: 'Failure', color: 'bg-rose-400', explanation: 'Threads that failed or were cancelled.' },
  { key: 'error_rate', label: 'Error', color: 'bg-amber-400', explanation: 'System errors, separate from business failures.' },
  { key: 'recovery_rate', label: 'Recovery', color: 'bg-sky-500', explanation: 'Failed deliveries that subsequently succeeded on retry.' },
] as const;

export default function DeliveryRatesChart({ data, range }: { data: Record<string, { value?: unknown } | undefined>; range: '7d' | '30d' | '90d' }) {
  return <figure aria-label={`Delivery rates bar chart for the last ${parseInt(range)} days`} className="rounded-xl bg-stone-50/80 p-4 sm:p-6">
    <figcaption className="mb-6 flex flex-wrap items-center justify-between gap-2">
      <span className="text-sm font-semibold text-stone-800">Delivery rates</span>
      <span className="rounded-full border border-stone-200 bg-white px-3 py-1 text-xs text-stone-500">Last {parseInt(range)} days</span>
    </figcaption>
    <dl className="space-y-5">
      {rates.map(rate => {
        const raw = data[rate.key]?.value;
        const value = typeof raw === 'number' && Number.isFinite(raw) && raw >= 0 && raw <= 100 ? raw : null;
        return <div key={rate.key} title={rate.explanation}>
          <div className="mb-2 flex items-center justify-between gap-3 text-sm">
            <dt className="flex items-center gap-2 text-stone-600"><span aria-hidden="true" className={`h-2 w-2 rounded-full ${rate.color}`} />{rate.label}</dt>
            <dd className={`font-semibold tabular-nums ${value === null ? 'text-stone-400' : 'text-stone-900'}`}>{value === null ? 'Unavailable' : `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })}%`}</dd>
          </div>
          <div aria-hidden="true" className="relative h-7 overflow-hidden rounded-md border border-stone-200/80 bg-white">
            <div className="absolute inset-0 grid grid-cols-4 divide-x divide-stone-100"><span /><span /><span /><span /></div>
            {value !== null && <div className={`relative h-full rounded-r-md ${rate.color}`} style={{ width: `${value}%` }} />}
            {value === null && <div className="absolute inset-x-0 top-1/2 border-t border-dashed border-stone-300" />}
          </div>
        </div>;
      })}
    </dl>
    <div aria-hidden="true" className="mt-3 flex justify-between text-[10px] tabular-nums text-stone-400"><span>0%</span><span>25%</span><span>50%</span><span>75%</span><span>100%</span></div>
    <p className="mt-6 text-xs leading-5 text-stone-500">Rates summarize the selected period. They can overlap and do not add up to 100%. Unavailable values are not counted as zero.</p>
  </figure>;
}
