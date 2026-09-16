import assert from 'node:assert/strict';
import test from 'node:test';
import { createRequestHandler } from '@remix-run/node';
import * as build from '../build/server/index.js';

const handle = createRequestHandler(build, 'production');

test('public pages render independently of the Engine and old web API', async t => {
  t.mock.method(globalThis, 'fetch', () => { assert.fail('Rendering contacted an API'); });
  process.env.FUSED_SIGNUP_TOKEN = 'server-only-signup-secret';
  process.env.RECAPTCHA_SECRET_KEY = 'server-only-captcha-secret';
  process.env.RECAPTCHA_SITE_KEY = 'public-site-key';
  for (const path of ['/', '/pricing', '/signup']) {
    const response = await handle(new Request(`https://threadify.dev${path}`));
    assert.equal(response.status, 200, path);
    const html = await response.text();
    assert.doesNotMatch(html, /server-only-|window\.__ENV__|ENGINE_URL|API_URL/);
    if (path === '/') assert.match(html, /Your delivery process/);
    if (path === '/signup') {
      assert.match(html, /Send verification code/);
      assert.doesNotMatch(html, /Products on your license|name="products"|name="deployment"|Fused|name="password"/);
      assert.equal(response.headers.get('Cache-Control'), 'no-store');
    }
  }
  assert.ok(!Object.values(build.routes).some(route => /routes\/(u\.|api\.harnest|cli-login)/.test(route.id)));
  const sitemap = await handle(new Request('https://threadify.dev/sitemap.xml'));
  const xml = await sitemap.text();
  assert.match(xml, /https:\/\/threadify.dev\/pricing/);
  assert.doesNotMatch(xml, /\/login|\/u\/|\/signup/);
});

test('signup disables submission when its server credentials are missing', async () => {
  delete process.env.FUSED_SIGNUP_TOKEN;
  const response = await handle(new Request('https://threadify.dev/signup'));
  const html = await response.text();
  assert.match(html, /Signup is temporarily unavailable/);
  assert.match(html, /<button[^>]*class="access-submit"[^>]*disabled=""/);
});
