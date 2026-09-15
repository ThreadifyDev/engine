import assert from 'node:assert/strict';
import {setTimeout as sleep} from 'node:timers/promises';
import {randomUUID} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {createDemoServer} from './server.mjs';

const required = ['THREADIFY_API_KEY', 'THREADIFY_WS_URL', 'THREADIFY_GRAPHQL_URL', 'REGISTRY_CONTROL_URL', 'REGISTRY_CONTROL_TOKEN', 'REGISTRY_ACCOUNT_ID','THREADIFY_TEST_DATABASE_URL'];
for (const key of required) assert.ok(process.env[key], `${key} is required; use isolated test infrastructure`);
const all = () => Object.fromEntries(['input_bandwidth_bytes','output_bandwidth_bytes','input_requests_per_second','entity_profile_limit'].map(key => [key,-1]));
const demo = createDemoServer({apiKey: process.env.THREADIFY_API_KEY, wsUrl: process.env.THREADIFY_WS_URL, graphqlUrl: process.env.THREADIFY_GRAPHQL_URL});
await new Promise(resolve => demo.server.listen(0, '127.0.0.1', resolve));
const base = `http://127.0.0.1:${demo.server.address().port}`;
const pricing = new URL('/v1/pricing', process.env.THREADIFY_GRAPHQL_URL);
console.log(`Live Node HTTP server: ${base}`);
const checks = [];
const runID = randomUUID();
const created = [];
// Assertions read only the explicitly supplied disposable database, never production configuration.
function sqlCount(query) {
  return Number(execFileSync('psql',[process.env.THREADIFY_TEST_DATABASE_URL,'-X','-A','-t','-c',query],{encoding:'utf8'}).trim());
}
function profileCount() {
  return sqlCount("SELECT COUNT(*) FROM entity_profile WHERE ref_key IN ('sdk-one','sdk-two')");
}
async function control(change) {
  const response = await fetch(process.env.REGISTRY_CONTROL_URL, {method:'POST', headers:{Authorization:`Bearer ${process.env.REGISTRY_CONTROL_TOKEN}`,'Content-Type':'application/json'}, body:JSON.stringify({account_id:process.env.REGISTRY_ACCOUNT_ID,...change}), signal:AbortSignal.timeout(5000)});
  assert.equal(response.status,200,await response.text());
}
async function call(path, data) {
  const response = await fetch(base+path,{method:data===undefined?'GET':'POST',headers:{'Content-Type':'application/json'},body:data===undefined?undefined:JSON.stringify(data), signal:AbortSignal.timeout(5000)});
  return {status:response.status, body:await response.json()};
}
async function eventually(label, condition, ms=12000) {
  const deadline=Date.now()+ms;
  while (!(await condition())) {assert.ok(Date.now()<deadline,label);await sleep(100);}
}
async function policy() {
  const response=await fetch(pricing,{headers:{'X-API-Key':process.env.THREADIFY_API_KEY},signal:AbortSignal.timeout(3000)});
  return {status:response.status, body:response.status===200?await response.json():{}};
}
async function reset() {
  await control({entitlements:all(),suspended:false,registry_unavailable:false,usage_unavailable:false,restore:true,heartbeat_interval_seconds:1,grace_period_seconds:3});
  await eventually('unlimited policy did not refresh',async()=> {const r=await policy();return r.status===200&&Object.entries(all()).every(([k,v])=>r.body.limits[k]===v);});
}
async function setLimit(name,value) {
  await control({entitlements:{...all(),[name]:value}});
  await eventually(`policy ${name} did not refresh`,async()=>{const r=await policy();return r.status===200?r.body.limits[name]===value:r.status===429;});
}
async function start(label='SDK test', refs={}) {
  const response=await call('/threads',{label,refs:{...refs,sdk_run:runID}});
  assert.equal(response.status,201,JSON.stringify(response));
  created.push(response.body.threadId);
  return response.body.threadId;
}
async function archived(id) {
  let result;
  await eventually('SDK GraphQL archival query did not converge',async()=> {result=await call(`/threads/${id}`);return result.status===200;});
  return result.body;
}
function checked(name,detail={}) {checks.push({name,...detail});console.log(`PASS ${name}`);}
try {
  await reset();
  const id=await start('SDK complete flow');
  const step=await call(`/threads/${id}/steps`,{name:'processed',context:{order:runID},idempotencyKey:runID});
  assert.equal(step.status,200,JSON.stringify(step));
  const complete=await call(`/threads/${id}/complete`,{reason:'SDK e2e done'});
  assert.equal(complete.status,200,JSON.stringify(complete));
  await eventually('completed thread/step not visible through SDK query',async()=>{const r=await call(`/threads/${id}`);return r.status===200&&r.body.status==='completed'&&r.body.steps.some(s=>s.stepName==='processed'&&s.status==='success');});
  checked('SDK create, step, complete and persisted GraphQL query',{threadId:id});

  for(const metric of ['input_bandwidth_bytes','input_requests_per_second','output_bandwidth_bytes']) {
    await archived(await start('prime connection'));
    await setLimit(metric,0);
    const threadsBefore = sqlCount('SELECT COUNT(*) FROM threads');
    const denied=await call('/threads',{label:`denied ${metric}`,refs:{sdk_run:runID}});
    assert.equal(denied.status,429,`${metric}: ${JSON.stringify(denied)}`);
    assert.equal(denied.body.code,'THREADIFY_ALLOWANCE_EXCEEDED');
    assert.equal(denied.body.closeCode,1008);
    if (metric.startsWith('input_')) {
      await sleep(500);
      assert.equal(sqlCount('SELECT COUNT(*) FROM threads'),threadsBefore,`${metric} denial must not create a thread`);
    }
    const queryDenied=await call(`/threads/${id}`);
    assert.equal(queryDenied.status,429,JSON.stringify(queryDenied));
    await reset();
    await start(`recovered ${metric}`);
    checked(`${metric} denial over SDK WS and GraphQL, then recovery`,{mutationOutcome:metric.startsWith('output_')?'may_have_applied_without_ack':'rejected_before_input_dispatch'});
  }

  // Bandwidth is a monthly byte allowance; rate ceilings count messages only.
  for (const direction of ['input','output']) {
    const metric = `${direction}_bandwidth_bytes`;
    const big = await start('large response fixture',{large_payload:'x'.repeat(131072)});
    await archived(big);
    const used = sqlCount(`SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.${direction}.bytes'`);
    await setLimit(metric,used+65536);
    await start(`finite ${metric} allowed`);
    const denied = direction === 'input'
      ? await call('/threads',{label:`finite ${metric} denied`,refs:{payload:'x'.repeat(131072)}})
      : await call(`/threads/${big}`);
    assert.equal(denied.status,429,`${metric} finite ceiling: ${JSON.stringify(denied)}`);
    await reset();
    checked(`${metric} positive finite allowance permits small traffic and denies larger payload`);
  }
  // Confirm a single second before asserting the exact finite ceiling; scheduling may cross a window.
  let confirmedRateWindow=false;
  for(let attempt=0;attempt<3;attempt++) {
    await reset();
    await archived(await start('finite rate prime'));
    await setLimit('input_requests_per_second',2);
    await sleep(1100-Date.now()%1000);
    const began=Date.now();
    const burst=await Promise.all(Array.from({length:3},(_,index)=>call('/threads',{label:`finite input rate burst ${index}`})));
    const ended=Date.now();
    const statuses=burst.map(r=>r.status).sort();
    for(const result of burst) if(result.status===201) created.push(result.body.threadId);
    if(Math.floor(began/1000)!==Math.floor(ended/1000)) {
      console.log(`Rate burst crossed a second boundary; retrying bounded test window: ${JSON.stringify({began,ended,statuses})}`);
      continue;
    }
    assert.deepEqual(statuses,[201,201,429],`finite incoming rate in one second: ${JSON.stringify({began,ended,burst})}`);
    const bucketCount=sqlCount(`SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds=1 AND metric='threadify.input.requests' AND bucket_start=to_timestamp(${Math.floor(began/1000)})`);
    assert.equal(bucketCount,2,'confirmed one-second usage bucket must record only the two admitted requests');
    confirmedRateWindow=true;
    checked('input_requests_per_second positive rate allows two requests then denies the third',{began,ended,statuses,bucketCount});
    break;
  }
  assert.ok(confirmedRateWindow,'could not obtain a same-second rate test window in three attempts');
  await reset();

  // A large payload uses one message regardless of bytes when monthly bandwidth is available.
  const largePayload = 'x'.repeat(131072);
  const largeThread = await start('count-only rate large response',{large_payload:largePayload});
  await archived(largeThread);
  await control({entitlements:{...all(),input_requests_per_second:2}});
  await eventually('count-only rate policy did not refresh',async()=> {const r=await policy();return r.status===200&&r.body.limits.input_requests_per_second===2;});
  await sleep(1100-Date.now()%1000);
  await start('count-only rate large input',{large_payload:largePayload});
  await sleep(1100-Date.now()%1000);
  const largeResult = await call(`/threads/${largeThread}`);
  assert.equal(largeResult.status,200,JSON.stringify(largeResult));
  assert.equal(largeResult.body.refs.large_payload.length,largePayload.length);
  await reset();
  checked('incoming request rate allows 128 KiB input and output without a byte throttle');

  // Each SDK archived read performs two GraphQL requests; output messages have no rate ceiling.
  await setLimit('input_requests_per_second',16);
  const currentPolicy=await policy();
  assert.equal(currentPolicy.status,200);
  assert.ok(!Object.hasOwn(currentPolicy.body.limits,'output_messages_per_second'),'response must not advertise a retired outgoing rate allowance');
  await sleep(1100-Date.now()%1000);
  const outputsBefore=sqlCount("SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.output.messages'");
  const reads=await Promise.all(Array.from({length:6},()=>call(`/threads/${largeThread}`)));
  for(const result of reads) {
    assert.equal(result.status,200,JSON.stringify(result));
    assert.equal(result.body.refs.large_payload.length,largePayload.length);
  }
  const outputResponses=sqlCount("SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.output.messages'")-outputsBefore;
  assert.ok(outputResponses>=12,`six SDK archived reads must emit their metadata and step responses, got ${outputResponses}`);
  await reset();
  checked('SDK read burst emits twelve responses with only incoming request rate enforced',{outputResponses});

  await setLimit('entity_profile_limit',0);
  const one=await start('profile one',{sdk_customer:'sdk-one'});
  const two=await start('profile two',{sdk_customer:'sdk-two'});
  assert.equal((await archived(one)).refs.sdk_customer,'sdk-one');
  assert.equal((await archived(two)).refs.sdk_customer,'sdk-two');
  assert.equal(profileCount(),0,'zero profile allowance must retain no materialized profiles');
  checked('zero profile allowance preserves threads and refs without materializing profiles');
  await setLimit('entity_profile_limit',1);
  for(const [id,ref] of [[one,'sdk-one'],[two,'sdk-two']]) assert.equal((await call(`/threads/${id}/refs`,{refs:{sdk_customer:ref}})).status,200);
  await eventually('profile cap materialization',async()=>profileCount()===1);
  checked('profile cap preserves thread creation and refs',{threads:[one,two],profiles:profileCount()});
  await setLimit('entity_profile_limit',2);
  for(const [id,ref] of [[one,'sdk-one'],[two,'sdk-two']]) assert.equal((await call(`/threads/${id}/refs`,{refs:{sdk_customer:ref}})).status,200);
  await eventually('profile allowance upgrade did not materialize second profile',async()=>profileCount()===2);
  checked('profile upgrade accepted through SDK addRefs',{profiles:profileCount()});
  await reset();

  await start('prime suspension');
  await control({suspended:true});
  await eventually('suspension did not refresh',async()=> (await policy()).status===503);
  const suspended=await call('/threads',{label:'must reject suspension'});
  assert.equal(suspended.status,503,JSON.stringify(suspended));
  assert.equal(suspended.body.code,'THREADIFY_LICENSE_UNAVAILABLE');
  await reset();await start('restored suspension');
  checked('explicit suspension rejects SDK mutation and restores');

  const outageUsed=sqlCount("SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.input.bytes'");
  await setLimit('input_bandwidth_bytes',outageUsed+65536);
  await control({registry_unavailable:true});
  const until=Date.now()+4200;
  let outageCreates=0;
  do {await start('Registry outage continuity');outageCreates++;await sleep(300);}while(Date.now()<until);
  const offlineDenied=await call('/threads',{label:'offline oversize denied',refs:{payload:'x'.repeat(131072)}});
  assert.equal(offlineDenied.status,429,JSON.stringify(offlineDenied));
  await control({registry_unavailable:false});
  await setLimit('input_bandwidth_bytes',0);
  assert.equal((await call(`/threads/${id}`)).status,429);
  await reset();await start('Registry recovery');
  checked('Registry outage retains monthly bandwidth allowance and availability beyond former grace; recovery applies new policy',{outageCreates});

  await control({revoke:true});
  await eventually('revocation did not refresh',async()=> (await policy()).status===503);
  const revoked=await call('/threads',{label:'must reject revoked license'});
  assert.equal(revoked.status,503,JSON.stringify(revoked));
  await reset();await start('restored revocation');
  checked('revoked license rejects SDK mutation and restores');
  console.log(JSON.stringify({passed:checks.length,runID,created,checks},null,2));
} finally {await reset();await demo.close();}
