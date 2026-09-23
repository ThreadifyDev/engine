import { browserHeaders, csrfToken } from './browser-session';

export interface HarnestSession {
  id: string;
  userId: string;
  state: Record<string, unknown>;
  applicationData: Record<string, unknown>;
  createdAt: string | null;
  updatedAt: string | null;
  metadata: Record<string, unknown>;
}

export interface HarnestSessionMessage {
  id: string;
  role: string;
  content: unknown;
  createdAt: string | null;
  metadata: Record<string, unknown>;
}

export interface HarnestStreamEvent {
  type: string;
  sequence?: number;
  responseId?: string;
  sessionId?: string;
  delta?: string;
  id?: string;
  callId?: string;
  name?: string;
  arguments?: Record<string, unknown>;
  output?: unknown;
  outputText?: string;
  status?: string;
  error?: string;
  clientTool?: HarnestClientTool;
  requiredAction?: { type: string } & Partial<HarnestClientTool>;
}

export interface HarnestClientTool {
  id: string;
  callId: string;
  name: string;
  arguments: Record<string, unknown>;
}

type SessionPage = { sessions: HarnestSession[]; nextCursor: string | null };
type MessagePage = {
  sessionId: string;
  userId: string;
  messages: HarnestSessionMessage[];
  nextCursor: string | null;
};

async function request(path: string, init: RequestInit = {}) {
  const token = csrfToken();
  if (!token) throw new Error('Please sign in again to use the Threadify agent.');

  const response = await fetch(`/api/harnest/${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      ...browserHeaders(),
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...init.headers,
    },
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    const detail = payload?.detail || payload?.error;
    throw new Error(typeof detail === 'string' ? detail : response.status === 503 || response.status === 502 || response.status === 404
      ? 'The agent is unavailable. Check the Engine’s agent connection.'
      : `Threadify agent request failed (${response.status})`);
  }
  return response;
}

export const harnest = {
  async listSessions(): Promise<HarnestSession[]> {
    const response = await request('sessions?limit=100');
    return ((await response.json()) as SessionPage).sessions || [];
  },

  async createSession(title: string, signal?: AbortSignal): Promise<HarnestSession> {
    const response = await request('sessions', {
      method: 'POST',
      body: JSON.stringify({ state: { title, source: 'threadify-web' } }),
      signal,
    });
    return response.json();
  },

  async getMessages(sessionId: string): Promise<HarnestSessionMessage[]> {
    const response = await request(`sessions/${encodeURIComponent(sessionId)}/messages?limit=100`);
    return ((await response.json()) as MessagePage).messages || [];
  },

  async deleteSession(sessionId: string): Promise<void> {
    await request(`sessions/${encodeURIComponent(sessionId)}`, { method: 'DELETE' });
  },

  async streamResponse(
    input: string,
    sessionId: string,
    onEvent: (event: HarnestStreamEvent) => void,
    signal?: AbortSignal,
    executeClientTool?: (tool: HarnestClientTool) => Promise<unknown>,
    pageContext?: unknown,
  ): Promise<void> {
    const response = await request('responses', {
      method: 'POST',
      body: JSON.stringify({
        input,
        sessionId,
        stream: true,
        metadata: { source: 'threadify-web', pageContext },
      }),
      signal,
      headers: { Accept: 'text/event-stream' },
    });

    if (!response.body) throw new Error('The Threadify agent returned an empty stream.');

    let pending: HarnestClientTool | undefined;
    let completed = false;
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    const consumeFrame = (frame: string) => {
      const data = frame
        .split(/\r?\n/)
        .filter((line) => line.startsWith('data:'))
        .map((line) => line.slice(5).trimStart())
        .join('\n');
      if (!data) return;
      const event = JSON.parse(data) as HarnestStreamEvent;
      if (event.type === 'client_tool.requested') pending = event.clientTool;
      if (event.type === 'response.completed') {
        completed = true;
        if (event.status === 'requires_action' && event.requiredAction?.type !== 'client_tool') {
          throw new Error('The agent requested an unsupported action. Start a new conversation.');
        }
        if (event.status === 'requires_action') {
          const action = event.requiredAction;
          if (!action?.id || !action.name || !action.arguments) throw new Error('The agent returned an incomplete frontend action.');
          pending = action as HarnestClientTool;
        } else if (event.status !== 'completed') {
          throw new Error('The agent did not complete the response.');
        }
      }
      onEvent(event);
    };

    try { while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split(/\r?\n\r?\n/);
      buffer = frames.pop() || '';
      frames.forEach(consumeFrame);
    }

    buffer += decoder.decode();
    if (buffer.trim()) consumeFrame(buffer);
    } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
    if (!completed) throw new Error('The agent connection ended before the response completed.');

    // Harnest closes SSE at a client-tool boundary. Submit the result to resume
    // that exact suspended invocation; JSON continuations may request more tools.
    const handled = new Set<string>();
    for (let count = 0; pending; count++) {
      signal?.throwIfAborted();
      if (count >= 24 || handled.has(pending.id)) throw new Error('The agent exceeded the client action limit.');
      if (!executeClientTool || !pending.id || !pending.name) throw new Error('The agent requested an unavailable frontend tool.');
      handled.add(pending.id);
      const output = await executeClientTool(pending);
      signal?.throwIfAborted();
      const resumed = await request(`client-tools/${encodeURIComponent(pending.id)}`, {
        method: 'POST', body: JSON.stringify({ output }), signal,
      });
      const result = await resumed.json() as HarnestStreamEvent;
      if (result.status === 'requires_action') {
        const action = result.requiredAction;
        if (action?.type !== 'client_tool' || !action.id || !action.name || !action.arguments) throw new Error('The agent requested an unsupported action.');
        pending = action as HarnestClientTool;
        onEvent({ ...result, type: 'client_tool.requested', clientTool: pending });
      } else {
        if (result.status !== 'completed') throw new Error('The agent did not complete the response.');
        pending = undefined;
        onEvent({ ...result, type: 'response.completed' });
      }
    }
  },
};

export function sessionTitle(session: HarnestSession): string {
  const title = session.state?.title;
  return typeof title === 'string' && title.trim()
    ? title
    : `Conversation ${session.id.slice(0, 8)}`;
}

export function messageText(content: unknown): string {
  if (typeof content === 'string') return content;
  if (!Array.isArray(content)) return content == null ? '' : JSON.stringify(content);
  return content
    .map((item) => {
      if (typeof item === 'string') return item;
      if (item && typeof item === 'object' && 'text' in item) {
        return String((item as { text: unknown }).text || '');
      }
      return '';
    })
    .join('');
}
