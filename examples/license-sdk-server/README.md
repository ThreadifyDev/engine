# Node server using the Threadify SDK

This runnable HTTP server uses the workspace **@threadify/sdk** for keyed thread creation/resume, steps, completion, references and GraphQL reads. Its `file:../../threadify-sdk` dependency loads the updated SDK directly.

## Run

Use Node.js 24 LTS (`nvm install && nvm use` from the repository root).

The SDK is an independent, ignored checkout. If `threadify-sdk` is absent, run
`git clone https://github.com/ThreadifyDev/node-sdk.git threadify-sdk` from the
repository root before installing this example.

```sh
cd examples/license-sdk-server
npm install --prefix ../../threadify-sdk
npm install
THREADIFY_API_KEY='your-engine-service-api-key' \
THREADIFY_WS_URL='ws://127.0.0.1:8081/threads' \
THREADIFY_GRAPHQL_URL='http://127.0.0.1:8081/graphql' \
PORT=3107 npm start
```

The API key belongs to an Engine service account; it is **not** the Registry license key. The server binds loopback and holds created thread handles in memory. Restarting this example loses those handles; POST the same `threadKey` to restore an open thread. Archived threads remain queryable. Add application authentication and durable business state before exposing a real application publicly.

```sh
curl http://127.0.0.1:3107/threads -H 'Content-Type: application/json' \
  -d '{"threadKey":"order:example-1","label":"checkout","refs":{"order":"example-1"}}'
# Use the returned threadId below.
curl http://127.0.0.1:3107/threads/THREAD_ID/steps -H 'Content-Type: application/json' \
  -d '{"name":"paid","context":{"amount":19},"idempotencyKey":"payment-example-1"}'
curl http://127.0.0.1:3107/threads/THREAD_ID/complete -H 'Content-Type: application/json' \
  -d '{"reason":"Order fulfilled"}'
curl http://127.0.0.1:3107/threads/THREAD_ID
```

Archival is asynchronous; the final query can briefly return 404 before data is persisted. `POST /threads/THREAD_ID/refs` accepts `{"refs":{"customer":"example"}}`. The demonstration HTTP parser has a 2 MiB transport guard, independent of Registry license limits.

`POST /threads` accepts an application `threadKey` and optional `label`, `refs`, and `contract`. Calling it again with the same key resumes the stored thread and contract. Omit `contract` when resuming. The response includes both `threadId` and `threadKey`; omitted keys are generated for one-off demo clients. A closed thread cannot be resumed for writes.

## Errors and recovery

The SDK rejects pending operations when their WebSocket closes. Recognized Engine policy close reasons retain useful codes and statuses: allowance exceeded → 429, unavailable license/accounting → 503. Other transport failures report 502. HTTP GraphQL failures retain their upstream status. On the next explicit request, the demo reconnects if necessary.

**Mutations are never automatically retried.** An output allowance can prevent delivery of an acknowledgement after the mutation has already happened. A closed transport reports `outcome: "not_acknowledged"`; inspect persisted state before retrying. Step idempotency keys are useful for this reconciliation.

## Tests

`npm test` runs the SDK error regressions, including pending create/step/complete cancellation, explicit close reasons, denied HTTP upgrade, early close before authentication acknowledgement, GraphQL 429, and successful response handling.

`npm run test:live` starts a real Node HTTP server and drives it through the SDK against an isolated compiled Engine and the retained Registry handler fixture. Supply:

- `THREADIFY_API_KEY`, `THREADIFY_WS_URL`, `THREADIFY_GRAPHQL_URL`
- `REGISTRY_CONTROL_URL`, `REGISTRY_CONTROL_TOKEN`, `REGISTRY_ACCOUNT_ID`
- `THREADIFY_TEST_DATABASE_URL` — explicitly disposable Engine PostgreSQL database; `psql` is used only for assertions.

Seed an `sdk_customer` entity profile type in that Engine account before running. The live test checks lifecycle persistence, three zero and positive finite input/output bandwidth and incoming request rate tests over SDK WebSocket and GraphQL, input rejection without thread creation, profile limits and upgrades, explicit suspension/revocation, a 128 KiB input/output check under the incoming rate, a six-read burst producing twelve SDK GraphQL responses without an outgoing rate limit, and continued operation during Registry heartbeat outages under a finite monthly bandwidth allowance followed by refreshed limits on recovery. Only incoming requests are rate limited. Outgoing messages remain usage telemetry and consume monthly output bandwidth, with no outgoing rate ceiling or per-second byte limits. Test fixture timing is one-second heartbeat/three-second advertised freshness. Production heartbeat timing is unchanged.

These tests cover the listed licensing paths; they do not claim every combination of all license features or production load behavior is tested. The existing local Registry at port 59903 is separate and is not modified by this test.
