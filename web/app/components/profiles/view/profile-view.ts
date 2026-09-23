import type { EntityTypeMetric } from '~/lib/api';
/** Versioned, declarative profile views. No code, URLs, credentials, or entity data. */
export const sources = {
  totalDeliveries: { label: 'Total deliveries', description: 'Delivery count from the profile summary.', kind: 'metric', operation: 'entityProfile', field: 'metrics.totalDeliveries' },
  completedSuccessfully: { label: 'Successful deliveries', description: 'Completed deliveries, including recovered runs.', kind: 'metric', operation: 'entityProfile', field: 'metrics.completedSuccessfully' },
  validationViolations: { label: 'Validation violations', description: 'Recorded violations from the profile summary.', kind: 'metric', operation: 'entityProfile', field: 'metrics.validationViolations' },
  averageDeliveryTimeMs: { label: 'Average delivery time', description: 'Average delivery duration in milliseconds.', kind: 'metric', operation: 'entityProfile', field: 'metrics.averageDeliveryTimeMs' },
  deliveryHealthScore: { label: 'Delivery health score', description: 'An aggregate health signal, not proof of compliance.', kind: 'metric', operation: 'entityProfile', field: 'metrics.deliveryHealthScore' },
  details: { label: 'Profile details', description: 'Identity, first observed, and last active dates.', kind: 'section', operation: 'entityProfile', field: '' },
  configuredMetric: { label: 'Configured metric', description: 'A presentation of an existing metric definition.', kind: 'section', operation: 'getComputedMetrics', field: 'computedMetrics' },
  computedMetrics: { label: 'Configured metrics', description: 'Tables and cards from this profile type’s metric definitions.', kind: 'section', operation: 'getComputedMetrics', field: 'computedMetrics' },
  deliveryHealth: { label: 'Delivery health & trends', description: 'Completion, failures, recovery, and duration for the selected period.', kind: 'section', operation: 'getDeliveryHealthMetrics', field: 'deliveryHealth' },
  deliveryTrendChart: { label: 'Delivery outcome line graph', description: 'Two-series line graph of daily completed and failed/cancelled thread counts by UTC creation date. Uses real daily_outcomes data; both series remain visible at zero.', kind: 'section', operation: 'getDeliveryHealthMetrics', field: 'deliveryHealth' },
  deliveryHealthChart: { label: 'Delivery rates chart', description: 'Horizontal bar graph comparing completion, failure, error, and recovery percentages for the selected period. Not a time series. Missing rates stay unavailable.', kind: 'section', operation: 'getDeliveryHealthMetrics', field: 'deliveryHealth' },
} as const;
export type ViewSource = keyof typeof sources;
export type ProfileView = {
  schemaVersion: 1;
  title: string;
  description: string;
  range: '7d' | '30d' | '90d';
  columns: 1 | 2;
  blocks: { id: string; source: ViewSource; title: string; metricId?: string; display?: 'card' | 'table' | 'line' | 'bar' }[];
};
export type ProfileViewTarget = { key: string; revision: number };
export type ProfileViewDesignerBridge = ProfileViewTarget & {
  definition: ProfileView;
  metrics?: { id: string; name: string }[];
  metricDraft?: EntityTypeMetric[];
  templates?: unknown[];
  applyMetrics?: (metrics: EntityTypeMetric[]) => void;
  apply: (definition: ProfileView) => void;
};
export const defaultView = (): ProfileView => ({
  schemaVersion: 1, title: 'Overview', description: 'The activity and evidence that matter for this entity.', range: '30d', columns: 2,
  blocks: ['totalDeliveries', 'completedSuccessfully', 'validationViolations', 'averageDeliveryTimeMs', 'details'].map(source => ({ id: source, source: source as ViewSource, title: sources[source as ViewSource].label })),
});

/** Fill presentation gaps from the type's saved data definitions. Entity data is
 * resolved at render time; a metric never needs a second creation step. */
