import http from 'node:http';
import { pathToFileURL } from 'node:url';
import { Threadify } from '@threadify/sdk';

/** A loopback demonstration server using only public SDK operations for Engine data. */
export function createDemoServer(config) {
  const threads = new Map();
  let connection;
  let connecting;
  // Recovery creates a connection for the next explicit operation; mutations are never replayed.
  async function connected() {
    if (connection?.isConnected) return connection;
    if (!connecting) connecting = Threadify.connect(config.apiKey, config.serviceName || 'license-sdk-demo', {
      wsUrl: config.wsUrl, graphqlUrl: config.graphqlUrl,
    }).then(value => (connection = value)).finally(() => { connecting = undefined; });
    return connecting;
  }
  async function body(req) {
    let raw = '';
    for await (const chunk of req) {
      raw += chunk;
      if (Buffer.byteLength(raw) > 2 * 1024 * 1024) throw Object.assign(new Error('Demo request exceeds 2 MiB transport limit'), {status: 413});
    }
    try { return raw ? JSON.parse(raw) : {}; }
    catch { throw Object.assign(new Error('Invalid JSON body'), {status: 400}); }
  }
  const server = http.createServer(async (req, res) => {
    res.setHeader('Content-Type', 'application/json');
    try {
      const path = new URL(req.url, 'http://localhost').pathname;
      if (req.method === 'GET' && path === '/health') {
        res.end(JSON.stringify({status: 'ok', sdkConnected: !!connection?.isConnected}));
        return;
      }
      let result;
      if (req.method === 'POST' && path === '/threads') {
        const input = await body(req);
        const thread = await (await connected()).start(input.label || 'Node SDK demonstration', {refs: input.refs || {}});
        threads.set(thread.id, thread);
        res.statusCode = 201;
        result = {threadId: thread.id};
      } else {
        const match = path.match(/^\/threads\/([^/]+)(?:\/(steps|complete|refs))?$/);
        if (!match) throw Object.assign(new Error('Route not found'), {status: 404});
        const [, id, action] = match;
        if (req.method === 'GET' && !action) {
          // GraphQL reads remain available through the SDK even after its WS transport closed.
          const archived = await (connection || await connected()).getThread(id);
          if (!archived) throw Object.assign(new Error('Thread not yet archived'), {status: 404});
          const steps = await archived.steps();
          result = {threadId: archived.id, status: archived.status, refs: archived.refs, steps: steps.map(s => ({stepName: s.stepName, status: s.status, idempotencyKey: s.idempotencyKey}))};
        } else if (req.method === 'POST' && action) {
          const thread = threads.get(id);
          if (!thread) throw Object.assign(new Error('Thread was not created by this demo process'), {status: 404});
          const input = await body(req);
          thread.connection = await connected();
          if (action === 'steps') {
            const step = thread.step(input.name || 'processed').addContext(input.context || {});
            if (input.idempotencyKey) step.idempotencyKey(input.idempotencyKey);
            result = await step.success(input.message || 'Processed by the Node SDK server');
          } else if (action === 'refs') {
            result = await thread.addRefs(input.refs || {});
          } else {
            result = await thread.complete(input.reason || 'Completed by the Node SDK server');
          }
        } else throw Object.assign(new Error('Method not allowed'), {status: 405});
      }
      res.end(JSON.stringify(result));
    } catch (error) {
      res.statusCode = Number.isInteger(error.status) && error.status >= 400 && error.status <= 599 ? error.status : 502;
      res.end(JSON.stringify({error: error.message, code: error.code || 'SDK_OPERATION_FAILED', ...(error.closeCode ? {closeCode: error.closeCode, outcome: 'not_acknowledged'} : {})}));
    }
  });
  return {server, async close() { if (connection) await connection.close(); await new Promise(resolve => server.close(resolve)); }};
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  for (const name of ['THREADIFY_API_KEY', 'THREADIFY_WS_URL', 'THREADIFY_GRAPHQL_URL']) {
    if (!process.env[name]) throw new Error(`${name} is required`);
  }
  const demo = createDemoServer({apiKey: process.env.THREADIFY_API_KEY, wsUrl: process.env.THREADIFY_WS_URL, graphqlUrl: process.env.THREADIFY_GRAPHQL_URL});
  demo.server.listen(Number(process.env.PORT || 3107), '127.0.0.1', () => console.log(`Node SDK server listening at http://127.0.0.1:${demo.server.address().port}`));
  process.once('SIGTERM', () => demo.close().then(() => process.exit(0)));
  process.once('SIGINT', () => demo.close().then(() => process.exit(0)));
}
