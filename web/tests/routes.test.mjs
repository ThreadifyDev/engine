import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { matchRoutes } from 'react-router';
import { fileURLToPath } from 'node:url';

const output = await build({
  stdin: {contents: "export {pages, redirects} from './app/routes'; export {supportsAgent} from './app/components/agent/agent-preview';", resolveDir: fileURLToPath(new URL('..', import.meta.url))},
  bundle: true, write: false, format: 'esm', platform: 'node',
  plugins: [{name: 'defer-page-components', setup(b) {
    b.onResolve({filter: /^\.\/routes\//}, args => ({path: args.path, external: true}));
  }}],
});
const {pages, redirects, supportsAgent} = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);

test('agent is available on every signed-in page and hidden on public routes', () => {
  for (const page of pages.filter(page => page.path.startsWith('/u/'))) {
    const path = page.path.replace(/:[^/]+/g, 'example');
    assert.equal(supportsAgent(path), true, path);
  }
  for (const path of ['/login', '/cli-login', '/pricing', '/u/unknown']) {
    assert.equal(supportsAgent(path), false, path);
  }
});

test('deep links preserve thread, contract version and decoded entity identifiers', () => {
  for (const [url, expected, params] of [
    ['/u/threads/thread-123?tab=history', '/u/threads/:id', {id: 'thread-123'}],
    ['/u/contracts/order-flow/versions/2', '/u/contracts/:id/versions/:version', {id: 'order-flow', version: '2'}],
    ['/u/profiles/E2E%20Customers/customer_123?tab=delivery-health', '/u/profiles/:type/:refKey', {type: 'E2E Customers', refKey: 'customer_123'}],
    ['/u/profile-views/Customer%20Accounts?ref=ACME%2F42', '/u/profile-views/:type', {type: 'Customer Accounts'}],
    ['/u/settings?tab=engine', '/u/settings', {}],
    ['/cli-login?request_id=cli-123', '/cli-login', {}],
  ]) {
    const match = matchRoutes(pages, url)?.at(-1);
    assert.equal(match?.route.path, expected);
    assert.deepEqual(match.params, params);
  }
  assert.equal(matchRoutes(pages, '/u/unknown'), null);
});

test('old authentication and assistant bookmarks redirect locally', () => {
  assert.equal(redirects['/'], '/u/dashboard');
  assert.equal(redirects['/u/assistant'], '/u/dashboard');
  for (const path of ['/signup', '/auth/forgot-password', '/auth/reset-password', '/auth/verify-otp']) {
    assert.equal(redirects[path], '/login');
  }
});
