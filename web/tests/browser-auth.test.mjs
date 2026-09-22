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

const loginOutput = await bundle({
  entryPoints: [fileURLToPath(new URL('../app/lib/managed-login.ts', import.meta.url))],
  bundle: true, write: false, format: 'esm', platform: 'node',
});
const { waitForManagedLogin, MANAGED_SIGN_IN_ERROR, managedLoginURL } = await import(`data:text/javascript;base64,${Buffer.from(loginOutput.outputFiles[0].text).toString('base64')}`);
const transaction = () => ({ transaction_id: 'transaction', poll_token: 'poll-capability', expires_at: new Date(Date.now()+60000).toISOString() });

test('account retry asks Registry for fresh authentication and preserves the transaction URL', () => {
  const url = 'https://auth.example.test/t/transaction?existing=value';
  assert.equal(managedLoginURL(url, false), url);
  const retry = new URL(managedLoginURL(url, true));
  assert.equal(retry.origin, 'https://auth.example.test');
  assert.equal(retry.pathname, '/t/transaction');
  assert.equal(retry.searchParams.get('existing'), 'value');
  assert.equal(retry.searchParams.get('reauthenticate'), 'true');
});

test('Registry tab closing does not cancel a pending Engine login', async () => {
  const popup = { closed: false };
  let calls = 0;
  await waitForManagedLogin(transaction(), new AbortController().signal, async (id, token) => {
    assert.equal(id, 'transaction'); assert.equal(token, 'poll-capability');
    if (++calls === 1) { popup.closed = true; return { status: 'pending' }; }
    assert.equal(popup.closed, true);
    return { status: 'authenticated' };
  }, async () => {});
  assert.equal(calls, 2);
});

test('access denial stops polling and public retry instructions do not disclose account state', async () => {
  let calls = 0;
  await assert.rejects(waitForManagedLogin(transaction(), new AbortController().signal, async () => {
    calls++; throw new Error('managed_login_denied');
  }, async () => {}), /managed_login_denied/);
  assert.equal(calls, 1);
  assert.match(MANAGED_SIGN_IN_ERROR, /Revalidate your sign-in/);
  assert.doesNotMatch(MANAGED_SIGN_IN_ERROR, /account|owner|invit|member|identity|license|provider/i);
});

test('login tolerates temporary exchange failures, but respects cancellation and expiry', async () => {
  let calls = 0;
  await waitForManagedLogin(transaction(), new AbortController().signal, async () => {
    if (++calls < 3) throw new Error('managed_login_unavailable');
    return { status: 'authenticated' };
  }, async () => {});
  assert.equal(calls, 3);
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(waitForManagedLogin(transaction(), controller.signal, async () => assert.fail('must not poll')), { name: 'AbortError' });
  await assert.rejects(waitForManagedLogin({ ...transaction(), expires_at: new Date(0).toISOString() }, new AbortController().signal, async () => assert.fail('must not poll')), /managed_login_expired/);
});

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
