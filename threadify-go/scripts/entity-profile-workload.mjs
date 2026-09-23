// Opt-in synthetic workload. Use only a disposable Engine/database.
// THREADIFY_PROFILE_SDK points at an SDK checkout containing src/index.js.
// Arguments: Engine URL, disposable API key, result JSON path.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { performance } from 'node:perf_hooks';

const { Threadify } = await import(pathToFileURL(`${process.env.THREADIFY_PROFILE_SDK}/src/index.js`));
const [url, key, output] = process.argv.slice(2);
assert(url && key && output, 'Expected Engine URL, API key, output path');
const count = Number(process.env.THREADIFY_PROFILE_RUNS || 1024);
const database = new URL(process.env.THREADIFY_SMOKE_POSTGRES_URL);
const concurrency = 32;
assert(Number.isInteger(count) && count >= 32 && count % concurrency === 0);
const runRef = `refund-assistant-${Date.now()}`;
const contract = `profile_workload_${Date.now()}`;
const result = { synthetic: true, startedAt: new Date().toISOString(), concurrency, requestedRuns: count,
  contract, runRef, checks: { blockedWaits: 0, contentRejections: 0, recordedViolations: 0, providerFailures: 0 }, runs: [], errors: [] };
const save = () => writeFileSync(output, JSON.stringify(result, null, 2) + '\n');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const latency = { permission: [], validation: [] };
const timed = async (name, fn) => { const start = performance.now(); try { return await fn(); } finally { latency[name].push(performance.now() - start); } };
const exchange = await fetch(url + '/auth/api-key/exchange', { method: 'POST', headers: { Origin: url, 'Content-Type': 'application/json' }, body: JSON.stringify({ api_key: key }) });
assert.equal(exchange.status, 200, await exchange.text());
const cookies = exchange.headers.getSetCookie().map(c => c.split(';')[0]);
const csrf = cookies.find(c => c.startsWith('threadify_csrf_dev='))?.split('=').slice(1).join('=');
assert(csrf);
async function request(path, body, method = 'POST', plain = false) {
  const response = await fetch(url + path, { method, headers: { Origin: url, Cookie: cookies.join('; '), 'X-Threadify-CSRF': csrf,
    'Content-Type': plain ? 'text/plain' : 'application/json' }, body: plain ? body : JSON.stringify(body) });
  const text = await response.text();
  assert(response.ok, `${path}: ${response.status}: ${text}`);
  return JSON.parse(text);
}
async function gql(query, variables = {}) {
  const data = await request('/graphql', { query, variables });
  assert(!data.errors?.length, JSON.stringify(data.errors));
  return data.data;
}
const source = `Feature: ${contract}
Rule: Request
 When step "requested" is submitted
 Then owner must be "processor"
 And this step is an entry point
Rule: Approval
 When step "approval" is submitted
 Then owner must be "processor"
 And this step is an entry point
Rule: Refund
 When step "refund" is submitted
 Then owner must be "processor"
 And step "approval" must succeed before each invocation
 And content "amount" must be a number greater than 0
 And content "currency" must be one of "GBP", "USD"
 And content "reference" must match regex "^PAY-[0-9]{8}$"
Rule: Finish
 When step "finish" is submitted
 Then owner must be "processor"
 And step "refund" must have succeeded
 And this step is terminal
`;
const definition = { name: 'Refund Assistant Workload', type: ['agent_id'], description: 'Synthetic refund workload; generated from recorded executions.', metrics: [
  { name: 'Runs', custom_definition: { name: 'Runs', target: 'thread', operation: 'COUNT', field: 'threads' } },
  { name: 'Recorded steps', custom_definition: { name: 'Recorded steps', target: 'step', operation: 'COUNT', field: 'steps' } },
  { name: 'Rule violations', custom_definition: { name: 'Rule violations', target: 'thread', operation: 'COUNT', field: 'violations' } },
  { name: 'Failed refunds', custom_definition: { name: 'Failed refunds', target: 'step', operation: 'COUNT', field: 'steps', step_name: 'refund', filters: [{ key: 'step outcome', value: 'failed' }] } },
] };
const connections = [];
try {
  await request('/v1/contracts', source, 'POST', true);
  const profileType = await request('/v1/entity-profile-types/refund_assistant_workload', definition, 'PUT');
  result.profileDefinition = definition;
  result.contractSource = source;
  for (let n = 0; n < concurrency; n++) connections.push(await Threadify.connect(key, 'processor', { wsUrl: url.replace('http', 'ws') + '/threads' }));
  const report = (thread, step, status = 'success', context = { amount: 25, currency: 'GBP', reference: 'PAY-12345678' }) => timed('validation', () =>
    thread.step(step).idempotencyKey(randomUUID()).addContext(context)[status]('', { waitFor: true }));
  const passed = async (...args) => assert.equal((await report(...args)).validation.decision, 'passed');
  const allowed = async thread => assert.equal((await timed('permission', () => thread.waitFor('refund'))).decision, 'allowed');
  const blocked = async thread => {
    await assert.rejects(thread.waitFor('refund', { timeout: 100 }), { code: 'THREADIFY_WAIT_TIMEOUT' });
    result.checks.blockedWaits++;
  };
  const started = performance.now();
  const workers = await Promise.allSettled(connections.map(async (connection, worker) => {
    // One unrelated entity per connection must never leak into the target profile.
    const control = await connection.thread(`${runRef}:control:${worker}`, { label: 'Control entity', contract: contract + ':1', refs: { agent_id: 'unrelated-control' } });
    await passed(control, 'requested'); await passed(control, 'approval'); await allowed(control); await passed(control, 'refund'); await passed(control, 'finish');
    for (let i = worker; i < count; i += concurrency) {
      const scenario = i % 4;
      const thread = await connection.thread(`${runRef}:refund:${i}`, { label: `Refund-${String(i + 1).padStart(4, '0')}`, contract: contract + ':1', refs: { agent_id: runRef } });
      await passed(thread, 'requested');
      if (scenario === 1) {
        await blocked(thread);
        // Simulate an unguarded application recording a refund without approval.
        await assert.rejects(report(thread, 'refund'), { code: 'THREADIFY_VALIDATION_VIOLATED' });
        result.checks.recordedViolations++;
      }
      if (scenario === 3) {
        for (const context of [
          { amount: -1, currency: 'GBP', reference: 'PAY-12345678' },
          { amount: 25, currency: 'EUR', reference: 'PAY-12345678' },
          { amount: 25, currency: 'GBP', reference: 'INVALID' },
          { amount: 25, currency: 'GBP' },
        ]) {
          await assert.rejects(report(thread, 'refund', 'success', context), error => /content/i.test(error.message));
          result.checks.contentRejections++;
        }
      }
      await passed(thread, 'approval');
      await allowed(thread);
      if (scenario === 2) {
        await passed(thread, 'refund', 'failed');
        result.checks.providerFailures++;
        await blocked(thread);
        await passed(thread, 'approval');
        await allowed(thread);
      }
      await passed(thread, 'refund');
      await passed(thread, 'finish');
      result.runs.push({ id: thread.threadId, label: `Refund-${String(i + 1).padStart(4, '0')}`, scenario,
        expectedSteps: [4, 5, 6, 4][scenario], expectedViolations: scenario === 1 ? 1 : 0 });
      if (result.runs.length % 128 === 0) console.log(`Recorded ${result.runs.length}/${count} runs`);
    }
  }));
  for (const worker of workers) if (worker.status === 'rejected') throw worker.reason;
  result.workloadElapsedMs = performance.now() - started;
  assert.deepEqual(result.checks, { blockedWaits: count / 2, contentRejections: count, recordedViolations: count / 4, providerFailures: count / 4 });
  const expectedArchive = { runs: count, completed: count, steps: count * 19 / 4, violations: count / 4, failedRefunds: count / 4 };
  const archiveDeadline = Date.now() + 60000;
  // GraphQL can serve current step state from Valkey. Independently wait for
  // the PostgreSQL archive, which is the source used by profile metrics.
  while (true) {
    const sql = `WITH target AS (SELECT t.id, t.status FROM threads t JOIN thread_refs r ON r.thread_id=t.id WHERE r.ref_key='agent_id' AND r.ref_value='${runRef}')
      SELECT json_build_object('runs',(SELECT count(*) FROM target),'completed',(SELECT count(*) FROM target WHERE status='completed'),
      'steps',(SELECT count(*) FROM thread_step_states WHERE thread_id IN (SELECT id FROM target)),
      'violations',(SELECT count(*) FROM thread_notifications WHERE thread_id IN (SELECT id FROM target) AND notification_type='rule.violated'),
      'failedRefunds',(SELECT count(*) FROM thread_step_states WHERE thread_id IN (SELECT id FROM target) AND step_name='refund' AND status='failed'))`;
    result.archiveCounts = JSON.parse(execFileSync('docker', ['exec', process.env.THREADIFY_PROFILE_PG_CONTAINER || 'threadifyengine-threadify_storage-1',
      'psql', '-U', decodeURIComponent(database.username), '-d', decodeURIComponent(database.pathname.slice(1)), '-Atc', sql], { encoding: 'utf8' }));
    if (JSON.stringify(result.archiveCounts) === JSON.stringify(expectedArchive)) break;
    assert(Date.now() < archiveDeadline, `Archive mismatch: ${JSON.stringify(result.archiveCounts)}`);
    await pause(250);
  }
  console.log('Checking persisted runs and their step/violation counts');
  // Independently drain persistence before computing cacheable profile metrics.
  let cursor = 0;
  await Promise.all(Array.from({ length: 8 }, async () => {
    while (cursor < result.runs.length) {
      const run = result.runs[cursor++];
      const deadline = Date.now() + 60000;
      while (true) {
        const data = await gql('query($id: ID!) { thread(id: $id) { id status steps { stepName status } notifications { notificationType } } }', { id: run.id });
        const thread = data.thread;
        const violations = thread?.notifications?.filter(n => n.notificationType === 'rule.violated').length;
        if (thread?.status === 'completed' && thread.steps.length === run.expectedSteps && violations === run.expectedViolations) {
          run.persisted = thread; break;
        }
        assert(Date.now() < deadline, `Persistence mismatch for ${run.label}: ${JSON.stringify(thread)}`);
        await pause(250);
      }
    }
  }));
  const lookup = await gql('query($ref: String!) { entityProfile(refKey: $ref, type: "refund_assistant_workload") { id name refKey } }', { ref: runRef });
  assert(lookup.entityProfile, 'Matching references should materialize a profile');
  const profileID = lookup.entityProfile.id;
  const named = await request('/v1/entity-profiles', { type_id: profileType.data.id, ref_value: runRef, name: 'Refund assistant — synthetic workload' }, 'PUT');
  assert.equal(named.data.id, profileID, 'Naming an existing entity must retain its identity');
  const profileData = await gql('query($id: String!, $profile: ID!) { entityProfile(id: $id) { id name refKey createdAt lastActiveAt profileType { name type } computedMetrics(range: "7d") } entityProfileHistory(profileID: $profile, limit: 1) { totalCount } }', { id: profileID, profile: profileID });
  result.entityProfile = profileData.entityProfile;
  result.historyCount = profileData.entityProfileHistory.totalCount;
  console.log(JSON.stringify(profileData));
  assert.equal(result.historyCount, count, 'Exclude other entities');
  const expected = { 'runs:value': count, 'recorded_steps:value': count * 19 / 4, 'rule_violations:value': count / 4, 'failed_refunds:value (step_name:refund)': count / 4 };
  assert.deepEqual(result.entityProfile.computedMetrics, expected);
  const cached = await gql('query($id: String!) { entityProfile(id: $id) { computedMetrics(range: "7d") } }', { id: profileID });
  assert.deepEqual(cached.entityProfile.computedMetrics, expected, 'Cached reads must agree');
  result.controlRunsExcluded = concurrency;
  result.completed = true;
} catch (error) {
  result.errors.push({ message: error.message, code: error.code, stack: error.stack });
  console.error(error);
  process.exitCode = 1;
} finally {
  result.latencyMs = Object.fromEntries(Object.entries(latency).map(([name, values]) => {
    values.sort((a, b) => a - b);
    return [name, { samples: values.length, median: values[Math.ceil(values.length * .5) - 1], p95: values[Math.ceil(values.length * .95) - 1], p99: values[Math.ceil(values.length * .99) - 1], max: values.at(-1) }];
  }));
  result.finishedAt = new Date().toISOString();
  result.runs.sort((a, b) => a.label.localeCompare(b.label));
  save();
  for (const connection of connections) connection.ws.close();
}
