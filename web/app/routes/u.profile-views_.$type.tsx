import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router';
import { ArrowLeft, Check, Loader2, Save, Sparkles } from 'lucide-react';
import { TabBar } from '~/components/TabBar';
import { ProfileTypeModal } from '~/components/profiles/ProfileTypeModal';
import { useQueryClient } from '@tanstack/react-query';
import AppLayout from '~/components/AppLayout';
import { useAgent } from '~/components/agent/agent-context';
import { api, ProfileViewConflictError, type ProfileViewResponse, type EntityProfile, type EntityProfileType, type MetricsTemplateResponse, type EntityTypeMetric } from '~/lib/api';
import { graphqlClient, type EntityProfileListItem } from '~/lib/graphql';
import ProfileViewControls from '~/components/profiles/view/ProfileViewControls';
import ProfileViewRenderer from '~/components/profiles/view/ProfileViewRenderer';
import { compileProfileRequests, withMetricPresentations, defaultView, loadProfileView, profileViewKey, validateProfileView, sources, type ProfileView } from '~/components/profiles/view/profile-view';

const inputClass = 'mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-gray-400 focus:outline-none focus:ring-2 focus:ring-gray-900/10';
const buttonClass = 'inline-flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40';

export default function ProfileViewPage() {
  const { type } = useParams();
  return <AppLayout><ProfileViewDesigner key={type} type={type ?? ''} /></AppLayout>;
}

