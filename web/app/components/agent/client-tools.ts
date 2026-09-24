import type { HarnestClientTool } from '~/lib/harnest';

export type ContractDraft = {
  source: string;
  revision: number;
  open: boolean;
  preview?: { valid: boolean; errors?: string[]; warnings?: string[] };
};

export type ClientToolHost = {
  getContext: () => unknown;
  navigate: (path: string) => Promise<void>;
  getDraft: () => ContractDraft;
  writeDraft: (source: string) => ContractDraft;
  previewDraft: (draft: ContractDraft) => Promise<unknown>;
};

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function navigationPath(args: Record<string, unknown>): string {
  const routes: Record<string, string> = { dashboard: '/u/dashboard', threads: '/u/threads', contracts: '/u/contracts', profiles: '/u/profiles', trace_settings: '/u/settings?tab=traces' };
  const page = typeof args.page === 'string' ? args.page : '';
  if (Object.hasOwn(routes, page)) return routes[page];
  if ((page === 'thread' || page === 'contract') && typeof args.id === 'string' && uuid.test(args.id)) return `/u/${page}s/${args.id}`;
  if (page === 'profile_designer' && typeof args.profile_type === 'string' && args.profile_type.trim() && args.profile_type.length <= 500 && !['.', '..'].includes(args.profile_type)) return `/u/profile-views/${encodeURIComponent(args.profile_type)}`;
  if (page === 'entity_profile' && typeof args.profile_type === 'string' && typeof args.ref_key === 'string'
    && args.profile_type.trim() && args.ref_key.trim() && args.profile_type.length <= 500 && args.ref_key.length <= 500
    && !['.', '..'].includes(args.profile_type) && !['.', '..'].includes(args.ref_key)) {
    return `/u/profiles/${encodeURIComponent(args.profile_type)}/${encodeURIComponent(args.ref_key)}`;
  }
  throw new Error('Unsupported page or missing resource identifier.');
}

export async function executeFrontendTool(call: HarnestClientTool, host: ClientToolHost, signal: AbortSignal): Promise<unknown> {
  signal.throwIfAborted();
  const args = call.arguments;
  try {
    if (!args || typeof args !== 'object' || Array.isArray(args)) throw new Error('Tool arguments must be an object.');
    if (call.name === 'get_page_context') return { ok: true, context: host.getContext() };
    if (call.name === 'navigate_ui') {
      const path = navigationPath(args);
      await host.navigate(path);
      return { ok: true, path };
    }
    if (call.name !== 'open_contract_draft' && call.name !== 'preview_contract_draft') throw new Error('Unknown frontend tool.');
    const draft = host.getDraft();
    if (!Number.isInteger(args.expected_revision) || args.expected_revision !== draft.revision) {
      return { ok: false, error: 'DRAFT_CONFLICT', revision: draft.revision, message: 'Read the current draft before changing or previewing it.' };
    }
    if (call.name === 'open_contract_draft') {
      const source = args.source;
      if (typeof source !== 'string' || source.length > 100_000 || !source.split(/\r?\n/).find(line => line.trim() && !line.trim().startsWith('#'))?.trim().startsWith('Feature:')) {
        throw new Error('Provide complete Gherkin source starting with Feature:, up to 100000 characters.');
      }
      const updated = host.writeDraft(source);
      await host.navigate('/u/contracts');
      return { ok: true, revision: updated.revision, published: false };
    }
    const preview = await host.previewDraft(draft);
    signal.throwIfAborted();
    if (host.getDraft().revision !== draft.revision) return { ok: false, error: 'DRAFT_CONFLICT', message: 'The draft changed during validation. Preview the current revision again.' };
    return { ok: true, revision: draft.revision, preview, published: false };
  } catch (error) {
    signal.throwIfAborted();
    return { ok: false, error: error instanceof Error ? error.message : 'Frontend action failed.' };
  }
}
