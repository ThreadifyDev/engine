# Historical validation — superseded rate model

This records the completed test run before the user clarified that rates count requests and responses only. Byte-per-second limits in this report are retired. Historical JSON evidence is unchanged.

# Node SDK licensing validation

Validated 15 September 2026 using the current local **@threadify/sdk 0.1.24**, a real Node HTTP server, the compiled Threadify Engine, PostgreSQL/Valkey, and the separate Registry licensing handler fixture. The root project's installed SDK remains 0.1.23; the example deliberately uses the local SDK through a file dependency.

**Result: 19/19 live scenario groups passed.** The final Node listener was `127.0.0.1:63015`, Engine `127.0.0.1:28191`, and isolated Registry `127.0.0.1:62483`. The test closed the Node server cleanly before the independent usage audit. The user's retained Registry on port 59903 and its database were not touched.

## Live coverage

| Restriction or flow | What passed |
|---|---|
| SDK lifecycle | Node HTTP create → SDK start, step success, complete; persisted metadata and successful step read through SDK GraphQL |
| Input monthly bandwidth | Zero rejects WS mutation and GraphQL; finite remaining allowance admits small traffic and rejects an oversized payload |
| Output monthly bandwidth | Zero rejects WS acknowledgement and GraphQL output; finite remaining allowance admits small traffic and rejects an oversized response |
| Input bytes per second | Zero rejects; positive 2,048-byte allowance admits small traffic and rejects a larger payload |
| Output bytes per second | Zero rejects; positive 2,048-byte allowance admits small traffic and rejects a larger response |
| Input requests per second | Zero rejects; positive limit of two permits two operations and rejects the third in the same second |
| Output messages per second | Zero rejects; positive limit of two permits two responses and rejects the third in the same second |
| Entity profile limit | Zero preserves threads and refs with no profiles; upgrade to one materializes one profile; upgrade to two materializes both |
| Suspension | SDK mutation receives explicit unavailable-license error; restoration permits another request |
| Revoked key | Signed heartbeat observes revocation; SDK mutation is denied; key restoration recovers |
| Registry outage | 14 SDK thread creations continue beyond the former grace interval under the last verified finite input limit; oversized input remains denied; recovery applies changed limits |

Every zero transport-limit case covered both a SDK WebSocket operation and a SDK GraphQL request, followed by recovery. Input denial also checked that the database thread count did not increase. Output-only denials intentionally make no assertion that the mutation did not happen: an accepted mutation can lose its acknowledgement when output is exhausted. The application does not automatically retry mutations.

Monthly finite tests account for the fact that inspecting `/v1/pricing` itself consumes output bandwidth. They use a finite 65,536-byte remaining budget and a 131,072-byte payload, while per-second byte tests use 2,048 bytes. The first test attempt exposed an insufficient test budget for metered pricing reads; the corrected complete matrix passed.

## SDK defect found and fixed

Three deterministic regressions initially failed: `start()`, step `success()`, and `complete()` never settled after Engine closed the WebSocket while a response was pending. Pending operations now reject on transport close/error, and their response handlers are removed. Recognized Engine close reasons map to allowance (429), license (503), and accounting (503) errors; an ordinary network close remains an unknown acknowledgement outcome (502). HTTP upgrade and GraphQL errors preserve their HTTP status. Failed initial connection handshakes clear their timeout.

**10/10 SDK regression tests passed**, including successful response handling and real localhost upgrade-denial/early-close cases. Run them with `npm run test:connection-errors` in `threadify-sdk`, or `npm test` in this example. The CommonJS build was regenerated successfully.

## Independent durable usage audit

After Node traffic and its connection stopped, an independent read-only audit passed all eight assertions:

| Metric | Threadify | Registry |
|---|---:|---:|
| Input bytes | 595,077 | 595,077 |
| Input requests | 575 | 575 |
| Output bytes | 675,563 | 675,563 |
| Output messages | 510 | 510 |

The durable outbox contained zero pending reports. There were 73 threads, two profiles, zero legacy credit accounts, and zero billing snapshots. These totals include the first harness attempt and the final 19-group run. The audit also checked that Registry entitlements were not stored in the Engine database.

## Scope

The Node server uses SDK methods for all Engine workflow operations and archived queries. Direct HTTP calls only control the isolated Registry fixture and observe Engine policy synchronization; `psql` only checks persisted outcomes. The fixture uses production licensing handlers and repository persistence, with a no-delivery identity stub and one-second heartbeat/three-second advertised freshness for test speed. Production cadence remains unchanged.

This is focused integration coverage of the listed paths, not proof of every product feature, every combination of limits, or production load behavior. Database storage capacity is not a commercial per-thread or thread-count limit. No deployment or package publication occurred.

The disposable Engine, Registry fixture and test containers were stopped after
validation. The user's local Registry at port 59903 remains healthy.

See [machine-readable final results](node-sdk-live-result.json),
[accounting audit](accounting.json), and
[Registry allowance audit](registry-allowances.json).
Reproduction instructions are in [README.md](../README.md).
