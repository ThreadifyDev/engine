import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';

// Bundle the real browser clients; importing them must also be safe during SSR.
const output = await build({
  stdin: {
    contents: "export { api } from './app/lib/api'; export { graphqlClient as graphql } from './app/lib/graphql';",
    resolveDir: fileURLToPath(new URL('..', import.meta.url)),
  },
  bundle: true, write: false, format: 'esm', platform: 'node',
});
const { api, graphql } = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);

// Exercise the outgoing requests, including YAML and cookie-session transport.
test('engine requests bypass the Web API while account routes stay external', async () => {
  const originalFetch = globalThis.fetch;
  const originalWindow = globalThis.window;
  const originalStorage = globalThis.localStorage;
  const originalDocument = globalThis.document;
  const calls = [];
  globalThis.window = { __ENV__: { API_URL: 'http://localhost:3001/', ENGINE_URL: 'http://127.0.0.1:8083/' } };
  globalThis.localStorage = { getItem: () => 'legacy-token', removeItem() {} };
  globalThis.document = { cookie: 'threadify_csrf_dev=csrf-proof' };
  globalThis.fetch = async (url, options) => {
    calls.push({ url, ...options });
    return Response.json(url.endsWith('/graphql') ? { data: { thread: { id: 'thread-1' } } } : { ok: true });
  };
  try {
    await graphql.getThread('thread-1');
    await api.getAllContracts({ search: 'support' });
    await api.createContract({ name: 'support', yaml: 'contract_name: support' });
    await api.previewContract({ yaml: 'contract_name: support' });
    await api.getUserProfile();
    await api.listServiceAccounts();
    await api.listEngineUsers();
    await api.inviteEngineUser({ email: 'member@example.test', full_name: 'Member', role: 'member' });
    await api.updateEngineUser('user-1', { status: 'suspended' });
    await api.getEngineSettings();
    await api.saveEnginePublicURL('https://threadify.example.test');
    await api.resetEnginePublicURL();
    assert.deepEqual(calls.map(call => call.url), [
      'http://127.0.0.1:8083/graphql',
      'http://127.0.0.1:8083/v1/contracts?search=support',
      'http://127.0.0.1:8083/v1/contracts',
      'http://127.0.0.1:8083/v1/contracts/preview',
      'http://localhost:3001/api/user/profile',
      'http://localhost:3001/api/service-accounts',
      'http://127.0.0.1:8083/v1/users',
      'http://127.0.0.1:8083/v1/users',
      'http://127.0.0.1:8083/v1/users/user-1',
      'http://127.0.0.1:8083/v1/engine/settings',
      'http://127.0.0.1:8083/v1/engine/settings',
      'http://127.0.0.1:8083/v1/engine/settings',
    ]);
    for (const call of calls) {
      assert.equal(call.headers.Authorization, undefined);
      assert.equal(call.headers['X-Threadify-CSRF'], 'csrf-proof');
      assert.equal(call.credentials, 'include');
    }
    assert.equal(calls[2].body, 'contract_name: support');
    assert.equal(calls[2].headers['Content-Type'], 'text/plain');
    assert.equal(calls[7].method, 'POST');
    assert.deepEqual(JSON.parse(calls[7].body), { email: 'member@example.test', full_name: 'Member', role: 'member' });
    assert.equal(calls[8].method, 'PATCH');
    assert.deepEqual(JSON.parse(calls[8].body), { status: 'suspended' });
    assert.equal(calls[10].method, 'PUT');
    assert.deepEqual(JSON.parse(calls[10].body), { public_url: 'https://threadify.example.test' });
    assert.equal(calls[11].method, 'DELETE');
    // Changing runtime configuration must affect existing client instances.
    window.__ENV__.ENGINE_URL = 'https://engine.example.test';
    await api.getContract('contract-1');
    assert.equal(calls.at(-1).url, 'https://engine.example.test/v1/contracts/contract-1');
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.window = originalWindow;
    globalThis.localStorage = originalStorage;
    globalThis.document = originalDocument;
  }
});
