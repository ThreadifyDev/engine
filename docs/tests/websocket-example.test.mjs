import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {randomUUID} from 'node:crypto';

const doc = readFileSync(new URL('../WEBSOCKET.md', import.meta.url), 'utf8');
const source = doc.slice(doc.indexOf('class ThreadifyClient {'), doc.indexOf('// Usage', doc.indexOf('class ThreadifyClient {')));

test('documented client authenticates, correlates concurrent keys, and rejects pending disconnects', async () => {
  let ws;
  class Socket {
    static OPEN = 1;
    constructor(url) { ws = this; this.url = url; this.readyState = 0; this.sent = []; }
    send(raw) { this.sent.push(JSON.parse(raw)); }
    reply(request, body) { this.onmessage({data: JSON.stringify({action: request.action, requestId: request.requestId, status: 'success', ...body})}); }
    close() { this.readyState = 3; this.onclose(); }
  }
  const Client = vm.runInNewContext(`${source}; ThreadifyClient`, {WebSocket: Socket, crypto: {randomUUID}, setTimeout, clearTimeout});
  const client = new Client('test');
  const first = client.thread('first');
  const second = client.thread('second');
  assert.equal(ws.url, 'ws://localhost:8081/threads');
  assert.equal(ws.sent.length, 0);
  ws.readyState = 1;
  const authenticating = ws.onopen();
  assert.equal(ws.sent.length, 1);
  assert.equal(ws.sent[0].action, 'connect');
  ws.reply(ws.sent[0], {});
  await authenticating;
  await Promise.resolve();
  const a = ws.sent.find(q => q.threadKey === 'first');
  const b = ws.sent.find(q => q.threadKey === 'second');
  assert(a && b);
  assert.notEqual(a.requestId, b.requestId);
  ws.reply(b, {threadId: 'B', threadKey: 'second'});
  ws.reply(a, {threadId: 'A', threadKey: 'first'});
  assert.equal((await first).threadId, 'A');
  assert.equal((await second).threadId, 'B');
  const pending = client.thread('third');
  const rejected = assert.rejects(pending, /Connection closed/);
  await Promise.resolve();
  ws.close();
  await rejected;
  assert.equal(client.pending.size, 0);
});