export function withMetricPresentations(view: ProfileView, metrics: EntityTypeMetric[]): ProfileView {
  const savedMetrics = metrics.filter(metric => metric.id);
  const ids = new Set(savedMetrics.map(metric => metric.id));
  const blocks = view.blocks.filter(block => block.source !== 'configuredMetric' || ids.has(block.metricId));
  // A legacy "all metrics" section already presents the full metric catalog.
  if (!blocks.some(block => block.source === 'computedMetrics')) {
    for (const metric of savedMetrics) {
      if (blocks.some(block => block.metricId === metric.id)) continue;
      const group = metric.custom_definition?.group_by;
      let id = `metric_${metric.id}`;
      while (blocks.some(block => block.id === id)) id = `_${id}`;
      blocks.push({ id, source: 'configuredMetric', metricId: metric.id,
        title: (metric.name || metric.custom_definition?.name || 'Metric').slice(0, 100),
        display: group === 'period' ? 'line' : group && group !== 'none' ? 'table' : 'card' });
    }
  }
  return { ...view, blocks };
}

export function validateProfileView(value: unknown): ProfileView {
  const fail = (message: string): never => { throw new Error(message); };
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail('A profile view must be an object.');
  const v = value as Record<string, unknown>;
  const allowed = ['schemaVersion', 'title', 'description', 'range', 'columns', 'blocks'];
  if (Object.keys(v).some(key => !allowed.includes(key))) return fail('The view contains unsupported properties.');
  if (v.schemaVersion !== 1) return fail('Unsupported profile view version.');
  if (typeof v.title !== 'string' || !v.title.trim() || v.title.length > 100) return fail('Use a view title of 1–100 characters.');
  if (typeof v.description !== 'string' || v.description.length > 500) return fail('Use a description of at most 500 characters.');
  if (typeof v.range !== 'string' || !['7d', '30d', '90d'].includes(v.range)) return fail('Choose a 7, 30, or 90 day range.');
  if (v.columns !== 1 && v.columns !== 2) return fail('Choose one or two columns.');
  if (!Array.isArray(v.blocks) || v.blocks.length < 1 || v.blocks.length > 100) return fail('Add between 1 and 100 blocks.');
  const ids = new Set<string>(); const usedSources = new Set<string>();
  const blocks = v.blocks.map(block => {
    if (!block || typeof block !== 'object' || Array.isArray(block)) return fail('Each block must be an object.');
    if (Object.keys(block).some(key => !['id', 'source', 'title', 'metricId', 'display'].includes(key))) return fail('A block contains unsupported properties.');
    if (typeof block.id !== 'string' || !/^[a-zA-Z0-9_-]{1,80}$/.test(block.id) || ids.has(block.id)) return fail('Block IDs must be unique letters, numbers, dashes, or underscores.');
    if (typeof block.source !== 'string' || !Object.hasOwn(sources, block.source)) return fail('Choose a supported Threadify data source.');
    const binding = block.source === 'configuredMetric' ? `metric:${block.metricId}` : block.source;
    if (block.source === 'configuredMetric') {
      if (typeof block.metricId !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(block.metricId)) return fail('Choose a saved metric definition.');
      if (!['card', 'table', 'line', 'bar'].includes(block.display)) return fail('Choose card, table, line, or bar presentation.');
    } else if (block.metricId !== undefined || block.display !== undefined) return fail('Metric bindings are only supported on configured metrics.');
    if (usedSources.has(binding)) return fail('Use each data source only once.');
    if (typeof block.title !== 'string' || !block.title.trim() || block.title.length > 100) return fail('Use block titles of 1–100 characters.');
    ids.add(block.id); usedSources.add(binding);
    return { id: block.id, source: block.source as ViewSource, title: block.title.trim(), ...(block.source === 'configuredMetric' ? { metricId: block.metricId as string, display: block.display as 'card' | 'table' | 'line' | 'bar' } : {}) };
  });
  return { schemaVersion: 1, title: v.title.trim(), description: v.description, range: v.range as ProfileView['range'], columns: v.columns, blocks };
}

