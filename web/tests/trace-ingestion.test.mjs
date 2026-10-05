import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';

const output = await build({ stdin: { contents: "export {parseTraceFilters, formatTraceFilters} from './app/lib/trace-ingestion'; export {api} from './app/lib/api';", resolveDir: fileURLToPath(new URL('..', import.meta.url)) }, bundle: true, write: false, format: 'esm', platform: 'node' });
const { parseTraceFilters, formatTraceFilters, api } = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);

test('one editor uses original span names for both keep and drop sections', () => {
  const rules = { filters: ['*'], exclude: ['POST /graphql*', 'internal.*'] };
  assert.deepEqual(parseTraceFilters('[keep spans]\n*\n\n[drop spans]\nPOST /graphql*\ninternal.*'), rules);
  assert.deepEqual(parseTraceFilters(formatTraceFilters({ ...rules, mode: 'include' })), rules);
  assert.deepEqual(parseTraceFilters('request*'), { filters: ['request*'], exclude: [] });
  assert.deepEqual(parseTraceFilters('[keep spans]\n*\n[drop spans]\nhealth\n[drop spans]\nhealth'), { filters: ['*'], exclude: ['health'] });
  assert.deepEqual(parseTraceFilters('[keep spans]\n\n[drop spans]'), { filters: [], exclude: [] });
});

test('URL sections and invalid patterns are rejected before save and preview', () => {
  for (const text of ['[keep urls]\n/health', '[drop urls]\n/health', '[keep spans', '[drop spans]\na*b', '[drop spans]\n/a**', '[keep spans]\n' + 'a'.repeat(257)]) {
    assert.throws(() => parseTraceFilters(text), /Line/);
  }
  assert.throws(() => parseTraceFilters('[drop spans]\n' + Array.from({length: 101}, (_,i) => `span.${i}`).join('\n')), /100/);
});

test('legacy exclusions retain their patterns in the sectioned editor', () => {
  assert.deepEqual(parseTraceFilters(formatTraceFilters({ filters: ['health*'], mode: 'exclude_legacy' })), {
    filters: ['*'], exclude: ['health*'],
  });
});

test('save and preview send the same span-name rules with browser authentication', async () => {
  const original = { fetch: globalThis.fetch, window: globalThis.window, document: globalThis.document, localStorage: globalThis.localStorage };
  const calls = [];
  globalThis.window = { location: { origin: 'https://engine.example' } };
  globalThis.document = { cookie: 'threadify_csrf_dev=csrf' };
  globalThis.localStorage = { removeItem() {} };
  globalThis.fetch = async (url, options) => { calls.push({ url, options }); return Response.json({}); };
  try {
    const rules = parseTraceFilters('[keep spans]\n*\n[drop spans]\nregex:(?i)^POST /graphql');
    const names = ['POST /graphql', 'checkout'];
    await api.saveIngestionRules(rules.filters, 'revision', rules.exclude);
    await api.previewIngestionRules(rules.filters, names, rules.exclude);
    assert.deepEqual(JSON.parse(calls[0].options.body), { filters: ['*'], revision: 'revision', exclude: ['regex:(?i)^POST /graphql'] });
    assert.deepEqual(JSON.parse(calls[1].options.body), { filters: ['*'], span_names: names, exclude: ['regex:(?i)^POST /graphql'] });
    for (const { options } of calls) {
      assert.equal(options.credentials, 'include');
      assert.equal(options.headers['X-Threadify-CSRF'], 'csrf');
    }
  } finally { Object.assign(globalThis, original); }
});

test('regex patterns round-trip unchanged in either section', () => {
  const text = '[keep spans]\nregex:(?i)^(GET|POST) /.*$\ncheckout*\n\n[drop spans]\nregex:(?i)health|heartbeat\nregex:^internal\\.[0-9]+$';
  const rules = { filters: ['regex:(?i)^(GET|POST) /.*$', 'checkout*'], exclude: ['regex:(?i)health|heartbeat', 'regex:^internal\\.[0-9]+$'] };
  assert.deepEqual(parseTraceFilters(text), rules);
  assert.deepEqual(parseTraceFilters(formatTraceFilters({...rules,mode:'include'})),rules);
  assert.throws(() => parseTraceFilters('[drop spans]\nregex:'), /Line 2: regex expression is empty/);
  // Engine validates Go's regex dialect; JS must not reject Go's inline flags.
  assert.deepEqual(parseTraceFilters('[drop spans]\nregex:(?i)health'), {filters:[],exclude:['regex:(?i)health']});
});
