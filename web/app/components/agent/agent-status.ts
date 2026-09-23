import { getConfig } from '~/config.client';
import { browserHeaders } from '~/lib/browser-session';

export type AgentStatus = {
  enabled: boolean;
  status: 'disabled' | 'starting' | 'ready' | 'unavailable';
  mode: 'none' | 'local' | 'external';
};

export const disabledAgent: AgentStatus = { enabled: false, status: 'disabled', mode: 'none' };

export function parseAgentStatus(value: unknown): AgentStatus {
  if (!value || typeof value !== 'object') throw new Error('Invalid agent status');
  const candidate = value as AgentStatus;
  if (candidate.enabled === false && candidate.status === 'disabled' && candidate.mode === 'none') return disabledAgent;
  if (candidate.enabled !== true || !['starting', 'ready', 'unavailable'].includes(candidate.status)
    || !['local', 'external'].includes(candidate.mode)) throw new Error('Invalid agent status');
  return { enabled: true, status: candidate.status, mode: candidate.mode };
}

export async function fetchAgentStatus(signal: AbortSignal): Promise<AgentStatus> {
  const response = await fetch(`${getConfig().engineUrl.replace(/\/+$/, '')}/v1/agent/status`, {
    credentials: 'include', headers: browserHeaders(), cache: 'no-store', signal,
  });
  // A signed-out session must not retain the previous user's agent controls.
  if (response.status === 401 || response.status === 403) return disabledAgent;
  if (!response.ok) throw new Error('Unable to check agent status');
  return parseAgentStatus(await response.json());
}

export function unavailableAgent(previous: AgentStatus): AgentStatus {
  return previous.enabled ? { ...previous, status: 'unavailable' } : disabledAgent;
}

export function agentPollInterval(status: AgentStatus): number {
  if (!status.enabled) return 60_000;
  return status.status === 'starting' ? 3_000 : status.status === 'unavailable' ? 10_000 : 30_000;
}