/** Runtime parameters are resolved from the authenticated page, never from generated input. */
export function compileProfileRequests(view: ProfileView) {
  return view.blocks.map(block => ({
    blockId: block.id, operation: sources[block.source].operation, field: sources[block.source].field,
    parameters: { ...(block.metricId ? { metricId: block.metricId } : {}), refKey: '$entity.refKey', type: '$profileType.name',
        ...(['configuredMetric', 'computedMetrics', 'deliveryHealth', 'deliveryHealthChart', 'deliveryTrendChart'].includes(block.source) ? { range: view.range } : {}) },
  }));
}
export function profileViewKey(companyId: string, userId: string, typeId: string) {
  if (!companyId || !userId || !typeId) throw new Error('Sign in to save a profile view.');
  return `threadify:profile-view:v1:${[companyId, userId, typeId].map(encodeURIComponent).join(':')}`;
}
export function loadProfileView(storage: Pick<Storage, 'getItem'>, key: string): ProfileView | null {
  const raw = storage.getItem(key);
  if (!raw) return null;
  const envelope = JSON.parse(raw);
  // Early local drafts allowed history in Overview. Keep those views usable while
  // moving history back to its dedicated tab; all new proposals reject it.
  const definition = envelope.definition;
  if (definition?.schemaVersion === 1 && Array.isArray(definition.blocks)) {
    const blocks = definition.blocks.filter((block: { source?: string } | null) => block?.source !== 'history');
    if (blocks.length !== definition.blocks.length) {
      if (!blocks.length) return null;
      return validateProfileView({ ...definition, blocks });
    }
  }
  return validateProfileView(definition);
}
export function saveProfileView(storage: Pick<Storage, 'setItem'>, key: string, definition: ProfileView) {
  const valid = validateProfileView(definition);
  storage.setItem(key, JSON.stringify({ definition: valid, requests: compileProfileRequests(valid), savedAt: new Date().toISOString() }));
  return valid;
}
export function readProfileViewProposal(text: string): ProfileView | null {
  const match = /```profile-view\s*\n([\s\S]*?)```/.exec(text);
  return match ? validateProfileView(JSON.parse(match[1])) : null;
}
/** Validate generated data before it can enter the deterministic form. The server
 * still validates supported fields, filters and template parameters on save. */
