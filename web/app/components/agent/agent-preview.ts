import { matchPath } from 'react-router';
import type { ProfileViewTarget } from '../profiles/view/profile-view';
import { pages } from '~/routes';

export type AgentContext = { title: string; path: string; detail?: string; resource?: Record<string, string | undefined> };

export function supportsAgent(pathname: string, search: string): boolean {
  if (pathname === '/u/settings') return new URLSearchParams(search).get('tab') === 'traces';
  return pages.some(page => page.agentSupported && matchPath(page.path, pathname));
}
export type PreviewTask = 'contract' | 'extraction' | 'investigation';
export type AgentMessage = {
  id: string;
  profileViewTarget?: ProfileViewTarget;
  role: 'user' | 'assistant';
  text: string;
  context?: AgentContext;
  tools?: { id: string; name: string; status: 'running' | 'completed' | 'failed' | 'stopped' }[];
};

export const previewTasks: { id: PreviewTask; title: string; description: string; prompt: string }[] = [
  { id: 'contract', title: 'Create a contract', description: 'Turn a workflow into Gherkin rules', prompt: 'Help me create a Gherkin contract for a refund workflow.' },
  { id: 'extraction', title: 'Shape your trace data', description: 'Understand trace ingestion filters', prompt: 'Open trace settings and explain how to configure span ingestion filters.' },
  { id: 'investigation', title: 'Understand a workflow', description: 'Inspect steps, owners, and dependencies', prompt: 'Help me understand the workflow on this page, or help me find a thread to inspect.' },
];

export function getAgentContext(pathname: string, search: string): AgentContext {
  for (const page of pages) {
    const match = matchPath(page.path, pathname);
    if (!match) continue;
    const params = Object.fromEntries(Object.entries(match.params).map(([key, value]) => {
      try { return [key, value ? decodeURIComponent(value) : value]; } catch { return [key, value]; }
    }));
    const title = typeof page.title === 'function' ? page.title(params) : page.title;
    const tab = new URLSearchParams(search).get('tab');
    const detail = match.params.id
      ? `${match.params.id}${match.params.version ? ` · v${match.params.version}` : ''}`
      : pathname === '/u/settings' && tab ? ({ traces: 'Trace ingestion', engine: 'Engine', profile: 'Profile', company: 'Company', billing: 'Billing & Credits' }[tab] ?? 'Settings tab') : undefined;
    return { title, detail, path: pathname + search, resource: { ...params, tab: tab ?? undefined } };
  }
  return { title: 'Workspace', path: pathname + search };
}
