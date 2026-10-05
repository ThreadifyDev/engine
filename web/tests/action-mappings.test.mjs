import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';

const output = await build({stdin:{contents:"export * from './app/lib/action-mappings';",resolveDir:fileURLToPath(new URL('..',import.meta.url))},bundle:true,write:false,format:'esm',platform:'node'});
const {parseActionMappings:parse,formatActionMappings:format,draftMappingActions:draft} = await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`);

test('regex mappings preserve commas, equals, flags, and escapes', () => {
  const text = 'checkout_clicked,checkout_confirmed=checkout\nregex:(?i)^POST /orders/[0-9]{1,3}\\?state=paid$=paid';
  const rules = parse(text,'order',1,['checkout','paid']);
  assert.deepEqual(rules.map(r=>r.action),['checkout_clicked','checkout_confirmed','regex:(?i)^POST /orders/[0-9]{1,3}\\?state=paid$']);
  assert.equal(rules[2].step,'paid');
  assert.equal(format(rules),text);
  assert.deepEqual(draft(text),rules.map(r=>r.action));
  assert.deepEqual(draft('regex:(?i)^x{1,3}=$=\ncheckout='),['regex:(?i)^x{1,3}=$','checkout']);
});

test('format and parse preserve regex ordering across different target steps', () => {
  const text = 'regex:(?i)^checkout=first\nexact=second\nregex:checkout.*=second\nregex:.*checkout=first';
  const rules = parse(text,'order',2,['first','second']);
  assert.equal(format(rules),text);
  assert.deepEqual(parse(format(rules),'order',2,['first','second']),rules);
});

test('malformed mappings, mixed regex lists, duplicate patterns and unknown steps are rejected', () => {
  for (const text of ['regex:=checkout','regex:abc','regex:abc=','a,regex:abc=checkout','a=checkout\na=checkout','regex:abc=unknown','a=b=checkout']) {
    assert.throws(()=>parse(text,'order',1,['checkout']),/Line/);
  }
  // Syntax is validated by the Go/RE2 engine, not JavaScript RegExp.
  assert.equal(parse('regex:(?i)checkout=checkout','order',1,['checkout'])[0].action,'regex:(?i)checkout');
});
