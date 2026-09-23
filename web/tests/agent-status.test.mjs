import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import ReactMarkdown from 'react-markdown';

const output = await build({
  stdin: {
    contents: `import React from 'react';
      import { renderToStaticMarkup } from 'react-dom/server';
      import { AgentStateContext } from './app/components/agent/agent-context';
      import AgentToggleButton from './app/components/agent/AgentToggleButton';
      import AgentSidebar from './app/components/agent/AgentSidebar';
      export * from './app/components/agent/agent-status';
      export const render = (state, sidebar = false) => renderToStaticMarkup(React.createElement(AgentStateContext.Provider, { value: state }, React.createElement(sidebar ? AgentSidebar : AgentToggleButton, { isCompact: false })));`,
    resolveDir: fileURLToPath(new URL('..', import.meta.url)),
  },
  bundle: true, write: false, format: 'cjs', packages: 'external', platform: 'node', jsx: 'automatic',
});
const compiled = { exports: {} };
const require = createRequire(import.meta.url);
new Function('require', 'module', 'exports', output.outputFiles[0].text)(name => name === 'react-markdown' ? { __esModule: true, default: ReactMarkdown } : require(name), compiled, compiled.exports);
const { render, disabledAgent, parseAgentStatus, fetchAgentStatus, unavailableAgent, agentPollInterval } = compiled.exports;
const state = status => ({
  isEnabled: status.enabled, agentStatus: status, isSupported: true, isOpen: false,
  context: { title: 'Contracts' }, messages: [], composer: 'Help me write a contract', includeContext: true,
});
const configured = status => ({ enabled: true, status, mode: 'local' });

test('agent launcher stays hidden until explicitly enabled, including supported pages', () => {
  assert.equal(render(state(disabledAgent)), '');
  for (const status of ['ready', 'starting', 'unavailable']) {
    const enabled = state(configured(status));
    assert.match(render(enabled), /aria-label="Ask agent"/);
    assert.equal(render({ ...enabled, isSupported: false }), '');
    assert.equal(render({ ...enabled, isOpen: true }), '');
  }
});

test('configured unavailable and starting agents keep conversation and offer a connection check without sending', () => {
  for (const status of ['starting', 'unavailable']) {
    const html = render({ ...state(configured(status)), messages: [{ id: 'one', role: 'user', text: 'Keep this draft' }] }, true);
    assert.match(html, /Keep this draft/);
    assert.match(html, /Check connection/);
    assert.match(html, /disabled="" aria-label="Send message"/);
    assert.doesNotMatch(html, /Harnest/);
  }
  const ready = render(state(configured('ready')), true);
  assert.doesNotMatch(ready, /Check connection/);
  assert.doesNotMatch(ready, /disabled="" aria-label="Send message"/);
});

test('invalid status cannot enable AI, and connection loss preserves only an explicitly configured agent', () => {
  assert.deepEqual(parseAgentStatus(disabledAgent), disabledAgent);
  for (const value of [{ enabled: true }, { enabled: false, status: 'ready', mode: 'external' }, { enabled: true, status: 'ready', mode: 'none' }, '<html>']) {
    assert.throws(() => parseAgentStatus(value));
  }
  assert.deepEqual(unavailableAgent(disabledAgent), disabledAgent);
  assert.deepEqual(unavailableAgent(configured('ready')), configured('unavailable'));
  assert.ok(agentPollInterval(configured('starting')) < agentPollInterval(configured('ready')));
});

test('status uses Engine origin and cookie authentication; expired sessions hide AI', async () => {
  const saved = { fetch: globalThis.fetch, window: globalThis.window, document: globalThis.document };
  globalThis.window = { location: { origin: 'https://engine.example' } };
  globalThis.document = { cookie: 'threadify_csrf_dev=csrf' };
  const signal = new AbortController().signal;
  try {
    globalThis.fetch = async (url, options) => {
      assert.equal(url, 'https://engine.example/v1/agent/status');
      assert.equal(options.credentials, 'include');
      assert.equal(options.headers.Authorization, undefined);
      assert.equal(options.cache, 'no-store');
      assert.equal(options.signal, signal);
      return Response.json(configured('ready'));
    };
    assert.deepEqual(await fetchAgentStatus(signal), configured('ready'));
    globalThis.fetch = async () => new Response('', { status: 401 });
    assert.deepEqual(await fetchAgentStatus(signal), disabledAgent);
    globalThis.fetch = async () => new Response('', { status: 503 });
    await assert.rejects(fetchAgentStatus(signal), /Unable to check/);
  } finally { Object.assign(globalThis, saved); }
});
