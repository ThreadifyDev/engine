import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { ArrowLeft, ArrowUpFromLine, Check, CheckCircle2, FileCode2, Loader2, Play, Sparkles, X, AlertCircle } from 'lucide-react';
import { api, ValidationError } from '~/lib/api';
import { useAgent } from '~/components/agent/agent-context';
import YamlEditor from '~/components/YamlEditor';

const example = `Feature: payment_processing
Version: 1
Description: Record valid payments.

Rule: Validate a payment
  When step "charge" is submitted
  Then owner must be "payment_processor"
  And content "amount" must be a number greater than 0
  And content "currency" must be one of "GBP", "USD", "EUR"
  And this step is an entry point
  And this step is terminal
`;
const focus = 'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700';

export default function ContractDraftEditor({ onSave }: { onSave: () => Promise<void> }) {
  const { isEnabled: agentEnabled, contractDraft: draft, editContractDraft, setContractEditorOpen, isOpen: agentOpen, openAgent } = useAgent();
  const [saving, setSaving] = useState(false);
  const [validating, setValidating] = useState(false);
  const [error, setError] = useState('');
  const [checked, setChecked] = useState<{ revision: number; valid: boolean; errors?: string[] }>();
  const fileInput = useRef<HTMLInputElement>(null);
  const revision = useRef(draft.revision);
  revision.current = draft.revision;
  useEffect(() => { setError(''); }, [draft.revision]);
  const preview = checked?.revision === draft.revision ? checked : draft.preview;
  const name = draft.source.match(/^\s*Feature:\s*(.+)$/m)?.[1]?.trim();
  const hasSource = Boolean(draft.source.trim());
  const busy = saving || validating;

  const validate = async () => {
    const currentRevision = draft.revision;
    setValidating(true);
    setError('');
    try {
      const result = await api.previewContract({ yaml: draft.source });
      if (revision.current === currentRevision) setChecked({ ...result, revision: currentRevision });
    } catch (cause) {
      if (revision.current === currentRevision) setError(cause instanceof Error ? cause.message : 'Could not validate this contract.');
    } finally { setValidating(false); }
  };

  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!hasSource || busy) return;
    setSaving(true);
    setError('');
    try { await onSave(); }
    catch (cause) {
      setError(cause instanceof ValidationError && cause.details?.length
        ? cause.details.map(detail => `${detail.field}: ${detail.message}`).join('\n')
        : cause instanceof Error ? cause.message : 'Could not create this contract.');
    } finally { setSaving(false); }
  };

  const importFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    setError('');
    if (file.size > 100_000) { setError('Choose a contract file smaller than 100 KB.'); return; }
    const currentRevision = revision.current;
    try {
      const source = await file.text();
      if (revision.current !== currentRevision) { setError('Your draft changed while the file was opening. Import it again to replace the current draft.'); return; }
      editContractDraft(source);
    } catch { setError('Could not read that file. Try again or paste its contents below.'); }
  };

  return (
    <section aria-labelledby="contract-editor-title" className="mx-auto max-w-5xl">
      <button type="button" onClick={() => setContractEditorOpen(false)} disabled={busy}
        className={`mb-6 inline-flex items-center gap-2 rounded-md text-xs font-medium text-stone-500 hover:text-stone-900 disabled:opacity-50 ${focus}`}>
        <ArrowLeft className="h-3.5 w-3.5" /> Contracts
      </button>
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <div className="mb-2 flex items-center gap-2"><h1 id="contract-editor-title" className="text-2xl font-semibold tracking-tight text-stone-900">New contract</h1><span className="rounded-md border border-stone-200 px-2 py-0.5 text-[10px] font-medium text-stone-500">Draft</span></div>
          <p className="max-w-lg text-sm leading-relaxed text-stone-500">Define the steps and rules for your workflow. Review them before creating your contract.</p>
        </div>
        <button type="button" aria-label="Close contract editor" disabled={busy} onClick={() => setContractEditorOpen(false)} className={`shrink-0 rounded-lg p-2 text-stone-400 hover:bg-stone-50 hover:text-stone-700 disabled:opacity-50 ${focus}`}><X className="h-4 w-4" /></button>
      </div>
      <form onSubmit={save}>
        <div className="overflow-hidden rounded-xl border border-stone-200 bg-white shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-200 bg-stone-50/70 px-4 py-3">
            <div className="flex min-w-0 items-center gap-2.5"><FileCode2 className="h-4 w-4 shrink-0 text-stone-400" /><span className="text-xs font-medium text-stone-700">Contract source</span><span className="text-[10px] text-stone-400">{name || !hasSource ? 'Gherkin' : 'YAML'}</span></div>
            <div className="flex items-center gap-3">
              {!hasSource && <button type="button" disabled={busy} onClick={() => editContractDraft(example)} className={`rounded text-xs text-stone-500 hover:text-stone-900 ${focus}`}>Use an example</button>}
              <button type="button" disabled={busy} onClick={() => fileInput.current?.click()} className={`inline-flex items-center gap-1.5 rounded-md text-xs font-medium text-stone-600 hover:text-stone-900 disabled:opacity-50 ${focus}`}><ArrowUpFromLine className="h-3.5 w-3.5" />Import file</button>
              <input ref={fileInput} type="file" accept=".feature,.gherkin,.yaml,.yml,.txt" onChange={importFile} aria-label="Import contract file" className="hidden" />
            </div>
          </div>
          <YamlEditor contractSource appearance="soft" value={draft.source} onChange={editContractDraft} readOnly={saving}
            placeholder={'Feature: your_workflow\n\nDescribe your steps and rules here…'} height="clamp(280px, 48dvh, 540px)" />
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-stone-100 bg-stone-50/50 px-4 py-2.5 text-[11px] text-stone-400">
            <span>Gherkin or YAML · .feature, .yaml, .yml</span><span>{hasSource ? `${draft.source.split('\n').length} lines` : 'Start with a file, an example, or your own rules'}</span>
          </div>
        </div>
        {(error || preview) && <div role={error || !preview?.valid ? 'alert' : 'status'} className={`mt-4 flex items-start gap-2.5 rounded-lg border px-4 py-3 text-xs leading-relaxed ${error || !preview?.valid ? 'border-red-100 bg-red-50 text-red-700' : 'border-emerald-100 bg-emerald-50/60 text-emerald-800'}`}>
          {error || !preview?.valid ? <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" /> : <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />}
          <div className="min-w-0 whitespace-pre-wrap">{error || (preview?.valid ? 'Validation passed. Your contract is ready to create.' : 'A few rules need attention.')}{!error && preview?.errors?.map((message, index) => <p key={index} className="mt-1">{message}</p>)}</div>
        </div>}
        <div className="mt-5 flex flex-wrap items-center justify-between gap-4 pb-4">
          <div className="text-xs text-stone-400">{agentEnabled && !agentOpen ? <button type="button" onClick={openAgent} className={`inline-flex items-center gap-1.5 rounded text-stone-500 hover:text-stone-800 ${focus}`}><Sparkles className="h-3.5 w-3.5" strokeWidth={1.5} />Get help with your draft</button> : 'Changes stay in your draft until you create it.'}</div>
          <div className="flex w-full items-center gap-2 sm:w-auto">
            <button type="button" onClick={validate} disabled={!hasSource || busy} className={`inline-flex flex-1 items-center justify-center gap-2 rounded-lg border border-stone-200 bg-white px-4 py-2.5 text-xs font-medium text-stone-600 transition hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-40 sm:flex-none ${focus}`}>
              {validating ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}{validating ? 'Validating…' : 'Validate'}
            </button>
            <button type="submit" disabled={!hasSource || busy} className={`inline-flex flex-1 items-center justify-center gap-2 rounded-lg bg-stone-900 px-4 py-2.5 text-xs font-medium text-white transition hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-40 sm:flex-none ${focus}`}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}{saving ? 'Creating…' : 'Create contract'}
            </button>
          </div>
        </div>
      </form>
    </section>
  );
}
