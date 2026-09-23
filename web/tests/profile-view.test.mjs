import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';

const output = await build({ entryPoints: [fileURLToPath(new URL('../app/components/profiles/view/profile-view.ts', import.meta.url))], bundle: true, write: false, format: 'esm', platform: 'node' });
const { withMetricPresentations, defaultView, validateProfileView, compileProfileRequests, profileViewKey, saveProfileView, loadProfileView, applyProfileViewProposal, readProfileViewProposal } = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);
const proposal = definition => `Here is the view.\n\`\`\`profile-view\n${JSON.stringify(definition)}\n\`\`\``;

test('presentations reference distinct saved metrics without duplicating data definitions', () => {
  const a = { id: 'a', source: 'configuredMetric', title: 'Count', metricId: '11111111-1111-4111-8111-111111111111', display: 'card' };
  const b = { ...a, id: 'b', metricId: '22222222-2222-4222-8222-222222222222', display: 'line' };
  const definition = validateProfileView({ ...defaultView(), blocks: [a, b] });
  assert.equal(compileProfileRequests(definition)[0].parameters.metricId, a.metricId);
  assert.throws(() => validateProfileView({ ...definition, blocks: [a, { ...a, id: 'duplicate' }] }));
  assert.throws(() => validateProfileView({ ...definition, blocks: [{ ...a, display: 'script' }] }));
  assert.throws(() => validateProfileView({ ...definition, blocks: [{ ...a, metricId: 'invented' }] }));
  assert.throws(() => validateProfileView({ ...definition, blocks: [{ ...a, source: 'deliveryHealth' }] }));
});

test('metric proposals change only the data draft and require current draft revision', () => {
  let metrics;
  const text = '```profile-metrics\n[{"name":"Count","custom_definition":{"name":"Count","operation":"COUNT","field":"threads"}}]\n```';
  const designer = { key: 'type', revision: 2, definition: defaultView(), apply: () => assert.fail('must not change presentation'), applyMetrics: value => { metrics = value; } };
  assert.throws(() => applyProfileViewProposal(text, { key: 'type', revision: 1 }, designer), /changed/);
  applyProfileViewProposal(text, { key: 'type', revision: 2 }, designer);
  assert.equal(metrics[0].name, 'Count');
});

test('rejects arbitrary code, queries, unknown sources and invalid layouts', () => {
  const view = defaultView();
  for (const invalid of [
    { ...view, code: '<script>' }, { ...view, schemaVersion: 2 }, { ...view, range: 'forever' },
    { ...view, blocks: [] }, { ...view, blocks: [{ id: 'one', source: '__proto__', title: 'Unsafe' }] },
    { ...view, blocks: [{ ...view.blocks[0], query: 'deleteAll' }] },
    { ...view, blocks: [view.blocks[0], view.blocks[0]] },
    { ...view, blocks: [view.blocks[0], { ...view.blocks[0], id: 'another' }] },
    { ...view, range: ['7d'] }, { ...view, columns: 0 }, { ...view, title: '   ' }, { ...view, description: 'x'.repeat(501) },
  ]) assert.throws(() => validateProfileView(invalid));
  assert.deepEqual(validateProfileView(view), view);
});

test('persists the view and parameterized calls without storing entity data', () => {
  const entries = new Map();
  const storage = { getItem: key => entries.get(key) ?? null, setItem: (key, value) => entries.set(key, value) };
  const key = profileViewKey('company', 'user', 'type');
  const view = { ...defaultView(), blocks: [{ id: 'health', source: 'deliveryHealth', title: 'Health' }, { id: 'metrics', source: 'computedMetrics', title: 'Metrics' }] };
  saveProfileView(storage, key, view);
  assert.deepEqual(loadProfileView(storage, key), view);
  assert.equal(loadProfileView(storage, profileViewKey('other-company', 'user', 'type')), null);
  assert.equal(loadProfileView(storage, profileViewKey('company', 'other-user', 'type')), null);
  assert.equal(loadProfileView(storage, profileViewKey('company', 'user', 'other-type')), null);
  const saved = JSON.parse(entries.get(key));
  assert.deepEqual(saved.requests[0].parameters, { refKey: '$entity.refKey', type: '$profileType.name', range: '30d' });
  assert.deepEqual(saved.requests[1].parameters, { refKey: '$entity.refKey', type: '$profileType.name', range: '30d' });
  assert.deepEqual(Object.keys(saved).sort(), ['definition', 'requests', 'savedAt']);
  assert.throws(() => profileViewKey('', 'user', 'type'));
});

