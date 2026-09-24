import { useState } from 'react';
import { Check, Copy } from 'lucide-react';

type LineKind = 'feature' | 'include' | 'rule' | 'step' | 'context' | 'owner' | 'dependency' | 'flow' | 'timing' | 'lifecycle' | 'background' | 'plain';

const lineStyles: Record<LineKind, { label: string; className: string }> = {
  feature: { label: 'Feature', className: 'bg-violet-50 text-violet-700 ring-violet-200' },
  include: { label: 'Module', className: 'bg-emerald-50 text-emerald-700 ring-emerald-200' },
  rule: { label: 'Rule', className: 'bg-slate-100 text-slate-700 ring-slate-200' },
  step: { label: 'Step name', className: 'bg-sky-50 text-sky-700 ring-sky-200' },
  context: { label: 'Context check', className: 'bg-emerald-50 text-emerald-700 ring-emerald-200' },
  owner: { label: 'Owner check', className: 'bg-amber-50 text-amber-800 ring-amber-200' },
  dependency: { label: 'Dependency', className: 'bg-indigo-50 text-indigo-700 ring-indigo-200' },
  flow: { label: 'Flow rule', className: 'bg-orange-50 text-orange-700 ring-orange-200' },
  timing: { label: 'Timing rule', className: 'bg-rose-50 text-rose-700 ring-rose-200' },
  lifecycle: { label: 'Lifecycle rule', className: 'bg-teal-50 text-teal-700 ring-teal-200' },
  background: { label: 'Thread rule', className: 'bg-cyan-50 text-cyan-700 ring-cyan-200' },
  plain: { label: '', className: '' },
};

