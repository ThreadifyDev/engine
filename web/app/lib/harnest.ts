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
    throw new Error(
      payload?.detail || payload?.error || `Threadify agent request failed (${response.status})`
    );
  }
  return response;
}

export const harnest = {
  async listSessions(): Promise<HarnestSession[]> {
    const response = await request('sessions?limit=100');
    return ((await response.json()) as SessionPage).sessions || [];
  },

  async createSession(title: string): Promise<HarnestSession> {
    const response = await request('sessions', {
      method: 'POST',
      body: JSON.stringify({ state: { title, source: 'threadify-web' } }),
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
    signal?: AbortSignal
  ): Promise<void> {
    const response = await request('responses', {
      method: 'POST',
      body: JSON.stringify({
        input,
        sessionId,
        stream: true,
        metadata: { source: 'threadify-web' },
      }),
      signal,
      headers: { Accept: 'text/event-stream' },
    });

    if (!response.body) throw new Error('The Threadify agent returned an empty stream.');

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
      onEvent(event);
    };

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split(/\r?\n\r?\n/);
      buffer = frames.pop() || '';
      frames.forEach(consumeFrame);
    }

    buffer += decoder.decode();
    if (buffer.trim()) consumeFrame(buffer);
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