function ProfileViewDesigner({ type }: { type: string }) {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [configTab, setConfigTab] = useState<'data' | 'presentation'>(params.get('tab') === 'data' ? 'data' : 'presentation');
  const [templates, setTemplates] = useState<MetricsTemplateResponse[]>([]);
  const [dataDirty, setDataDirty] = useState(false);
  const [metricDraft, setMetricDraft] = useState<EntityTypeMetric[]>([]);
  const [metricOverride, setMetricOverride] = useState<EntityTypeMetric[] | null>(null);
  const queryClient = useQueryClient();
  useEffect(() => { api.listMetricsTemplates().then(result => setTemplates(result.data ?? [])).catch(() => {}); }, []);
  const dataDraftFingerprint = useRef('');
  const onDataDraft = useCallback((draft: Partial<EntityProfileType>, changed: boolean) => { setDataDirty(changed); setMetricDraft(draft.metrics ?? []); const fingerprint = JSON.stringify(draft); if (dataDraftFingerprint.current !== fingerprint) { dataDraftFingerprint.current = fingerprint; revisionRef.current += 1; setRevision(revisionRef.current); } }, []);
  const { isEnabled: agentEnabled, openAgent, setComposer, setIncludeContext, registerProfileDesigner } = useAgent();
  const metricCatalog = useRef<EntityTypeMetric[] | null>(null);
  const [profileType, setProfileType] = useState<EntityProfileType | null>(null);
  const [entities, setEntities] = useState<EntityProfileListItem[]>([]);
  const [refKey, setRefKey] = useState(params.get('ref') ?? '');
  const [profile, setProfile] = useState<EntityProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const [error, setError] = useState('');
  const [storageWarning, setStorageWarning] = useState('');
  const [localView, setLocalView] = useState<ProfileView | null>(null);
  const [serverRevision, setServerRevision] = useState(0);
  const [canManage, setCanManage] = useState(false);
  const [backendReady, setBackendReady] = useState(false);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [reloading, setReloading] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [definition, setDefinition] = useState<ProfileView>(defaultView);
  const definitionRef = useRef(definition);
  const revisionRef = useRef(0);
  const [draftSession] = useState(() => crypto.randomUUID());
  const [revision, setRevision] = useState(0);
  const [saved, setSaved] = useState<ProfileView | null>(null);
  const [selected, setSelected] = useState('totalDeliveries');
  const [status, setStatus] = useState('');
  const dirty = JSON.stringify(definition) !== JSON.stringify(saved ?? defaultView());
  const change = useCallback((next: ProfileView) => {
    if (metricCatalog.current) next = withMetricPresentations(next, metricCatalog.current);
    definitionRef.current = next;
    revisionRef.current += 1;
    setRevision(revisionRef.current);
    setDefinition(next);
    setStatus('');
  }, []);

  const acceptShared = useCallback((response: ProfileViewResponse) => {
    const stored = response.data.definition ? validateProfileView(response.data.definition) : null;
    const next = withMetricPresentations(stored ?? defaultView(), metricCatalog.current ?? []);
    setSaved(next); setServerRevision(response.data.revision); setCanManage(response.can_manage);
    setBackendReady(true); setConflict(false); setStorageWarning('');
    change(next); setSelected(next.blocks[0].id);
  }, [change]);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const [result, session] = await Promise.all([api.listEntityProfileTypes(), api.session()]);
        if (!active) return;
        const found = result.data?.find((item: EntityProfileType) => item.name === type);
        if (!found) throw new Error('Profile type not found. Create a profile type before designing its view.');
        metricCatalog.current = found.metrics ?? [];
        setProfileType(found);
        const user = session.user;
        if (user && user.company_id === found.company_id) {
          try { setLocalView(loadProfileView(localStorage, profileViewKey(found.company_id, user.id, found.id))); }
          catch { /* Legacy browser storage is optional; the shared view is authoritative. */ }
        }
        try {
          const response = await api.getProfileView(found.id);
          if (active) acceptShared(response);
        } catch (cause) {
          if (active) setStorageWarning(cause instanceof Error ? cause.message : 'Could not load the shared view.');
        }
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : 'Could not load profile type.');
      } finally { if (active) setLoading(false); }
    })();
    return () => { active = false; };
  }, [type, acceptShared]);

  useEffect(() => {
    let active = true;
    graphqlClient.getEntityProfilesByType({ type, limit: 20 }).then(result => {
      if (active) { setEntities(result.items); setRefKey(current => current || result.items[0]?.refKey || ''); }
    }).catch(() => { if (active) setPreviewError('Could not load preview entities. You can still design the layout or enter an entity reference below.'); });
    return () => { active = false; };
  }, [type]);

  useEffect(() => {
    let active = true;
    setProfile(null);
    if (!refKey) { setPreviewLoading(false); setPreviewError(''); return; }
    setPreviewLoading(true); setPreviewError('');
    graphqlClient.getEntityProfile({ refKey, type }).then(result => {
      if (active) { setProfile(result); if (!result) setPreviewError('No data is available for this entity yet.'); }
    }).catch(cause => { if (active) setPreviewError(cause instanceof Error ? cause.message : 'Could not load preview.'); })
      .finally(() => { if (active) setPreviewLoading(false); });
    return () => { active = false; };
  }, [refKey, type]);

  useEffect(() => {
    if (!profileType) return;
    return registerProfileDesigner({ key: `${profileType.id}:${draftSession}`, revision, definition, metricDraft, templates, applyMetrics: metrics => { setMetricOverride(metrics); setConfigTab('data'); change(definitionRef.current); }, metrics: (profileType.metrics ?? []).filter(metric => metric.id).map(metric => ({ id: metric.id!, name: metric.name || metric.custom_definition?.name || metric.template_id || 'Metric' })), apply: next => {
      if (revisionRef.current !== revision) throw new Error('The draft has changed. Ask for an updated proposal.');
      change(next); setConfigTab('presentation'); setSelected(next.blocks[0].id); setStatus('Agent proposal applied. Review the preview, then save.');
    } });
  }, [profileType, draftSession, revision, definition, metricDraft, templates, registerProfileDesigner, change]);

  useEffect(() => {
    if (!dirty && !dataDirty) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ''; };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty, dataDirty]);

  const startAI = () => {
    if (!agentEnabled) return;
    setIncludeContext(true);
    setComposer(configTab === 'data' ? `Help me configure the data and metrics for ${type}. I want to measure ` : `Help me configure the presentation for ${type} using its configured metric definitions. I want to see `);
    openAgent();
  };
  const launchedAI = useRef(false);
  useEffect(() => {
    if (agentEnabled && profileType && params.get('ai') === '1' && !launchedAI.current) {
      launchedAI.current = true; startAI();
    }
  }, [profileType, agentEnabled]);

  const save = async () => {
    if (!profileType || !backendReady || !canManage || conflict || dataDirty || savingRef.current) return;
    savingRef.current = true; setSaving(true); setStorageWarning('');
    const submittedRevision = revisionRef.current;
    try {
      const input = validateProfileView(definitionRef.current);
      const response = await api.saveProfileView(profileType.id, input, serverRevision);
      const valid = validateProfileView(response.data.definition);
      setSaved(withMetricPresentations(valid, metricCatalog.current ?? [])); setServerRevision(response.data.revision); setCanManage(response.can_manage);
      const draftUnchanged = revisionRef.current === submittedRevision;
      if (draftUnchanged) change(valid);
      setStatus(draftUnchanged
        ? `Saved for all ${type} profiles in this workspace.`
        : 'Shared view saved. Your newer draft changes are still unsaved.');
    } catch (cause) {
      if (cause instanceof ProfileViewConflictError) setConflict(true);
      setStorageWarning(cause instanceof Error ? cause.message : 'Could not save this view.');
    } finally { savingRef.current = false; setSaving(false); }
  };
  const reloadShared = async () => {
    if (!profileType || reloading || savingRef.current) return;
    setReloading(true);
    const startedRevision = revisionRef.current;
    try {
      const response = await api.getProfileView(profileType.id);
      if (revisionRef.current !== startedRevision) throw new Error('Your draft changed while loading. Try loading the shared view again.');
      acceptShared(response); setStatus('Latest shared view loaded.');
    } catch (cause) { setStorageWarning(cause instanceof Error ? cause.message : 'Could not reload the shared view.'); }
    finally { setReloading(false); }
  };
  const back = `/u/profiles/${encodeURIComponent(type)}`;
  const move = (index: number, offset: number) => {
    const blocks = [...definition.blocks];
    [blocks[index], blocks[index + offset]] = [blocks[index + offset], blocks[index]];
    change({ ...definition, blocks });
  };

  if (loading) return <div role="status" className="flex items-center justify-center gap-2 p-16 text-sm text-gray-500"><Loader2 className="h-4 w-4 animate-spin" />Loading profile designer…</div>;
  if (error || !profileType) return <div className="p-8"><Link to="/u/profiles" className="text-sm underline">Back to profiles</Link><p role="alert" className="mt-5 text-sm text-red-700">{error}</p></div>;

  return <div className="min-h-screen min-w-0 bg-[#f8f8f6] px-4 py-7 sm:px-7 sm:py-10 lg:px-10"><div className="mx-auto max-w-7xl">
    <button className="mb-5 inline-flex items-center gap-2 text-sm text-gray-500 hover:text-gray-900" onClick={() => { if ((!dirty && !dataDirty) || window.confirm('Leave without saving your profile view changes?')) navigate(back); }}><ArrowLeft className="h-4 w-4" />Back to {type} profiles</button>
    <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div className="min-w-0"><p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Profile configuration</p><h1 className="break-words text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">{type}</h1><p className="mt-2 max-w-xl text-sm leading-relaxed text-gray-500">Define the data once, then choose how each profile presents it.</p></div>
      <div className="flex flex-wrap gap-2">{agentEnabled && <button className={buttonClass} onClick={startAI}><Sparkles className="h-4 w-4" />Configure with AI</button>}{configTab === 'presentation' && <button className="inline-flex items-center gap-2 rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-700 disabled:opacity-40" disabled={!backendReady || !canManage || saving || reloading || conflict || dataDirty || !definition.blocks.length} onClick={save}><Save className="h-4 w-4" />{saving ? 'Saving…' : 'Save presentation'}</button>}</div>
    </header>
    <div className="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-stone-200 bg-white shadow-sm px-4 py-3 text-xs leading-relaxed text-gray-500"><span>Applies to all <strong className="font-medium text-gray-800">{type}</strong> profiles in this workspace. {backendReady && !canManage ? 'You can preview changes; saving requires profile-type update permission.' : 'Saving updates the Overview for everyone with access.'}</span><span className="inline-flex items-center gap-1.5 whitespace-nowrap">{dirty ? <><span className="h-1.5 w-1.5 rounded-full bg-amber-500" />Unsaved changes</> : saved ? <><Check className="h-3.5 w-3.5 text-emerald-700" />Shared revision {serverRevision}</> : 'Starter view'}</span></div>
    {storageWarning && <div role="alert" className="mb-4 rounded-lg bg-amber-50 p-3 text-sm text-amber-900"><p>{storageWarning}</p><button disabled={reloading || saving} className="mt-2 underline disabled:opacity-40" onClick={reloadShared}>{reloading ? 'Loading…' : 'Discard draft and load latest shared view'}</button></div>}
    {localView && <div className="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-gray-200 p-3 text-xs text-gray-600"><span>A previous browser-only view is available. Import it into the preview before saving it for the workspace.</span><button disabled={saving || reloading} className="font-medium underline disabled:opacity-40" onClick={() => { change(localView); setSelected(localView.blocks[0].id); setLocalView(null); setStatus('Browser view imported into the draft. Review it before saving for everyone.'); }}>Import browser view</button></div>}
    {status && <p role="status" className="mb-4 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-900">{status}</p>}
    <TabBar label="Profile configuration" value={configTab} onChange={setConfigTab} panelId="profile-configuration-panel" className="mb-5" items={[{ value: 'data', label: 'Data & metrics' }, { value: 'presentation', label: 'Presentation' }]} />
    {dataDirty && <p className="mb-4 text-sm text-amber-800">Save your data and metric changes before saving the presentation. The preview uses the last saved metric definitions.</p>}
    <div id="profile-configuration-panel" role="tabpanel" aria-label={configTab === 'data' ? 'Data & metrics' : 'Presentation'}>
    <div hidden={configTab !== 'data'}><ProfileTypeModal embedded isOpen mode="edit" initialData={profileType} metricsTemplates={templates} persistedTypes={profileType.type} onClose={() => {}} onRefresh={async () => {}} onDraftChange={onDataDraft} metricOverride={metricOverride} onSaved={updated => { metricCatalog.current = updated.metrics ?? []; setProfileType(updated); setSaved(current => withMetricPresentations(current ?? defaultView(), updated.metrics ?? [])); setDataDirty(false); queryClient.invalidateQueries({ queryKey: ['profile-metric-presentations'] }); change(definitionRef.current); setStatus('Data and metrics saved. Each metric now has a default presentation; adjust it in Presentation.'); }} /></div>
    <div hidden={configTab !== 'presentation'}>
    <div className="grid min-w-0 items-start gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
      <ProfileViewControls metrics={profileType.metrics ?? []} onAddMetric={metric => { const id = crypto.randomUUID(); change({ ...definition, blocks: [...definition.blocks, { id, source: 'configuredMetric', metricId: metric.id, display: metric.custom_definition?.group_by && metric.custom_definition.group_by !== 'none' ? 'table' : 'card', title: metric.name || metric.custom_definition?.name || metric.template_id || 'Metric' }] }); setSelected(id); }} definition={definition} selected={selected} dirty={dirty} onChange={change} onSelect={setSelected} onMove={move}
        onAdd={source => { const id = crypto.randomUUID(); change({ ...definition, blocks: [...definition.blocks, { id, source, title: sources[source].label }] }); setSelected(id); }}
        onRemove={id => { change({ ...definition, blocks: definition.blocks.filter(item => item.id !== id) }); if (selected === id) setSelected(''); }}
        onDiscard={() => { const next = saved ?? defaultView(); change(next); setSelected(next.blocks[0].id); }}
        onReset={() => { const next = defaultView(); change(next); setSelected(next.blocks[0].id); }} />
      <section className="min-w-0 space-y-4" aria-label="Profile view preview">
        <div className="flex flex-wrap items-center justify-between gap-3"><div><h2 className="text-sm font-semibold text-gray-800">Live preview</h2><p className="mt-1 text-xs text-gray-500">{profile ? `Showing ${profile.name || profile.refKey}` : 'Layout preview · No sample data'}</p></div><label className="max-w-full text-xs text-gray-500">Preview entity<select className={`${inputClass} max-w-full sm:w-56`} value={refKey} onChange={event => setRefKey(event.target.value)}><option value="">Layout only</option>{refKey && !entities.some(entity => entity.refKey === refKey) && <option value={refKey}>{refKey}</option>}{entities.map(entity => <option key={entity.id} value={entity.refKey}>{entity.name || entity.refKey}</option>)}</select></label></div>
        <details className="text-xs text-gray-500"><summary className="cursor-pointer">Preview another entity</summary><form className="mt-2 flex gap-2" onSubmit={event => { event.preventDefault(); const value = new FormData(event.currentTarget).get('ref'); if (typeof value === 'string') setRefKey(value.trim()); }}><input name="ref" aria-label="Entity reference to preview" placeholder="Enter an entity reference" maxLength={500} required className={inputClass} /><button className={buttonClass}>Load</button></form></details>
        {previewError && <p role="alert" className="rounded-lg bg-amber-50 p-3 text-sm text-amber-900">{previewError}</p>}
        <div className="min-w-0 rounded-2xl border border-gray-200 bg-gray-50/70 p-4 sm:p-6">
          {previewLoading ? <p role="status" className="flex items-center gap-2 py-12 text-sm text-gray-500"><Loader2 className="h-4 w-4 animate-spin" />Loading entity data…</p> : <ProfileViewRenderer definition={definition} profile={profile} type={type} onRangeChange={range => change({ ...definition, range })} />}
          {!definition.blocks.length && <p className="py-16 text-center text-sm text-gray-500">Add a block to start your view.</p>}
        </div>
        <details className="rounded-xl border border-gray-200 bg-white p-4"><summary className="cursor-pointer text-xs font-medium text-gray-500">View definition & data requests</summary><p className="mt-3 text-xs text-gray-500">The current entity is bound when a profile opens. Only the definition is saved; entity data stays live.</p><pre className="mt-3 max-h-80 overflow-auto rounded-lg bg-gray-50 p-3 text-[11px] text-gray-600">{JSON.stringify({ definition, requests: compileProfileRequests(definition) }, null, 2)}</pre></details>
      </section>
    </div>
    </div></div>
  </div></div>;
}