function classifyLine(line: string): LineKind {
  const trimmed = line.trim();
  if (/^Feature:/.test(trimmed)) return 'feature';
  if (/^Include:/.test(trimmed)) return 'include';
  if (/^Rule:/.test(trimmed)) return 'rule';
  if (/^When\s+step\s+"/.test(trimmed)) return 'step';
  if (/^(?:Then|And)\s+content\s+"/.test(trimmed) || /^Given\s+the thread/.test(trimmed)) return 'context';
  if (/^(?:Then|And)\s+owner\s+must\s+be\s+"/.test(trimmed)) return 'owner';
  if (/^(?:Then|And)\s+step\s+".*"\s+must\s+(?:have succeeded|succeed before)/.test(trimmed)) return 'dependency';
  if (/^(?:Then|And)\s+(?:next step|the next step)/.test(trimmed)) return 'flow';
  if (/^(?:Then|And)\s+(?:this step must finish|the next step must start|this step may be retried)/.test(trimmed)) return 'timing';
  if (/^(?:Then|And)\s+this step is (?:an entry point|terminal)/.test(trimmed)) return 'lifecycle';
  if (/^Background:/.test(trimmed)) return 'background';
  return 'plain';
}

function renderLine(line: string, kind: LineKind) {
  if (kind === 'step') {
    const match = line.match(/^(\s*When\s+step\s+)("(?:[^"\\]|\\.)*")(\s+is submitted\s*)$/);
    if (match) return <>{match[1]}<span className="rounded bg-sky-100 px-1 py-0.5 font-semibold text-sky-900">{match[2]}</span>{match[3]}</>;
  }

  if (kind === 'context') {
    const match = line.match(/^(\s*(?:(?:Then|And)\s+content\s+))("(?:[^"\\]|\\.)*")(.*)$/);
    if (match) return <>{match[1]}<span className="rounded bg-emerald-100 px-1 py-0.5 font-semibold text-emerald-900">{match[2]}</span>{match[3]}</>;
  }

  if (kind === 'rule') {
    const match = line.match(/^(\s*Rule:\s*)(.*)$/);
    if (match) return <><span className="text-slate-400">{match[1]}</span><span className="font-semibold text-slate-900">{match[2]}</span></>;
  }

  if (kind === 'feature') {
    const match = line.match(/^(\s*Feature:\s*)(.*)$/);
    if (match) return <><span className="text-violet-500">{match[1]}</span><span className="font-semibold text-violet-900">{match[2]}</span></>;
  }

  if (kind === 'include') {
    const match = line.match(/^(\s*Include:\s*)([^\s]+)(.*)$/);
    if (match) return <><span className="text-emerald-600">{match[1]}</span><span className="font-semibold text-emerald-900">{match[2]}</span>{match[3]}</>;
  }

  return line;
}

export default function ContractSourceView({ source }: { source: string }) {
  const [copied, setCopied] = useState(false);
  const isGherkin = /^\s*Feature:/m.test(source);
  const copySource = async () => {
    await navigator.clipboard.writeText(source);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  if (!isGherkin) {
    let formatted = source;
    try { formatted = JSON.stringify(JSON.parse(source), null, 2); }
    catch { /* YAML and other stored source are shown unchanged. */ }
    return <section aria-label="Contract source" className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
      <div className="flex items-center justify-between gap-3 border-b border-stone-200 px-5 py-4">
        <div><h2 className="text-sm font-semibold text-stone-900">Contract source</h2><p className="mt-1 text-xs text-stone-500">Published source for this version</p></div>
        <button type="button" onClick={copySource} className="inline-flex items-center gap-1.5 rounded-lg border border-stone-200 px-3 py-2 text-xs font-medium text-stone-700 hover:bg-stone-50">
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}{copied ? 'Copied' : 'Copy source'}
        </button>
      </div>
      <pre className="max-h-[calc(100vh-350px)] min-h-72 overflow-auto bg-[#fbfbfa] p-5 text-xs leading-6 text-stone-800"><code>{formatted || 'No contract source available.'}</code></pre>
    </section>;
  }

  return (
    <section aria-label="Formatted Gherkin contract source" className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-200 bg-white px-5 py-4">
        <div>
          <h2 className="text-sm font-semibold text-stone-900">Contract source</h2>
          <p className="mt-0.5 text-xs text-stone-500">Threadify’s interpretation of each Gherkin rule</p>
        </div>
        <button type="button" onClick={copySource} className="inline-flex items-center gap-1.5 rounded-lg border border-stone-200 bg-white px-3 py-2 text-xs font-medium text-stone-700 transition-colors hover:bg-stone-50">
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}{copied ? 'Copied' : 'Copy source'}
        </button>
      </div>
      <div className="max-h-[calc(100vh-350px)] min-h-72 overflow-auto bg-[#fbfbfa] py-3">
        <ol className="min-w-[680px] font-mono text-[12px] leading-7">
          {source.split(/\r?\n/).map((line, index) => {
            const kind = classifyLine(line);
            const style = lineStyles[kind];
            return (
              <li key={index} className={`grid grid-cols-[2.5rem_6.25rem_minmax(0,1fr)] items-start gap-2 border-l-2 px-4 transition-colors hover:bg-[#edf1ec] ${kind === 'rule' ? 'border-l-slate-400 bg-[#eff1ed]' : kind === 'context' ? 'border-l-emerald-300' : kind === 'step' ? 'border-l-sky-300' : 'border-l-transparent'}`}>
                <span className="select-none pt-0.5 text-right text-[11px] tabular-nums text-stone-400">{index + 1}</span>
                <span className="pt-1">
                  {style.label && <span className={`inline-flex rounded-md px-1.5 py-0.5 font-sans text-[9px] font-semibold leading-4 ring-1 ring-inset ${style.className}`}>{style.label}</span>}
                </span>
                <code className={`${kind === 'plain' ? 'text-stone-600' : 'text-stone-800'} whitespace-pre-wrap break-words`}>{renderLine(line, kind)}</code>
              </li>
            );
          })}
        </ol>
      </div>
    </section>
  );
}
