import { useEffect, useState } from 'react';
import { Check, RefreshCw } from 'lucide-react';
import { api, type BrowserActionMappingRule, type BrowserActionMappings, type ObservedBrowserAction } from '~/lib/api';

type Props = { contractName: string; version: number; steps: string[] };
const scoped = (rule: BrowserActionMappingRule, name: string, version: number) => rule.contract === name && rule.version === version;
function format(rules: BrowserActionMappingRule[]) {
  const byStep = new Map<string, string[]>();
  for (const rule of rules) byStep.set(rule.step, [...(byStep.get(rule.step) ?? []), rule.action]);
  return [...byStep].map(([step, actions]) => `${actions.join(',')}=${step}`).join('\n');
}

function draftActions(value: string) {
  return value.split(/\r?\n/).flatMap(line => line.split('=')[0].split(',').map(action => action.trim()));
}

function parse(value: string, contract: string, version: number, steps: string[]): BrowserActionMappingRule[] {
  const seen = new Set<string>();
  return value.split(/\r?\n/).flatMap((line, index) => {
    const text = line.trim();
    if (!text) return [];
    const equals = text.indexOf('=');
    if (equals < 1 || equals !== text.lastIndexOf('=') || equals === text.length - 1)
      throw new Error(`Line ${index + 1} must use action=contract_step.`);
    const actions = text.slice(0, equals).split(',').map(action => action.trim());
    const step = text.slice(equals + 1).trim();
    if (actions.some(action => !action) || !step) throw new Error(`Line ${index + 1} needs action names and a step.`);
    if (steps.length && !steps.includes(step)) throw new Error(`Line ${index + 1}: "${step}" is not a step in version ${version}.`);
    return actions.map(action => {
      if (seen.has(action)) throw new Error(`Line ${index + 1} repeats action "${action}".`);
      seen.add(action);
      return { action, contract, version, step };
    });
  });
}

