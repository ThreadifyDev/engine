import { FormEvent, KeyboardEvent, useEffect, useMemo, useRef, useState } from 'react';
import {
  Bot,
  CheckCircle2,
  ChevronDown,
  Loader2,
  Menu,
  Plus,
  Search,
  Send,
  Trash2,
  User,
  Wrench,
  X,
} from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import {
  harnest,
  messageText,
  sessionTitle,
  type HarnestSession,
  type HarnestStreamEvent,
} from '~/lib/harnest';

interface ToolActivity {
  id: string;
  name: string;
  status: 'running' | 'completed';
}

interface ChatMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: Date;
  tools?: ToolActivity[];
}

const LAST_SESSION_KEY = 'threadify.harnest.lastSessionId';

function titleFromInput(input: string) {
  const compact = input.replace(/\s+/g, ' ').trim();
  return compact.length > 54 ? `${compact.slice(0, 51)}…` : compact;
}

function displayDate(value: string | null) {
  if (!value) return 'Recent';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? 'Recent' : date.toLocaleDateString();
}

export default function HarnestThreadChat() {
  const [sessions, setSessions] = useState<HarnestSession[]>([]);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [search, setSearch] = useState('');
  const [isLoadingSessions, setIsLoadingSessions] = useState(true);
  const [isSending, setIsSending] = useState(false);
  const [isSessionMenuOpen, setIsSessionMenuOpen] = useState(false);
  const [isMobileSessionsOpen, setIsMobileSessionsOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  const filteredSessions = useMemo(
    () =>
      sessions.filter((session) =>
        sessionTitle(session).toLowerCase().includes(search.toLowerCase())
      ),
    [search, sessions]
  );

  const currentTitle = sessionId
    ? sessionTitle(sessions.find((session) => session.id === sessionId) || {
        id: sessionId,
        userId: '',
        state: {},
        applicationData: {},
        createdAt: null,
        updatedAt: null,
        metadata: {},
      })
    : 'New conversation';

  const refreshSessions = async () => {
    try {
      const next = await harnest.listSessions();
      setSessions(next);
      return next;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load agent sessions.');
      return [];
    } finally {
      setIsLoadingSessions(false);
    }
  };

  const openSession = async (id: string) => {
    setError(null);
    setIsSessionMenuOpen(false);
    setIsMobileSessionsOpen(false);
    try {
      const history = await harnest.getMessages(id);
      setMessages(
        history
          .filter((message) => message.role === 'user' || message.role === 'assistant')
          .flatMap((message) => {
            const content = messageText(message.content);
            // Framework transcripts may retain an empty assistant envelope for
            // a tool call. The live UI renders the tool on the final response,
            // so an empty historical bubble would look permanently unfinished.
            if (message.role === 'assistant' && !content.trim()) return [];
            return [{
              id: message.id,
              role: message.role as 'user' | 'assistant',
              content,
              createdAt: message.createdAt ? new Date(message.createdAt) : new Date(),
            }];
          })
      );
      setSessionId(id);
      localStorage.setItem(LAST_SESSION_KEY, id);
    } catch (cause) {
      localStorage.removeItem(LAST_SESSION_KEY);
      setError(cause instanceof Error ? cause.message : 'Failed to load this conversation.');
    }
  };

  useEffect(() => {
    let active = true;
    void (async () => {
      const next = await refreshSessions();
      if (!active) return;
      const remembered = localStorage.getItem(LAST_SESSION_KEY);
      if (remembered && next.some((session) => session.id === remembered)) {
        await openSession(remembered);
      }
    })();
    return () => {
      active = false;
      abortRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const startNewSession = () => {
    abortRef.current?.abort();
    setSessionId(null);
    setMessages([]);
    setError(null);
    setInput('');
    setIsSessionMenuOpen(false);
    setIsMobileSessionsOpen(false);
    localStorage.removeItem(LAST_SESSION_KEY);
  };

  const deleteSession = async (id: string) => {
    if (!window.confirm('Delete this agent conversation?')) return;
    try {
      await harnest.deleteSession(id);
      setSessions((current) => current.filter((session) => session.id !== id));
      if (sessionId === id) startNewSession();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete conversation.');
    }
  };

  const updateAssistant = (id: string, change: (message: ChatMessage) => ChatMessage) => {
    setMessages((current) =>
      current.map((message) => (message.id === id ? change(message) : message))
    );
  };

  const handleStreamEvent = (assistantId: string, event: HarnestStreamEvent) => {
    if (event.type === 'response.text.delta' && event.delta) {
      updateAssistant(assistantId, (message) => ({
        ...message,
        content: message.content + event.delta,
      }));
      return;
    }

    if (event.type === 'response.tool_call' && event.name) {
      updateAssistant(assistantId, (message) => ({
        ...message,
        tools: [
          ...(message.tools || []),
          { id: event.id || `${event.name}-${event.sequence}`, name: event.name!, status: 'running' },
        ],
      }));
      return;
    }

    if (event.type === 'response.tool_result' && event.name) {
      updateAssistant(assistantId, (message) => ({
        ...message,
        tools: (message.tools || []).map((tool) =>
          tool.id === event.callId || tool.name === event.name
            ? { ...tool, status: 'completed' }
            : tool
        ),
      }));
      return;
    }

    if (event.type === 'response.completed') {
      updateAssistant(assistantId, (message) => ({
        ...message,
        content: event.outputText || message.content,
      }));
      return;
    }

    if (event.type === 'error') {
      throw new Error(event.error || 'The Threadify agent could not complete this response.');
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const prompt = input.trim();
    if (!prompt || isSending) return;

    setError(null);
    setInput('');
    setIsSending(true);
    const userId = `user-${crypto.randomUUID()}`;
    const assistantId = `assistant-${crypto.randomUUID()}`;
    setMessages((current) => [
      ...current,
      { id: userId, role: 'user', content: prompt, createdAt: new Date() },
      { id: assistantId, role: 'assistant', content: '', createdAt: new Date(), tools: [] },
    ]);

    try {
      let activeSessionId = sessionId;
      if (!activeSessionId) {
        const created = await harnest.createSession(titleFromInput(prompt));
        activeSessionId = created.id;
        setSessionId(created.id);
        setSessions((current) => [created, ...current]);
        localStorage.setItem(LAST_SESSION_KEY, created.id);
      }

      const controller = new AbortController();
      abortRef.current = controller;
      await harnest.streamResponse(
        prompt,
        activeSessionId,
        (streamEvent) => handleStreamEvent(assistantId, streamEvent),
        controller.signal
      );
      await refreshSessions();
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return;
      const message = cause instanceof Error ? cause.message : 'The Threadify agent failed.';
      setError(message);
      updateAssistant(assistantId, (current) => ({
        ...current,
        content: current.content || 'I could not complete that request. Please try again.',
      }));
    } finally {
      abortRef.current = null;
      setIsSending(false);
    }
  };

  const onInputKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  };

  const sessionList = (
    <>
      <div className="border-b border-gray-200 p-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search conversations"
            className="w-full rounded-lg border border-gray-200 bg-white py-2 pl-9 pr-3 text-sm outline-none focus:border-gray-400 focus:ring-2 focus:ring-gray-100"
          />
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {isLoadingSessions ? (
          <div className="flex items-center justify-center gap-2 py-8 text-sm text-gray-500">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading
          </div>
        ) : filteredSessions.length ? (
          filteredSessions.map((session) => (
            <div
              key={session.id}
              className={`group mb-1 flex items-center rounded-lg ${session.id === sessionId ? 'bg-gray-900 text-white' : 'text-gray-700 hover:bg-gray-100'}`}
            >
              <button
                onClick={() => void openSession(session.id)}
                className="min-w-0 flex-1 px-3 py-2.5 text-left"
              >
                <span className="block truncate text-sm font-medium">{sessionTitle(session)}</span>
                <span className={`block text-xs ${session.id === sessionId ? 'text-gray-300' : 'text-gray-400'}`}>
                  {displayDate(session.updatedAt || session.createdAt)}
                </span>
              </button>
              <button
                onClick={() => void deleteSession(session.id)}
                className={`mr-2 rounded-md p-1.5 opacity-100 transition hover:bg-red-50 hover:text-red-600 lg:opacity-0 lg:group-hover:opacity-100 ${session.id === sessionId ? 'text-gray-300' : 'text-gray-400'}`}
                aria-label={`Delete ${sessionTitle(session)}`}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))
        ) : (
          <p className="px-3 py-8 text-center text-sm text-gray-400">No conversations yet</p>
        )}
      </div>
    </>
  );

  return (
    <div className="relative flex h-full min-h-0 overflow-hidden bg-white">
      <aside className="hidden w-72 shrink-0 flex-col border-r border-gray-200 bg-gray-50/70 lg:flex">
        <div className="border-b border-gray-200 p-3">
          <button
            onClick={startNewSession}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-gray-900 px-3 py-2.5 text-sm font-medium text-white hover:bg-gray-800"
          >
            <Plus className="h-4 w-4" /> New conversation
          </button>
        </div>
        {sessionList}
      </aside>

      {isMobileSessionsOpen && (
        <div className="absolute inset-0 z-40 flex lg:hidden">
          <button
            className="absolute inset-0 bg-gray-950/40"
            onClick={() => setIsMobileSessionsOpen(false)}
            aria-label="Close conversations"
          />
          <aside className="relative z-10 flex h-full w-[min(20rem,88vw)] flex-col bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-gray-200 p-3">
              <span className="text-sm font-semibold text-gray-900">Conversations</span>
              <button
                onClick={() => setIsMobileSessionsOpen(false)}
                className="rounded-md p-2 text-gray-500 hover:bg-gray-100"
                aria-label="Close conversations"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="border-b border-gray-200 p-3">
              <button
                onClick={startNewSession}
                className="flex w-full items-center justify-center gap-2 rounded-lg bg-gray-900 px-3 py-2.5 text-sm font-medium text-white"
              >
                <Plus className="h-4 w-4" /> New conversation
              </button>
            </div>
            {sessionList}
          </aside>
        </div>
      )}

      <section className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center gap-3 border-b border-gray-200 bg-white px-3 py-3 sm:px-5">
          <button
            onClick={() => setIsMobileSessionsOpen(true)}
            className="rounded-lg border border-gray-200 p-2 text-gray-600 hover:bg-gray-50 lg:hidden"
            aria-label="Open conversations"
          >
            <Menu className="h-4 w-4" />
          </button>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold text-gray-900">{currentTitle}</p>
            <p className="flex items-center gap-1.5 text-xs text-gray-500">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> Harnest agent
            </p>
          </div>
          <div className="relative lg:hidden">
            <button
              onClick={() => setIsSessionMenuOpen((open) => !open)}
              className="rounded-lg p-2 text-gray-500 hover:bg-gray-50"
              aria-label="Conversation actions"
            >
              <ChevronDown className="h-4 w-4" />
            </button>
            {isSessionMenuOpen && (
              <div className="absolute right-0 top-full z-20 mt-1 w-44 rounded-lg border border-gray-200 bg-white p-1 shadow-lg">
                <button onClick={startNewSession} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-gray-50">
                  <Plus className="h-4 w-4" /> New conversation
                </button>
              </div>
            )}
          </div>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-3 py-5 sm:px-6">
          <div className="mx-auto flex max-w-3xl flex-col gap-5">
            {messages.length === 0 && (
              <div className="flex min-h-[22rem] flex-col items-center justify-center px-4 text-center">
                <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-violet-100 text-violet-700">
                  <Bot className="h-6 w-6" />
                </div>
                <h2 className="text-lg font-semibold text-gray-900">Ask your execution data</h2>
                <p className="mt-2 max-w-md text-sm leading-6 text-gray-500">
                  Analyze threads, investigate failures, inspect entity profiles, or design a contract from observed execution.
                </p>
              </div>
            )}

            {messages.map((message) => (
              <article
                key={message.id}
                className={`flex gap-3 ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                {message.role === 'assistant' && (
                  <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-violet-100 text-violet-700">
                    <Bot className="h-4 w-4" />
                  </div>
                )}
                <div className={`min-w-0 max-w-[88%] sm:max-w-[80%] ${message.role === 'user' ? 'rounded-2xl rounded-br-md bg-gray-900 px-4 py-2.5 text-white' : 'text-gray-800'}`}>
                  {message.tools && message.tools.length > 0 && (
                    <div className="mb-3 flex flex-wrap gap-2">
                      {message.tools.map((tool) => (
                        <span key={tool.id} className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-violet-200 bg-violet-50 px-2.5 py-1 text-xs font-medium text-violet-700">
                          {tool.status === 'completed' ? <CheckCircle2 className="h-3.5 w-3.5" /> : <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                          <Wrench className="h-3 w-3" />
                          <span className="truncate">{tool.name}</span>
                        </span>
                      ))}
                    </div>
                  )}
                  {message.role === 'assistant' ? (
                    message.content ? (
                      <div className="prose prose-sm max-w-none break-words prose-pre:max-w-full prose-pre:overflow-x-auto">
                        <ReactMarkdown>{message.content}</ReactMarkdown>
                      </div>
                    ) : (
                      <div className="flex items-center gap-2 py-2 text-sm text-gray-500">
                        <Loader2 className="h-4 w-4 animate-spin" /> Thinking…
                      </div>
                    )
                  ) : (
                    <p className="whitespace-pre-wrap break-words text-sm leading-6">{message.content}</p>
                  )}
                </div>
                {message.role === 'user' && (
                  <div className="mt-0.5 hidden h-8 w-8 shrink-0 items-center justify-center rounded-full bg-gray-200 text-gray-700 sm:flex">
                    <User className="h-4 w-4" />
                  </div>
                )}
              </article>
            ))}
            <div ref={messagesEndRef} />
          </div>
        </div>

        <div className="border-t border-gray-200 bg-white p-3 sm:p-5">
          <div className="mx-auto max-w-3xl">
            {error && (
              <div className="mb-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                {error}
              </div>
            )}
            <form onSubmit={submit} className="flex items-end gap-2 rounded-2xl border border-gray-300 bg-white p-2 shadow-sm focus-within:border-gray-500 focus-within:ring-2 focus-within:ring-gray-100">
              <textarea
                value={input}
                onChange={(event) => setInput(event.target.value)}
                onKeyDown={onInputKeyDown}
                rows={1}
                placeholder="Ask Threadify…"
                className="max-h-36 min-h-10 min-w-0 flex-1 resize-none bg-transparent px-2 py-2 text-sm leading-6 text-gray-900 outline-none placeholder:text-gray-400"
              />
              <button
                type="submit"
                disabled={!input.trim() || isSending}
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-gray-900 text-white transition hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="Send message"
              >
                {isSending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              </button>
            </form>
            <p className="mt-2 text-center text-[11px] text-gray-400">
              Threadify permissions are inherited from your signed-in account.
            </p>
          </div>
        </div>
      </section>
    </div>
  );
}
