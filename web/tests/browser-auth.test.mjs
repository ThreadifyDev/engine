import assert from 'node:assert/strict';
import test from 'node:test';
import { build as bundle } from 'esbuild';
import { fileURLToPath } from 'node:url';
import { readFile } from 'node:fs/promises';

test('production dashboard is a static shell without server configuration', async () => {
  const html = await readFile(new URL('../build/client/index.html', import.meta.url), 'utf8');
  assert.match(html, /Loading Threadify/);
  assert.match(html, /assets\/[^" ]+\.js/);
  assert.doesNotMatch(html, /window\.__ENV__|localhost:3001|localhost:8081|Service-delivery Intelligence/);
});

const output = await bundle({
  stdin: { contents: "export { api } from './app/lib/api';", resolveDir: fileURLToPath(new URL('..', import.meta.url)) },
  bundle: true, write: false, format: 'esm', platform: 'node',
});
const { api } = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);

test('key exchange uses cookies, purges the old bearer and caches only profile data', async () => {
  const saved = { fetch: globalThis.fetch, window: globalThis.window, document: globalThis.document, localStorage: globalThis.localStorage };
  const storage = new Map([['auth_token', 'legacy-token']]);
  const calls = [];
  let redirect;
  globalThis.document = { cookie: 'threadify_csrf_dev=session-csrf' };
  globalThis.window = { __ENV__: { API_URL: 'http://127.0.0.1:3003', ENGINE_URL: 'http://127.0.0.1:8083' }, location: { assign(value) { redirect = value; } } };
  globalThis.localStorage = { getItem: key => storage.get(key), setItem: (key,value) => storage.set(key,value), removeItem: key => storage.delete(key) };
  globalThis.fetch = async (url, init) => {
    calls.push({url, ...init});
    return Response.json(url.endsWith('/session') ? {authenticated: true, user: {id:'owner',company_id:'company'}} : {status:'authenticated'});
  };
  try {
    await api.exchangeAPIKey('private-api-key');
    assert.deepEqual(calls.map(c => c.url), ['http://127.0.0.1:8083/auth/api-key/exchange','http://127.0.0.1:8083/auth/session']);
    assert.equal(storage.has('auth_token'), false);
    assert.deepEqual([...storage.keys()], ['user']);
    assert.equal(JSON.stringify([...storage]).includes('private-api-key'), false);
    for (const c of calls) { assert.equal(c.credentials,'include'); assert.equal(c.headers.Authorization,undefined); }
    await api.logout();
    assert.equal(calls.at(-1).headers['X-Threadify-CSRF'],'session-csrf');
    assert.equal(storage.size,0);
    assert.equal(redirect,'/login');
  } finally { Object.assign(globalThis,saved); }
});