export function BrowserActionMappings({ contractName, version, steps }: Props) {
  const [saved, setSaved] = useState<BrowserActionMappings | null>(null);
  const [value, setValue] = useState('');
  const [observed, setObserved] = useState<ObservedBrowserAction[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const current = saved?.rules.filter(rule => scoped(rule, contractName, version)) ?? [];
  const dirty = saved !== null && value !== format(current);
  const canEdit = saved?.can_manage ?? false;
  const recent = observed.filter(item => item.contract === contractName && item.version === version);

  function show(result: BrowserActionMappings) {
    setSaved(result);
    setValue(format(result.rules.filter(rule => scoped(rule, contractName, version))));
  }
  useEffect(() => {
    let active = true;
    setSaved(null); setError('');
    api.getBrowserActionMappings().then(result => { if (active) show(result); })
      .catch(cause => { if (active) setError(cause.message); });
    api.getObservedBrowserActions().then(result => { if (active) setObserved(result.actions); })
      .catch(() => { /* Manual entry remains available. */ });
    return () => { active = false; };
  }, [contractName, version]);

  async function reload() {
    setBusy(true); setError(''); setMessage('');
    try {
      show(await api.getBrowserActionMappings());
      setObserved((await api.getObservedBrowserActions()).actions);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not reload mappings.'); }
    finally { setBusy(false); }
  }
  async function save() {
    if (!saved) return;
    let rules: BrowserActionMappingRule[];
    try { rules = parse(value, contractName, version, steps); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Invalid mapping.'); return; }
    setBusy(true); setError(''); setMessage('');
    try {
      const others = saved.rules.filter(rule => !scoped(rule, contractName, version));
      show(await api.saveBrowserActionMappings([...others, ...rules], saved.revision));
      setMessage(`Mappings saved for version ${version}.`);
    } catch (cause) {
      setError(`${cause instanceof Error ? cause.message : 'Could not save mappings.'} Reload if another administrator changed them.`);
    } finally { setBusy(false); }
  }
  function addObserved(action: string) {
    if (draftActions(value).includes(action)) return;
    setValue(previous => `${previous.trimEnd()}${previous.trim() ? '\n' : ''}${action}=`);
    setError(''); setMessage('');
  }

  return <section aria-labelledby="browser-mappings-title" className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-200 px-5 py-4 sm:px-6">
      <div><h3 id="browser-mappings-title" className="text-sm font-semibold text-stone-900">Input mappings</h3>
        <p className="mt-1 text-xs text-stone-500">OTel spans and auto-captured browser actions. Direct SDK events bypass these mappings.</p></div>
      <div className="flex items-center gap-2">
        <span className="rounded-md bg-stone-100 px-2 py-1 text-xs font-medium text-stone-600">{current.length} {current.length === 1 ? 'input' : 'inputs'} mapped</span>
        <button type="button" onClick={reload} disabled={busy} aria-label="Reload input mappings" className="rounded-lg border border-stone-200 bg-white p-2 text-stone-600 hover:bg-stone-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-emerald-700 disabled:opacity-40"><RefreshCw size={15} aria-hidden="true" /></button>
      </div>
    </div>
    {error && <div role="alert" className="mx-5 mt-5 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 sm:mx-6">{error}</div>}
    {!saved && !error && <p role="status" className="px-5 py-10 text-sm text-stone-500 sm:px-6">Loading input mappings…</p>}
    {saved && <>
      <div className="space-y-4 p-5 sm:p-6">
        <div><label htmlFor="browser-action-mappings" className="text-sm font-medium text-stone-800">Input name = contract step</label>
          <p id="browser-mappings-hint" className="mt-1 text-xs leading-5 text-stone-500">Exact names or a trailing *. Separate names with commas: <code className="font-mono text-stone-700">checkout_clicked,checkout_confirmed=browser_checkout</code>. Applies to version {version}.</p></div>
        <textarea id="browser-action-mappings" aria-describedby="browser-mappings-hint" value={value} onChange={event => { setValue(event.target.value); setError(''); setMessage(''); }} disabled={busy || !canEdit} rows={8} spellCheck={false} autoCapitalize="none" placeholder={'checkout_clicked,checkout_confirmed=browser_checkout\nreview_delivery_options=delivery_reviewed'} className="block min-h-52 w-full resize-y rounded-lg border border-stone-200 bg-[#fbfbfa] p-4 font-mono text-sm leading-7 text-stone-900 placeholder:text-stone-400 focus:border-emerald-500 focus:bg-white focus:outline-none focus:ring-1 focus:ring-emerald-500 disabled:opacity-60" />
        {steps.length > 0 && <p className="text-xs leading-5 text-stone-500">Steps in this version: <span className="font-mono text-stone-700">{steps.join(', ')}</span></p>}
        {canEdit && recent.length > 0 && <div className="flex flex-wrap items-center gap-2 border-t border-stone-100 pt-4">
          <span className="mr-1 text-xs font-medium text-stone-500">Recently captured</span>
          {recent.map(item => <button key={item.name} type="button" onClick={() => addObserved(item.name)} disabled={busy || draftActions(value).includes(item.name)} title={`Seen ${item.count} times`} className="rounded-md border border-stone-200 bg-white px-2.5 py-1 text-xs font-mono text-stone-700 hover:border-stone-400 focus-visible:outline focus-visible:outline-2 focus-visible:outline-emerald-700 disabled:opacity-40">{item.name}</button>)}
        </div>}
      </div>
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-stone-100 bg-stone-50/60 px-5 py-4 sm:px-6">
        <p role="status" className="flex items-center gap-1.5 text-xs text-stone-500">{message ? <><Check size={14} className="text-emerald-600" aria-hidden="true" />{message}</> : !canEdit ? 'Only administrators can edit input mappings.' : dirty ? 'Unsaved changes' : 'Up to date'}</p>
        {canEdit && <div className="flex items-center gap-3">
          {dirty && <button type="button" onClick={() => { setValue(format(current)); setError(''); setMessage(''); }} disabled={busy} className="rounded px-2 py-2 text-sm text-stone-500 hover:text-stone-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-emerald-700 disabled:opacity-40">Discard</button>}
          <button type="button" onClick={save} disabled={busy || !dirty} className="rounded-lg bg-stone-900 px-4 py-2 text-sm font-medium text-white hover:bg-stone-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700 disabled:opacity-40">Save changes</button>
        </div>}
      </footer>
    </>}
  </section>;
}
