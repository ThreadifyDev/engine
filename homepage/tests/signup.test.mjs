import assert from 'node:assert/strict';
import test from 'node:test';
import { createRequestHandler } from '@remix-run/node';
import * as build from '../build/server/index.js';

const handle = createRequestHandler(build, 'production');
const signup = (fields = {}) => handle(new Request('https://threadify.dev/api/signup', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ account_name: 'Acme', email: ' OWNER@EXAMPLE.COM ', recaptcha_token: 'captcha', ...fields }),
}));

test('Registry signup verifies email and issues only the selected licenses', async t => {
  process.env.NODE_ENV = 'production';
  process.env.BACKEND_URL = 'https://registry.example.test/';
  process.env.FUSED_SIGNUP_TOKEN = 'server-token';
  process.env.RECAPTCHA_SECRET_KEY = 'captcha-secret';
  for (const scenario of [
    { name: 'send verification', fields: {}, status: 202, result: { verification_required: true, email: 'owner@example.com' } },
    { name: 'Threadify license', fields: { verification_code: '012345' }, status: 201, result: { account_id: 'account-1', api_key: 'license', products: ['threadify'] } },
    { name: 'invalid code', fields: { verification_code: 'bad' }, status: 403, result: { error: 'Invalid verification code' } },
    { name: 'throttled resend', fields: {}, status: 429, result: { error: 'Try again later' } },
    { name: 'incomplete Registry response', fields: {}, status: 200, result: {} },
  ]) {
    await t.test(scenario.name, async t => {
      const calls = [];
      t.mock.method(globalThis, 'fetch', async (url, init) => {
        calls.push(url);
        if (calls.length === 1) {
          assert.equal(url, 'https://www.google.com/recaptcha/api/siteverify');
          return Response.json({ success: true, score: 0.9, action: 'signup' });
        }
        assert.equal(url, 'https://registry.example.test/internal/signup');
        assert.equal(new Headers(init.headers).get('Authorization'), 'Bearer server-token');
        assert.match(new Headers(init.headers).get('Idempotency-Key'), /^homepage-/);
        const body = JSON.parse(init.body);
        assert.equal(body.email, 'owner@example.com');
        assert.equal(body.plan, 'dev');
        assert.deepEqual(body.products, scenario.fields.products || ['threadify']);
        assert.equal(body.hosted_engine, undefined);
        assert.equal(body.verification_code, scenario.fields.verification_code || '');
        return Response.json(scenario.result, { status: scenario.status });
      });
      const response = await signup({ ...scenario.fields, plan: 'enterprise' });
      assert.equal(response.status, scenario.status === 200 ? 502 : scenario.status);
      assert.equal(response.headers.get('Cache-Control'), 'no-store');
      const body = await response.json();
      if (scenario.status === 202) assert.deepEqual(body, scenario.result);
      if (scenario.status === 201) assert.equal(body.api_key, scenario.result.api_key);
      assert.equal(calls.length, 2);
    });
  }

  await t.test('invalid selections and oversized input never reach Registry', async t => {
    t.mock.method(globalThis, 'fetch', () => { assert.fail('Invalid signup reached an API'); });
    for (const fields of [{ products: ['fused'] }, { products: ['threadify', 'fused'] }, { hosted_engine: {} }, { products: [] }, { products: ['unknown'] }, { products: ['threadify', 'threadify'] }, { products: 'threadify' }, { deployment: 'hosted' }, { email: 'bad' }, { account_name: 'a'.repeat(17000) }]) {
      assert.equal((await signup(fields)).status, 400);
    }
  });
  for (const captcha of [{ success: false }, { success: true, score: 0.1, action: 'signup' }, { success: true, score: 0.9, action: 'login' }]) {
    await t.test(`reject captcha ${JSON.stringify(captcha)}`, async t => {
      let calls = 0;
      t.mock.method(globalThis, 'fetch', () => { calls++; return Response.json(captcha); });
      assert.equal((await signup()).status, 400);
      assert.equal(calls, 1);
    });
  }
  await t.test('production requires CAPTCHA and scoped signup credentials', async t => {
    t.mock.method(globalThis, 'fetch', () => { assert.fail('Unconfigured signup reached Registry'); });
    delete process.env.RECAPTCHA_SECRET_KEY;
    assert.equal((await signup()).status, 400);
    delete process.env.FUSED_SIGNUP_TOKEN;
    assert.equal((await signup()).status, 503);
  });
  assert.equal((await handle(new Request('https://threadify.dev/api/signup'))).status, 405);
});
