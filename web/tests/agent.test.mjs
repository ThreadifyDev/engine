import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';

const output = await build({ stdin: { contents: "export { harnest } from './app/lib/harnest'; export * from './app/components/agent/client-tools'; export { agentInput, connectedPageContext, pageContextForAgent } from './app/components/agent/agent-input'; export { updateToolActivity } from './app/components/agent/tool-activity';", resolveDir: fileURLToPath(new URL('..', import.meta.url)) }, bundle: true, write: false, format: 'esm', platform: 'node' });
const { harnest, agentInput, connectedPageContext, pageContextForAgent, executeFrontendTool, navigationPath, updateToolActivity } = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);
const call = (name, args = {}, id = 'client-1') => ({ id, callId: 'call-1', name, arguments: args });

test('navigation only targets supported local pages and encodes entity identifiers', () => {
  for (const page of ['https://evil.test', '//evil.test', 'constructor', '__proto__']) assert.throws(() => navigationPath({ page }));
  assert.throws(() => navigationPath({ page: 'contract', id: '../settings' }));
  assert.equal(navigationPath({ page: 'entity_profile', profile_type: 'Customer', ref_key: 'ACME/42' }), '/u/profiles/Customer/ACME%2F42');
});

test('each page-enabled turn carries the current route, not an earlier page', () => {
  const context = {
    title: 'Settings', path: '/u/settings?tab=engine', detail: 'Engine', resource: { tab: 'engine' },
  };
  const input = agentInput('can you read this page', context);
  assert.match(input, /"title":"Settings"/);
  assert.match(input, /"path":"\/u\/settings\?tab=engine"/);
  assert.match(input, /"renderedPageContentAvailable":false/);
  assert.doesNotMatch(input, /\/u\/threads/);
  assert.equal(agentInput('can you read this page', undefined), 'can you read this page');
  const settingsContext = pageContextForAgent(context, true, { source: 'Feature: old draft', revision: 4, open: true });
  assert.equal(settingsContext.pageContentAvailable, false);
  assert.equal(settingsContext.contractDraft, undefined);
  assert.equal(pageContextForAgent(context, false, { source: '', revision: 0, open: false }).pageContextEnabled, false);
});

test('connected Engine page context reads saved settings and rejects a route change', async () => {
  const context = { title: 'Settings', path: '/u/settings?tab=engine', detail: 'Engine', resource: { tab: 'engine' } };
  const draft = { source: '', revision: 0, open: false };
  let reads = 0;
  const read = async () => { reads++; return { public_url: 'https://engine.example.test', config_public_url: '', source: 'ui' }; };
  const result = await connectedPageContext(context, true, draft, undefined, read, () => true);
  assert.deepEqual(result.connectedPageData, {
    kind: 'engine_settings', read_from: 'engine_settings_api', public_url: 'https://engine.example.test',
    config_public_url: '', source: 'ui', unsaved_form_values_included: false,
  });
  assert.equal(result.contractDraft, undefined);
  assert.equal(reads, 1);
  const toolResult = await executeFrontendTool(call('get_page_context'), {
    getContext: () => connectedPageContext(context, true, draft, undefined, read, () => true),
  }, new AbortController().signal);
  assert.equal(toolResult.context.connectedPageData.public_url, 'https://engine.example.test');
  assert.equal((await connectedPageContext(context, false, draft, undefined, read, () => true)).pageContextEnabled, false);
  assert.equal(reads, 2);
  const unavailable = await connectedPageContext(context, true, draft, undefined, async () => { throw new Error('network'); }, () => true);
  assert.deepEqual(unavailable.connectedPageData, { kind: 'engine_settings', status: 'unavailable' });
  await assert.rejects(connectedPageContext(context, true, draft, undefined, read, () => false), /page changed/);
});

test('draft writes fail closed on stale revision and do not publish', async () => {
  let draft = { source: 'User changes', revision: 2, open: true };
  const paths = [];
  const host = { getDraft: () => draft, writeDraft: source => (draft = { source, revision: 3, open: true }), navigate: async path => paths.push(path) };
  const signal = new AbortController().signal;
  assert.equal((await executeFrontendTool(call('open_contract_draft', { expected_revision: 1, source: 'Feature: refund' }), host, signal)).error, 'DRAFT_CONFLICT');
  assert.equal(draft.source, 'User changes');
  const result = await executeFrontendTool(call('open_contract_draft', { expected_revision: 2, source: 'Feature: refund' }), host, signal);
  assert.equal(result.published, false);
  assert.deepEqual(paths, ['/u/contracts']);
  assert.equal(draft.source, 'Feature: refund');
});

test('preview rejects results if the user edits while compilation is in flight', async () => {
  let draft = { source: 'Feature: refund', revision: 1 };
  const result = await executeFrontendTool(call('preview_contract_draft', { expected_revision: 1 }), {
    getDraft: () => draft, previewDraft: async () => { draft = { ...draft, revision: 2 }; return { valid: true }; },
  }, new AbortController().signal);
  assert.equal(result.error, 'DRAFT_CONFLICT');
});

