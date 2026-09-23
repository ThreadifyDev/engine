import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';
const output = await build({ stdin: {contents: "export {api, ProfileViewConflictError} from './app/lib/api'; export {defaultView} from './app/components/profiles/view/profile-view';", resolveDir:fileURLToPath(new URL('..',import.meta.url))},bundle:true,write:false,format:'esm',platform:'node'});
const {api,ProfileViewConflictError,defaultView}=await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);

test('shared view API uses authenticated Engine calls and sends only the definition and expected revision',async()=>{
 const saved={fetch:globalThis.fetch,window:globalThis.window,document:globalThis.document,localStorage:globalThis.localStorage};
 const calls=[];
 globalThis.window={location:{origin:'https://engine.example'}};
 globalThis.document={cookie:'threadify_csrf_dev=csrf'};
 globalThis.localStorage={removeItem:()=>{}};
 globalThis.fetch=async(url,options)=>{calls.push({url,options});return Response.json({data:{definition:defaultView(),revision:4},can_manage:true})};
 try{
  await api.getProfileView('type/id');
  await api.saveProfileView('type/id',defaultView(),3);
  assert.deepEqual(calls.map(call=>call.url),['https://engine.example/v1/entity-profile-views/type%2Fid','https://engine.example/v1/entity-profile-views/type%2Fid']);
  assert.equal(calls[1].options.method,'PUT');
  assert.deepEqual(JSON.parse(calls[1].options.body),{definition:defaultView(),expected_revision:3});
  for(const {options} of calls){assert.equal(options.credentials,'include');assert.equal(options.headers['X-Threadify-CSRF'],'csrf');assert.equal(options.headers.Authorization,undefined)}
  globalThis.fetch=async()=>Response.json({error:'Load the latest shared view.',code:'PROFILE_VIEW_CONFLICT'},{status:409});
  await assert.rejects(api.saveProfileView('type',defaultView(),3),ProfileViewConflictError);
  globalThis.fetch=async()=>Response.json({error:'Insufficient permissions'},{status:403});
  await assert.rejects(api.saveProfileView('type',defaultView(),3),/Insufficient permissions/);
 }finally{Object.assign(globalThis,saved)}
});
