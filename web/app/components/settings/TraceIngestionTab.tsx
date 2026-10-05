import { useEffect, useState } from 'react';
import { ArrowRight, Check, ChevronDown, FlaskConical, Info, RefreshCw } from 'lucide-react';
import { api, type IngestionRules, type IngestionPreview } from '~/lib/api';

import { parseTraceFilters, formatTraceFilters } from '~/lib/trace-ingestion';

const lines = (value: string) => value.split(/\r?\n/).map(v => v.trim()).filter(Boolean);
const secondaryButton = 'inline-flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 disabled:opacity-40';

export function TraceIngestionTab() {
  const [saved, setSaved] = useState<IngestionRules | null>(null);
  const [rulesText, setRulesText] = useState('');
  const [samples, setSamples] = useState('');
  const [preview, setPreview] = useState<IngestionPreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [previewOpen, setPreviewOpen] = useState(false);
  const dirty = saved !== null && (saved.mode === 'exclude_legacy' || rulesText !== formatTraceFilters(saved));
  const keptSpans = saved ? Math.max(0, saved.evaluated_spans - saved.dropped_spans) : 0;

  function show(result: IngestionRules) { setSaved(result); setRulesText(formatTraceFilters(result)); setPreview(null); }
  useEffect(() => {
    let mounted = true;
    api.getIngestionRules().then(result => { if (mounted) show(result); })
      .catch(e => { if (mounted) setError(e.message); });
    return () => { mounted = false; };
  }, []);

  async function reload() {
    setBusy(true); setError(''); setMessage('');
    try { show(await api.getIngestionRules()); } catch (e: any) { setError(e.message); }
    finally { setBusy(false); }
  }
  async function save() {
    if (!saved) return;
    setBusy(true); setError(''); setMessage('');
    try { const rules = parseTraceFilters(rulesText); show(await api.saveIngestionRules(rules.filters, saved.revision, rules.exclude)); setMessage('Changes saved'); }
    catch (e: any) { setError(e.message); }
    finally { setBusy(false); }
  }
  async function test() {
    setBusy(true); setError(''); setMessage(''); setPreview(null);
    try { const rules = parseTraceFilters(rulesText); setPreview(await api.previewIngestionRules(rules.filters, lines(samples), rules.exclude)); }
    catch (e: any) { setError(e.message); }
    finally { setBusy(false); }
  }

  return <section className="w-full space-y-6 text-stone-900" aria-labelledby="trace-ingestion-title">
    <header className="flex items-start justify-between gap-4">
      <div>
        <h3 id="trace-ingestion-title" className="text-xl font-semibold tracking-tight">Trace ingestion</h3>
        <p className="mt-1.5 text-sm text-gray-500">OTel spans for general threads only.</p>
      </div>
      <button type="button" onClick={reload} disabled={busy} aria-label="Reload saved rules and counts" title="Reload saved rules and counts" className={`${secondaryButton} !p-2`}>
        <RefreshCw size={16} aria-hidden="true" className={busy ? 'animate-spin' : ''} />
      </button>
    </header>
    {error && <div role="alert" className="rounded-lg border border-red-100 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>}
    {!saved && !error && <p role="status" className="py-8 text-sm text-gray-500">Loading rules…</p>}
    {saved && <>
      {saved.mode === 'exclude_legacy' && <div role="status" className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">Your saved exclusion rules still apply. The editor carries them into [drop spans]; saving preserves their behavior.</div>}
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-gray-500">
        <span><strong className="mr-1 font-semibold tabular-nums text-gray-900">{saved.evaluated_spans.toLocaleString()}</strong> evaluated</span>
        <span><strong className="mr-1 font-semibold tabular-nums text-gray-900">{keptSpans.toLocaleString()}</strong> kept</span>
        <span className="inline-flex items-center gap-1.5 text-xs"><span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${saved.mode === 'exclude_legacy' || (saved.exclude?.length ?? 0) > 0 || saved.filters.join('\n') !== '*' ? 'bg-emerald-500' : 'bg-gray-300'}`} />{saved.mode === 'exclude_legacy' ? 'Legacy exclusions active' : (saved.exclude?.length ?? 0) > 0 ? 'Drop rules active' : saved.filters.join('\n') === '*' ? 'All spans allowed' : saved.filters.length ? 'Keep list active' : 'No spans allowed'}</span>
      </div>
      <div className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm">
        <div className="p-5 sm:p-6">
          <label htmlFor="trace-filters" className="mb-4 block text-sm font-semibold">Span filters</label>
          <textarea id="trace-filters" aria-describedby="trace-filter-hint" value={rulesText} disabled={busy || !saved.can_manage} onChange={e => { setRulesText(e.target.value); setPreview(null); setMessage(''); }} rows={8} spellCheck={false} autoCapitalize="none" className="block w-full resize-y rounded-lg border border-gray-200 bg-gray-50/70 p-4 font-mono text-sm leading-6 text-gray-800 placeholder:text-gray-400 focus:border-gray-400 focus:bg-white focus:outline-none focus:ring-1 focus:ring-gray-400 disabled:opacity-60" />
          <p id="trace-filter-hint" className="mt-3 text-xs leading-5 text-gray-500">One span pattern per line: exact, trailing <code>*</code>, or <code>regex:</code>. Use <code>(?i)</code> for case-insensitive regex. Drop rules win.</p>
        </div>
        <div className="border-t border-gray-100">
          <button type="button" onClick={() => setPreviewOpen(!previewOpen)} aria-expanded={previewOpen} aria-controls="trace-preview" className="flex w-full items-center gap-2.5 px-5 py-4 text-left text-sm text-gray-600 transition hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-gray-400 sm:px-6">
            <FlaskConical size={16} aria-hidden="true" />Test your rules<ChevronDown size={16} aria-hidden="true" className={`ml-auto transition-transform ${previewOpen ? 'rotate-180' : ''}`} />
          </button>
          <div id="trace-preview" hidden={!previewOpen} className="space-y-4 px-5 pb-5 sm:px-6 sm:pb-6">
            <label htmlFor="trace-samples" className="text-xs text-gray-500">Span names</label>
            <textarea id="trace-samples" value={samples} disabled={busy} onChange={e => { setSamples(e.target.value); setPreview(null); }} rows={4} placeholder={'POST /graphql\ncheckout.completed'} spellCheck={false} className="block w-full rounded-lg border border-gray-200 p-3 font-mono text-sm leading-6 placeholder:text-gray-400 focus:border-gray-400 focus:outline-none focus:ring-1 focus:ring-gray-400" />
            <button type="button" disabled={busy || !lines(samples).length} onClick={test} className={secondaryButton}>Run preview<ArrowRight size={14} aria-hidden="true" /></button>
            {preview && <div aria-live="polite" className="overflow-hidden rounded-lg border border-gray-200">
              <div className="border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs text-gray-500">{preview.kept} kept · {preview.dropped} excluded</div>
              <ul className="max-h-64 divide-y divide-gray-100 overflow-y-auto">
                {preview.spans.map((span, i) => <li key={i} className="flex items-start justify-between gap-4 px-3 py-2.5 text-sm">
                  <div className="min-w-0"><p className="break-all font-mono text-xs leading-5">{span.name || 'JSON sample'}</p>{span.drop_pattern ? <p className="mt-0.5 text-xs text-gray-500">[drop spans] <code>{span.drop_pattern}</code></p> : span.drop ? <p className="mt-0.5 text-xs text-gray-500">Outside keep list</p> : span.pattern && <p className="mt-0.5 break-all text-xs text-gray-400">Matches <code>{span.pattern}</code></p>}</div>
                  <span className={`shrink-0 rounded-md px-2 py-0.5 text-xs font-medium ${span.drop ? 'bg-gray-100 text-gray-600' : 'bg-emerald-50 text-emerald-700'}`}>{span.drop ? 'Excluded' : 'Kept'}</span>
                </li>)}
              </ul>
            </div>}
          </div>
        </div>
        <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 bg-gray-50/60 px-5 py-4 sm:px-6">
          <p role="status" className="flex items-center gap-1.5 text-xs text-gray-500">{message ? <><Check size={14} className="text-emerald-600" aria-hidden="true" />{message}</> : !saved.can_manage ? 'Only administrators can edit rules.' : saved.mode === 'exclude_legacy' ? 'Save sectioned rules' : dirty ? 'Unsaved changes' : 'Up to date'}</p>
          {saved.can_manage && <div className="flex items-center gap-3">
            {dirty && saved.mode !== 'exclude_legacy' && <button type="button" disabled={busy} onClick={() => { show(saved); setError(''); setMessage(''); }} className="rounded px-2 py-2 text-sm text-gray-500 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 disabled:opacity-40">Discard</button>}
            <button type="button" disabled={busy || !dirty} onClick={save} className="rounded-lg bg-stone-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-stone-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 focus-visible:ring-offset-2 disabled:opacity-40">Save changes</button>
          </div>}
        </footer>
      </div>
      <div className="space-y-3 text-xs text-gray-500">
        <p>Contract threads use their input config. Direct SDK events bypass these filters.</p>
        <details className="group">
          <summary className="flex w-fit cursor-pointer list-none items-center gap-1.5 rounded text-gray-500 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 [&::-webkit-details-marker]:hidden"><Info size={13} aria-hidden="true" />How filtering works<ChevronDown size={13} aria-hidden="true" className="transition-transform group-open:rotate-180" /></summary>
          <ul className="mt-3 list-disc space-y-2 pl-5 leading-5">
            <li>Patterns match original span names. Plain patterns are case-sensitive; a trailing <code>*</code> matches a prefix. Use <code>regex:(?i)^POST /graphql</code> for case-insensitive matching.</li>
            <li>Regex uses Go/RE2 syntax and can match anywhere in the name; use <code>^</code> and <code>$</code> for boundaries. Lookarounds and backreferences are unsupported.</li>
            <li>Empty [keep spans] keeps none. Empty [drop spans] adds no exclusions.</li>
            <li>A span must match [keep spans]. Any matching [drop spans] pattern then excludes it, regardless of section order.</li>
            <li>Changes apply to the next batches on every replica. Filtering does not redact fields or reduce received bandwidth usage.</li>
            <li>Counts include export retries. Previews do not change counts or save rules.</li>
          </ul>
        </details>
      </div>
    </>}
  </section>;
}