function validateMetricDraft(value: unknown): EntityTypeMetric[] {
  const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
  const shortText = (v: unknown) => typeof v === 'string' && !!v.trim() && v.length <= 255;
  const fail = (): never => { throw new Error('Provide metric definitions with a name and a template or valid custom_definition.'); };
  if (!Array.isArray(value) || value.length > 50) return fail();
  for (const metric of value) {
    if (!record(metric) || Object.keys(metric).some(key => !['name', 'template_id', 'parameters', 'custom_definition'].includes(key))) return fail();
    if (metric.name !== undefined && !shortText(metric.name)) return fail();
    if (metric.template_id !== undefined && !shortText(metric.template_id)) return fail();
    if (metric.parameters !== undefined && !record(metric.parameters)) return fail();
    if (!metric.template_id && !metric.custom_definition) return fail();
    if (metric.custom_definition !== undefined) {
      const def = metric.custom_definition;
      if (!record(def) || !shortText(def.name) || !shortText(def.field) || !['COUNT', 'RATE', 'AVG', 'SUM', 'MIN', 'MAX'].includes(String(def.operation))) return fail();
      if (Object.keys(def).some(key => !['name', 'operation', 'field', 'target', 'step_name', 'filters', 'group_by', 'granularity', 'visualisation'].includes(key))) return fail();
      if (def.target !== undefined && !['thread', 'step'].includes(String(def.target))) return fail();
      for (const key of ['step_name', 'group_by', 'granularity', 'visualisation']) if (def[key] !== undefined && typeof def[key] !== 'string') return fail();
      if (def.filters !== undefined && (!Array.isArray(def.filters) || def.filters.length > 50 || def.filters.some(filter => !record(filter) || !shortText(filter.key) || typeof filter.value !== 'string' || Object.keys(filter).some(key => !['key', 'value'].includes(key))))) return fail();
    }
  }
  return value as EntityTypeMetric[];
}
export function applyProfileViewProposal(text: string, target: ProfileViewTarget | undefined, designer: ProfileViewDesignerBridge | null) {
  if (!target || !designer || designer.key !== target.key || designer.revision !== target.revision) throw new Error('The profile draft has changed. Ask the agent to revise the current view.');
  const metricProposal = /```profile-metrics\s*\n([\s\S]*?)```/.exec(text);
  if (metricProposal) {
    const metrics = validateMetricDraft(JSON.parse(metricProposal[1]));
    if (!designer.applyMetrics) throw new Error('Open the profile configuration to apply metric changes.');
    designer.applyMetrics(metrics);
    return;
  }
  const definition = readProfileViewProposal(text);
  if (!definition) throw new Error('No profile view proposal found.');
  designer.apply(definition);
}
export function profileViewInstructions(designer: ProfileViewDesignerBridge) {
  return `You are configuring one Threadify profile type. There are two connected tabs: Data & metrics defines fetching/calculation; Presentation references saved definitions. For metric changes return the COMPLETE metric array in a fenced profile-metrics block. Each metric contains name and either template_id plus parameters, or custom_definition. For example: [{"name":"Daily volume","custom_definition":{"name":"Daily volume","operation":"COUNT","field":"threads","target":"thread","filters":[],"group_by":"period","granularity":"day"}}]. Put operation/field inside custom_definition, never at the top level. Strip database id fields from the complete metric array. Custom definitions contain name, operation COUNT|RATE|AVG|SUM|MIN|MAX, field (threads/steps/duration/violations/retries/outcome), optional target thread|step, filters array of {key,value}, group_by, granularity. No SQL/code/endpoints. Available templates with parameter definitions: ${JSON.stringify(designer.templates ?? [])}. Current metric draft: ${JSON.stringify(designer.metricDraft ?? [])}. Applying only edits the data draft. The user must save Data & metrics before a new metric can be referenced in Presentation. Never invent an ID for a new metric; wait for the saved catalog. For presentation changes use profile-view below. Current draft: ${JSON.stringify(designer.definition)}. Available configured metrics: ${JSON.stringify(designer.metrics ?? [])}. Every configured metric gets a default presentation. Preserve all metric presentations and change their display or order when requested. For metric presentations use source:configuredMetric, metricId from this list, and display:card|table|line|bar. Never invent metric IDs. Use line only for temporal grouped metrics. Prefer references to configured metrics over built-in sections. Supported sources: ${JSON.stringify(sources)}. When proposing a change, return a complete JSON definition in a fenced code block tagged profile-view. Keep schemaVersion:1, title (1-100 chars), description (0-500 chars), range (7d|30d|90d), columns (1|2), and blocks (1-100 objects containing id, source, title and, for configuredMetric only, metricId and display). IDs must be unique alphanumeric/dash/underscore strings; metric references and other sources must be unique and from the supported catalog. Do not invent metrics, fields, filters, calls, or arbitrary code. Thread history belongs in the existing History tab and must never be included in an Overview view. The user can apply your proposal to the preview, then explicitly save it as the shared Overview for this profile type if they have update permission. You cannot publish it. Do not use or offer the contract draft editor for profile views; the user applies the proposal with the Apply to preview button. Summary cards use the profile summary; range only affects configured metrics and delivery health. deliveryHealth renders cards; deliveryHealthChart renders a bar graph of aggregate rates, not a time series. Null profile summary metrics do not imply delivery health is unavailable because it loads from a separate operation. Use deliveryTrendChart for a real line graph of daily outcome counts, which loads daily_outcomes from delivery health. Do not infer daily values from aggregate rates. Explain unavailable data and ask what the user wants when needed.`;
}