test('Engine settings read returns only saved public addresses and reports failures', async () => {
  const signal = new AbortController().signal;
  let reads = 0;
  const host = { getEngineSettings: async () => { reads++; return {
    public_url: 'https://engine.example.test', config_public_url: 'https://default.example.test',
    source: 'ui', can_manage: true, endpoints: { mcp: 'https://engine.example.test/sse' },
  }; } };
  assert.deepEqual(await executeFrontendTool(call('get_engine_settings'), host, signal), {
    ok: true, read_from: 'engine_settings_api', public_url: 'https://engine.example.test',
    config_public_url: 'https://default.example.test', source: 'ui', unsaved_form_values_included: false,
  });
  assert.equal(reads, 1);
  assert.equal((await executeFrontendTool(call('get_engine_settings'), {
    getEngineSettings: async () => { throw new Error('Engine settings unavailable'); },
  }, signal)).ok, false);
});

test('unknown tools and cancelled requests cannot run UI actions', async () => {
  const abort = new AbortController(); abort.abort();
  await assert.rejects(executeFrontendTool(call('navigate_ui', { page: 'contracts' }), {}, abort.signal), { name: 'AbortError' });
  assert.equal((await executeFrontendTool(call('delete_contract'), {}, new AbortController().signal)).ok, false);
});

test('SSE resumes multiple client tools through JSON without starting another turn', async () => {
  const saved = { fetch: globalThis.fetch, document: globalThis.document };
  globalThis.document = { cookie: 'threadify_csrf_dev=synthetic-csrf' };
  const calls = []; const executed = []; const events = [];
  const first = call('get_page_context');
  const second = call('navigate_ui', { page: 'contracts' }, 'client-2');
  const stream = [
    { type: 'response.text.delta', delta: 'Looking at your page.' },
    { type: 'client_tool.requested', clientTool: first },
    { type: 'response.completed', status: 'requires_action', requiredAction: { type: 'client_tool', ...first } },
  ].map(event => `data: ${JSON.stringify(event)}\r\n\r\n`).join('');
  globalThis.fetch = async (url, options) => {
    calls.push({ url, options });
    if (calls.length === 1) {
      const bytes = new TextEncoder().encode(stream);
      return new Response(new ReadableStream({ start(controller) { controller.enqueue(bytes.slice(0, 23)); controller.enqueue(bytes.slice(23)); controller.close(); } }));
    }
    if (calls.length === 2) return Response.json({ status: 'requires_action', requiredAction: { type: 'client_tool', ...second } });
    return Response.json({ status: 'completed', outputText: 'Opened contracts.' });
  };
  try {
    await harnest.streamResponse('Open contracts', 'session-1', event => events.push(event), undefined, async tool => { executed.push(tool.name); return { ok: true }; });
    assert.deepEqual(executed, ['get_page_context', 'navigate_ui']);
    assert.deepEqual(calls.map(item => item.url), ['/api/harnest/responses', '/api/harnest/client-tools/client-1', '/api/harnest/client-tools/client-2']);
    assert.equal(events.at(-1).outputText, 'Opened contracts.');
    for (const { options } of calls) { assert.equal(options.credentials, 'include'); assert.equal(options.headers.Authorization, undefined); assert.equal(options.headers['X-Threadify-CSRF'], 'synthetic-csrf'); }
    assert.deepEqual(JSON.parse(calls[1].options.body), { output: { ok: true } });
  } finally { Object.assign(globalThis, saved); }
});

test('truncated SSE is an error, not a successful empty reply', async () => {
  const saved = { fetch: globalThis.fetch, document: globalThis.document };
  globalThis.document = { cookie: 'threadify_csrf_dev=synthetic-csrf' };
  globalThis.fetch = async () => new Response('data: {"type":"response.text.delta","delta":"partial"}\n\n');
  try { await assert.rejects(harnest.streamResponse('hello', 's', () => {}), /ended before/); }
  finally { Object.assign(globalThis, saved); }
});

test('failed completion and malformed pending actions are reported as failures', async () => {
  const saved = { fetch: globalThis.fetch, document: globalThis.document };
  globalThis.document = { cookie: 'threadify_csrf_dev=synthetic-csrf' };
  try {
    for (const result of [{ status: 'failed' }, { status: 'requires_action', requiredAction: { type: 'client_tool' } }]) {
      globalThis.fetch = async () => new Response(`data: ${JSON.stringify({ type: 'response.completed', ...result })}\n\n`);
      await assert.rejects(harnest.streamResponse('hello', 's', () => {}), /did not complete|incomplete frontend action/);
    }
  } finally { Object.assign(globalThis, saved); }
});


test('client handoff adopts its streamed activity and preserves completed repeated calls', () => {
  let tools = updateToolActivity([], { id: 'model-call', name: 'get_page_context', status: 'running' });
  tools = updateToolActivity(tools, { id: 'client:one', name: 'get_page_context', status: 'running' }, true);
  tools = updateToolActivity(tools, { id: 'client:one', name: 'get_page_context', status: 'completed' });
  assert.equal(tools.length, 1);
  tools = updateToolActivity(tools, { id: 'client:two', name: 'get_page_context', status: 'running' }, true);
  tools = tools.map(tool => tool.status === 'running' ? { ...tool, status: 'failed' } : tool);
  assert.deepEqual(tools.map(tool => [tool.id, tool.status]), [['client:one', 'completed'], ['client:two', 'failed']]);
});


test('unavailable connection errors use Threadify terminology', async () => {
  const saved = { fetch: globalThis.fetch, document: globalThis.document };
  globalThis.document = { cookie: 'threadify_csrf_dev=synthetic-csrf' };
  globalThis.fetch = async () => new Response('', { status: 503 });
  try { await assert.rejects(harnest.createSession('Test connection'), { message: 'The agent is unavailable. Check the Engine’s agent connection.' }); }
  finally { Object.assign(globalThis, saved); }
});
