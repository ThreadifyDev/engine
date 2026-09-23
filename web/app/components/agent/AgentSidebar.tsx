import { useEffect, useRef, useState, type FormEvent } from 'react';
import { ArrowUp, Check, ChevronRight, FileCode2, GitBranch, Loader2, MapPin, Plus, ScanLine, Sparkles, Square, Wrench, X } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import { useAgent } from './agent-context';
import { previewTasks, type PreviewTask } from './agent-preview';

const taskIcons = { contract: FileCode2, extraction: ScanLine, investigation: GitBranch };
const focusStyle = 'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700';

export default function AgentSidebar({ isCompact }: { isCompact: boolean }) {
  const { agentStatus, checkingAgentStatus, refreshAgentStatus, applyProfileProposal, closeAgent, context, includeContext, setIncludeContext, messages, composer, setComposer, sendMessage, newConversation, isSending, error, stop } = useAgent();
  const isReady = agentStatus.status === 'ready';
  const [proposalStatus, setProposalStatus] = useState<Record<string, { error: boolean; text: string }>>({});
  const panel = useRef<HTMLElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const scrollArea = useRef<HTMLDivElement>(null);
  const followResponse = useRef(true);

  useEffect(() => { input.current?.focus(); }, []);
  useEffect(() => {
    if (followResponse.current) scrollArea.current?.scrollTo({ top: messages.length ? scrollArea.current.scrollHeight : 0, behavior: 'instant' });
  }, [messages]);
  useEffect(() => {
    if (isCompact && !panel.current?.contains(document.activeElement)) input.current?.focus();
  }, [isCompact]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !event.defaultPrevented) { event.preventDefault(); closeAgent(); }

    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [closeAgent, isCompact]);

  const send = (event?: FormEvent) => {
    event?.preventDefault();
    if (isReady && !isSending) sendMessage(composer);
    input.current?.focus();
  };
  const startExample = (task: PreviewTask) => {
    const example = previewTasks.find(item => item.id === task)!;
    if (isReady && !isSending) sendMessage(example.prompt);
    input.current?.focus();
  };

  return (
    <>
      {isCompact && <div className="fixed inset-x-0 bottom-0 top-14 z-[60] bg-stone-950/25 backdrop-blur-[2px]" onClick={closeAgent} aria-hidden="true" />}
      <aside ref={panel} id="threadify-agent" role="complementary" aria-labelledby="agent-title" aria-describedby="agent-preview-note"
        className="fixed bottom-0 top-14 right-0 z-[70] flex w-full sm:max-w-[420px] flex-col border-l border-stone-200 bg-[#fafaf8] text-stone-900 shadow-[-12px_0_40px_-24px_rgba(0,0,0,0.2)] lg:w-[360px] xl:w-[420px]">
        <header className="flex shrink-0 items-center gap-3 border-b border-stone-200/80 bg-white px-5 py-4">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-[#172e28] text-emerald-100"><Sparkles className="h-[18px] w-[18px]" /></div>
          <div className="min-w-0 flex-1"><h2 id="agent-title" className="text-sm font-semibold tracking-tight">Threadify agent</h2><p className="mt-0.5 text-[11px] text-stone-500">A hand with your workflows</p></div>
          <button onClick={() => { newConversation(); input.current?.focus(); }} aria-label="New conversation" title="New conversation" className={`rounded-lg p-2 text-stone-500 hover:bg-stone-100 hover:text-stone-900 ${focusStyle}`}><Plus className="h-4 w-4" /></button>
          <button onClick={closeAgent} aria-label="Close agent" title="Close (Esc)" className={`rounded-lg p-2 text-stone-500 hover:bg-stone-100 hover:text-stone-900 ${focusStyle}`}><X className="h-4 w-4" /></button>
        </header>

        <div className="shrink-0 border-b border-stone-200/80 bg-white/60 px-5 py-3">
          <div className="flex items-center gap-2 text-xs text-stone-500"><MapPin className="h-3.5 w-3.5 shrink-0" /><span>Viewing</span><span className="truncate font-medium text-stone-800">{context.title}</span></div>
          {context.detail && <p title={context.detail} className="mt-1 truncate pl-[22px] font-mono text-[10px] text-stone-400">{context.detail}</p>}
        </div>

        {!isReady && <div role="status" aria-live="polite" className="shrink-0 border-b border-stone-200 bg-stone-100/70 px-5 py-4">
          <p className="text-sm font-medium">{agentStatus.status === 'starting' ? 'Agent is starting' : 'Agent is temporarily unavailable'}</p>
          <p className="mt-1 text-xs leading-relaxed text-stone-500">{agentStatus.status === 'starting' ? 'Getting the agent ready. You can keep working while it starts.' : 'Your conversation and draft are still here. Check again after the agent connection is restored.'}</p>
          <button type="button" onClick={refreshAgentStatus} disabled={checkingAgentStatus} className={`mt-3 inline-flex items-center gap-2 rounded-lg border border-stone-300 bg-white px-3 py-1.5 text-xs font-medium disabled:opacity-50 ${focusStyle}`}>
            {checkingAgentStatus && <Loader2 className="h-3 w-3 animate-spin" />}{checkingAgentStatus ? 'Checking…' : 'Check connection'}
          </button>
        </div>}

        <div ref={scrollArea} onScroll={event => { const element = event.currentTarget; followResponse.current = element.scrollHeight - element.scrollTop - element.clientHeight < 80; }} className="min-h-0 flex-1 overflow-y-auto overscroll-contain scroll-smooth motion-reduce:scroll-auto">
          {messages.length === 0 ? (
            <div className="px-6 pb-7 pt-9 sm:pt-12">
              <div className="mb-6 flex h-12 w-12 items-center justify-center rounded-2xl border border-emerald-900/10 bg-[#eef3ed] text-[#365b49]"><Sparkles className="h-6 w-6" strokeWidth={1.5} /></div>
              <p className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-stone-400">Your workspace companion</p>
              <h3 className="text-[26px] font-semibold leading-tight tracking-[-0.035em]">What are we<br />working on?</h3>
              <p className="mt-3 max-w-[300px] text-[13px] leading-[1.7] text-stone-500">From the first rule to the next step.<br />Ask, inspect, and draft right where you work.</p>
              <div className="mt-7 space-y-2.5">
                {previewTasks.map(task => {
                  const Icon = taskIcons[task.id];
                  return <button key={task.id} disabled={!isReady} onClick={() => startExample(task.id)} className={`group disabled:cursor-not-allowed disabled:opacity-50 flex w-full items-center gap-3 rounded-xl border border-stone-200 bg-white p-3.5 text-left transition hover:border-emerald-800/30 hover:bg-emerald-50/30 ${focusStyle}`}>
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-stone-50 text-stone-600 group-hover:bg-emerald-50 group-hover:text-emerald-800"><Icon className="h-4 w-4" strokeWidth={1.6} /></div>
                    <div className="min-w-0 flex-1"><p className="text-xs font-semibold">{task.title}</p><p className="mt-1 text-[11px] text-stone-500">{task.description}</p></div><ChevronRight className="h-3.5 w-3.5 shrink-0 text-stone-400" />
                  </button>;
                })}
              </div>
              <p className="mt-5 text-[11px] leading-relaxed text-stone-400">Choose a task or describe what you need.</p>
            </div>
          ) : (
            <div role="log" aria-label="Agent conversation" aria-live="polite" aria-relevant="additions" className="space-y-7 px-5 py-6">
              {messages.map(message => message.role === 'user' ? (
                <div key={message.id} className="ml-8 rounded-2xl rounded-br-sm border border-stone-200/70 bg-[#efefeb] px-4 py-3">
                  {message.context && <div className="mb-2 flex min-w-0 items-center gap-1 text-[10px] text-stone-500" title={message.context.path}><MapPin className="h-3 w-3 shrink-0" /><span className="truncate">{message.context.title}{message.context.detail ? ` · ${message.context.detail}` : ''}</span></div>}
                  <p className="whitespace-pre-wrap break-words text-[13px] leading-relaxed">{message.text}</p>
                </div>
              ) : (
                <div key={message.id}>
                  <div className="mb-2.5 flex items-center gap-2 text-[11px] font-medium text-stone-500"><Sparkles className="h-3.5 w-3.5 text-emerald-800" />Threadify<span className="text-stone-300">/</span><span className="font-normal text-stone-400">Agent</span></div>
                  {message.tools?.length ? <div className="mb-3 space-y-1.5">{message.tools.map(tool => <div key={tool.id} className="flex items-center gap-2 rounded-lg border border-stone-200 bg-white px-2.5 py-2 text-[11px] text-stone-500">
                    {tool.status === 'running' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : tool.status === 'completed' ? <Check className="h-3.5 w-3.5 text-emerald-700" /> : <Wrench className="h-3.5 w-3.5" />}
                    <span className="min-w-0 flex-1 truncate">{tool.name.replaceAll('_', ' ')}</span><span>{tool.status}</span>
                  </div>)}</div> : null}
                  {message.profileViewTarget && (message.text.includes('```profile-view') || message.text.includes('```profile-metrics')) && <div className="mb-3 rounded-xl border border-emerald-200 bg-emerald-50 p-3">
                    <p className="mb-2 text-xs text-emerald-900">Review these changes in the profile configuration before saving.</p>
                    <button disabled={!isReady || isSending || proposalStatus[message.id]?.error === false} onClick={() => {
                      try { applyProfileProposal(message); setProposalStatus(previous => ({ ...previous, [message.id]: { error: false, text: message.text.includes('```profile-metrics') ? 'Applied to data draft. Review and save Data & metrics when ready.' : 'Applied to preview. Save the presentation when ready.' } })); if (isCompact) closeAgent(); }
                      catch (cause) { setProposalStatus(previous => ({ ...previous, [message.id]: { error: true, text: cause instanceof Error ? cause.message : 'Could not apply this view.' } })); }
                    }} className="rounded-lg bg-emerald-900 px-3 py-2 text-xs font-medium text-white disabled:opacity-40">{message.text.includes('```profile-metrics') ? 'Apply to data draft' : 'Apply to preview'}</button>
                    {proposalStatus[message.id] && <p role={proposalStatus[message.id].error ? 'alert' : 'status'} className="mt-2 text-xs">{proposalStatus[message.id].text}</p>}
                  </div>}
                  {message.text ? <div className="break-words text-[13px] leading-[1.75] text-stone-600 [&_p]:mb-3 [&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:bg-stone-100 [&_pre]:p-3 [&_pre]:text-[11px] [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_a]:underline"><ReactMarkdown>{message.text}</ReactMarkdown></div>
                    : <p role="status" className="flex items-center gap-2 text-xs text-stone-400"><Loader2 className="h-3.5 w-3.5 animate-spin" />Working…</p>}
                </div>
              ))}
            </div>
          )}
        </div>

        <footer className="shrink-0 border-t border-stone-200/80 bg-[#fafaf8] px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-3">
          {error && <p role="alert" className="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs leading-relaxed text-red-700">{error}</p>}
          <form onSubmit={send} className="rounded-2xl border border-stone-300/80 bg-white p-3 shadow-sm focus-within:border-emerald-800/50 focus-within:ring-2 focus-within:ring-emerald-800/5">
            <button type="button" aria-pressed={includeContext} onClick={() => setIncludeContext(!includeContext)} title="Allow the agent to read this page’s route and current editor draft" className={`mb-2 flex max-w-full items-center gap-1.5 rounded-md px-2 py-1 text-[10px] ${includeContext ? 'bg-[#eef3ed] text-emerald-900' : 'bg-stone-100 text-stone-500'} ${focusStyle}`}>
              <MapPin className="h-3 w-3 shrink-0" /><span className="truncate">{includeContext ? context.title : 'Page context off'}</span>{includeContext && <Check className="h-3 w-3 shrink-0" />}
            </button>
            <label htmlFor="agent-message" className="sr-only">Message Threadify agent</label>
            <textarea ref={input} id="agent-message" value={composer} onChange={event => setComposer(event.target.value)} maxLength={4000} rows={2}
              onKeyDown={event => { if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); send(); } }}
              placeholder="Describe what you’d like to do…"
              className="block max-h-36 min-h-12 w-full resize-none bg-transparent text-[13px] leading-relaxed text-stone-800 outline-none placeholder:text-stone-400" />
            <div className="mt-2 flex items-center justify-between gap-2"><span className="text-[10px] text-stone-400">Enter to send · Shift + Enter for a new line</span>{isSending ? <button type="button" onClick={stop} aria-label="Stop response" className={`flex h-8 w-8 items-center justify-center rounded-lg bg-stone-800 text-white ${focusStyle}`}><Square className="h-3.5 w-3.5" /></button> : <button type="submit" disabled={!isReady || !composer.trim()} aria-label="Send message" className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-[#172e28] text-white transition hover:bg-[#25463d] disabled:cursor-not-allowed disabled:bg-stone-100 disabled:text-stone-300 ${focusStyle}`}><ArrowUp className="h-4 w-4" /></button>}</div>
          </form>
          <p id="agent-preview-note" className="mt-2.5 text-center text-[10px] leading-relaxed text-stone-400">Draft changes stay local until you save them.</p>
        </footer>
      </aside>
    </>
  );
}