test('malformed or unsupported saved views fail explicitly', () => {
  for (const value of ['invalid-json', '{}', JSON.stringify({ definition: { ...defaultView(), schemaVersion: 10 } })]) {
    assert.throws(() => loadProfileView({ getItem: () => value }, 'key'));
  }
  assert.throws(() => saveProfileView({ setItem: () => { throw new Error('Storage is full'); } }, 'key', defaultView()), /Storage is full/);
});

test('agent proposals cannot overwrite a changed draft or another profile type', () => {
  const definition = defaultView();
  const text = proposal(definition);
  let applied;
  const designer = { key: 'customers', revision: 3, definition, apply: value => { applied = value; } };
  assert.throws(() => applyProfileViewProposal(text, { key: 'customers', revision: 2 }, designer), /changed/);
  assert.throws(() => applyProfileViewProposal(text, { key: 'agents', revision: 3 }, designer), /changed/);
  assert.throws(() => applyProfileViewProposal(text, { key: 'customers', revision: 3 }, null), /changed/);
  assert.equal(applied, undefined);
  applyProfileViewProposal(text, { key: 'customers', revision: 3 }, designer);
  assert.deepEqual(applied, definition);
  assert.equal(readProfileViewProposal('An ordinary response'), null);
  assert.throws(() => readProfileViewProposal(proposal({ ...definition, blocks: [] })));
});

test('date ranges affect only supported operations and calls follow block bindings', () => {
  const requests = compileProfileRequests({ ...defaultView(), range: '90d', blocks: [
    { id: 'summary', source: 'totalDeliveries', title: 'Volume' },
    { id: 'metrics', source: 'computedMetrics', title: 'Configured metrics' },
  ] });
  assert.equal(requests[0].parameters.range, undefined);
  assert.equal(requests[0].field, 'metrics.totalDeliveries');
  assert.equal(requests[1].parameters.range, '90d');
  assert.equal(requests[1].operation, 'getComputedMetrics');
});

test('Overview rejects history in new proposals and migrates early saved views', () => {
  const history = { id: 'history', source: 'history', title: 'Thread history' };
  const definition = { ...defaultView(), blocks: [history, ...defaultView().blocks] };
  assert.throws(() => validateProfileView(definition), /supported Threadify data source/);
  assert.throws(() => readProfileViewProposal(proposal(definition)), /supported Threadify data source/);
  const storage = { getItem: () => JSON.stringify({ definition }) };
  assert.deepEqual(loadProfileView(storage, 'key'), defaultView());
  assert.equal(loadProfileView({ getItem: () => JSON.stringify({ definition: { ...definition, blocks: [history] } }) }, 'key'), null);
});

test('AI chart proposals bind the selected range to authenticated delivery health', () => {
  const definition = { ...defaultView(), range: '90d', blocks: [{ id: 'rates', source: 'deliveryHealthChart', title: 'Delivery rates' }] };
  const parsed = readProfileViewProposal(proposal(definition));
  assert.deepEqual(compileProfileRequests(parsed), [{ blockId: 'rates', operation: 'getDeliveryHealthMetrics', field: 'deliveryHealth', parameters: { refKey: '$entity.refKey', type: '$profileType.name', range: '90d' } }]);
});

