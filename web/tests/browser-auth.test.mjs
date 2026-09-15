import assert from 'node:assert/strict';
import test from 'node:test';
import { build as bundle } from 'esbuild';
import { fileURLToPath } from 'node:url';
import { createRequestHandler } from '@remix-run/node';
import * as build from '../build/server/index.js';

const handle = createRequestHandler(build, 'production');

test('login presents Registry sign-in and API-key exchange without passwords', async () => {
  const response = await handle(new Request('http://localhost/login'));
  assert.equal(response.status, 200);
  const html = await response.text();
  assert.match(html, /Continue with email or SSO/);
  assert.match(html, /Sign in with API key/);
  assert.doesNotMatch(html, /name="password"|Forgot password|Supabase/);
});

test('old password and verification pages redirect to Registry login', async () => {
  for (const path of ['/signup', '/auth/forgot-password', '/auth/reset-password', '/auth/verify-otp']) {
    const response = await handle(new Request(`http://localhost${path}`));
    assert.equal(response.status, 302, path);
    assert.equal(response.headers.get('Location'), '/login');
  }
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

test('Harnest proxy rejects requests without a cookie-bound CSRF proof', async () => {
  const saved = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = async (url, init) => {
    calls++;
    assert.match(url, /\/auth\/session\/verify$/);
    assert.equal(init.headers.Cookie,'threadify_session_dev=tfs_invalid');
    return Response.json({error:'csrf_denied'},{status:403});
  };
  try {
    let response = await handle(new Request('http://localhost/api/harnest/sessions'));
    assert.equal(response.status,401); assert.equal(calls,0);
    response = await handle(new Request('http://localhost/api/harnest/sessions', {headers:{Cookie:'threadify_session_dev=tfs_invalid'}}));
    assert.equal(response.status,403); assert.equal(calls,1);
  } finally { globalThis.fetch=saved; }
});