test('daily graph preserves zero failures and rejects missing or malformed daily data', async () => {
  const chart = await build({ entryPoints: [fileURLToPath(new URL('../app/components/profiles/view/DeliveryTrendChart.tsx', import.meta.url))], bundle: true, write: false, format: 'esm', platform: 'node' });
  const { parseDeliveryTrend } = await import(`data:text/javascript;base64,${Buffer.from(chart.outputFiles[0].text).toString('base64')}`);
  const days = [{ date: '2026-09-21', completed: 0, failed: 0, total: 0 }, { date: '2026-09-22', completed: 1, failed: 0, total: 2 }];
  assert.deepEqual(parseDeliveryTrend(days), days);
  for (const invalid of [null, [], [{ ...days[0], failed: null }], [{ ...days[0], completed: -1 }], [{ ...days[0], date: '2026-02-30' }], [days[1], days[0]], [{ ...days[1], total: 0 }]]) assert.equal(parseDeliveryTrend(invalid), null);
  const definition = validateProfileView({ ...defaultView(), blocks: [{ id: 'trend', source: 'deliveryTrendChart', title: 'Daily outcomes' }] });
  assert.equal(compileProfileRequests(definition)[0].parameters.range, '30d');
});

test('malformed AI metric definitions never enter the deterministic form', () => {
  const valid = { name: 'Volume', custom_definition: { name: 'Volume', operation: 'COUNT', field: 'threads' } };
  const designer = { key: 'type', revision: 1, definition: defaultView(), apply: () => assert.fail(), applyMetrics: () => assert.fail('invalid metric must not apply') };
  for (const value of [null, {}, [{ name: 'Volume', operation: 'COUNT', field: 'threads' }], [{ ...valid, custom_definition: 'COUNT' }], [{ ...valid, custom_definition: { ...valid.custom_definition, filters: {} } }], [{ ...valid, custom_definition: { ...valid.custom_definition, filters: [{ key: 'outcome', value: {} }] } }], [{ ...valid, custom_definition: { ...valid.custom_definition, query: 'SELECT *' } }], [{ ...valid, parameters: [] }]]) {
    assert.throws(() => applyProfileViewProposal('```profile-metrics\n' + JSON.stringify(value) + '\n```', { key: 'type', revision: 1 }, designer), /metric definitions/);
  }
});


test('every saved metric receives an idempotent default presentation, preserving customization', () => {
  const metrics = [
    { id: '11111111-1111-4111-8111-111111111111', name: 'Volume' },
    { id: '22222222-2222-4222-8222-222222222222', name: 'Daily volume', custom_definition: { group_by: 'period' } },
    { id: '33333333-3333-4333-8333-333333333333', name: 'Outcomes', custom_definition: { group_by: 'outcome' } },
    { name: 'Unsaved metric' },
  ];
  const initial = defaultView();
  const view = withMetricPresentations(initial, metrics);
  assert.deepEqual(view.blocks.slice(-3).map(block => block.display), ['card', 'line', 'table']);
  assert.deepEqual(withMetricPresentations(view, metrics), view);
  assert.equal(initial.blocks.length, 5);
  view.blocks[6] = { ...view.blocks[6], title: 'My graph', display: 'bar' };
  const updated = withMetricPresentations(view, metrics.slice(1));
  assert.equal(updated.blocks.some(block => block.metricId === metrics[0].id), false);
  assert.deepEqual(updated.blocks.find(block => block.metricId === metrics[1].id), view.blocks[6]);
  assert.equal(compileProfileRequests(validateProfileView(updated)).at(-1).parameters.refKey, '$entity.refKey');
});

test('metric defaults cover existing types beyond twelve blocks without duplicating legacy all-metrics sections', () => {
  const metrics = Array.from({ length: 50 }, (_, i) => ({ id: `11111111-1111-4111-8111-${String(i).padStart(12, '0')}`, name: `Metric ${i}` }));
  const view = validateProfileView(withMetricPresentations(defaultView(), metrics));
  assert.equal(view.blocks.length, 55);
  const legacy = { ...defaultView(), blocks: [{ id: 'all', source: 'computedMetrics', title: 'All metrics' }] };
  assert.deepEqual(withMetricPresentations(legacy, metrics), legacy);
});
