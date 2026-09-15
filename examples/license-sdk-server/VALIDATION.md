# Node SDK licensing validation

## Current model: incoming request rate only

Validated 15 September 2026 against the rebuilt four-allowance Engine and Registry. **15/15 live scenario groups passed.** A real Node HTTP server used local **@threadify/sdk 0.1.24** with the compiled Engine, PostgreSQL/Valkey, and a separate Registry licensing handler fixture.

The four allowances are input monthly bandwidth in bytes, output monthly bandwidth in bytes, input requests per second, and entity profile count. Outgoing messages remain usage telemetry, with no outgoing rate ceiling. There are no per-second byte limits.

| Allowance or flow | Passing checks |
|---|---|
| SDK lifecycle | Create → step success → complete; persisted metadata and successful step read through SDK GraphQL |
| Input monthly bandwidth | Zero denies SDK WS and GraphQL; finite remaining allowance admits small traffic and denies an oversized payload |
| Output monthly bandwidth | Zero denies SDK WS acknowledgement and GraphQL output; finite remaining allowance admits a small response and denies a larger response |
| Incoming request rate | Zero denies; a simultaneous three-request burst returns 201, 201, 429 in one confirmed second, with durable counter exactly two |
| Large payload | 128 KiB input and output succeed under an incoming request rate of two per second with monthly bandwidth available |
| Outgoing responses | Six concurrent SDK archived reads produce all twelve GraphQL responses under an incoming request allowance of sixteen; output usage increases by twelve; pricing has no outgoing rate field |
| Entity profile count | Zero preserves threads and refs without profiles; upgrades to one and two materialize the corresponding number |
| Suspension and revocation | Explicit changes deny SDK mutations; restoration permits subsequent requests |
| Registry outage | Fourteen small SDK creates continue beyond the former grace interval using a finite monthly input allowance; oversized input is still denied; recovery applies new limits |

Each zero bandwidth/rate case covered both SDK WebSocket and SDK GraphQL behavior, followed by recovery. Input rejection also checked that database thread count did not increase. An output denial does not imply rollback: a mutation may have happened before its acknowledgement was blocked. The demo never automatically retries mutations.

Finite monthly tests use 65,536 remaining bytes and a 131,072-byte oversized payload. Policy inspection itself is metered. The outage test retains a finite monthly allowance, not a per-second byte limit.

## Execution and evidence

Node listener: `127.0.0.1:62519`; Engine: `127.0.0.1:28191`; isolated Registry: `127.0.0.1:62485`; disposable PostgreSQL port: 50679. Run ID: `9cd12b20-254d-4e5e-ac11-f9ba791c0067`.

The final process exited successfully and closed the Node server and SDK connection before the independent usage audit. No additional SDK traffic was sent afterward. [Current machine-readable results](evidence/node-sdk-incoming-rate-live-result.json) contain all fifteen groups.

The first attempt used sequential requests for the finite rate check and could cross a one-second boundary. The corrected test sends concurrent requests, records start/end timestamps, and retries at most three times only when those timestamps prove a boundary was crossed. A confirmed window still requires exactly two successes, one denial, and a durable counter of two. The final burst completed in **32 ms within one second** (`1789470722101`–`1789470722133` Unix milliseconds).

The SDK transport fixes remain unchanged. Their ten previously passing regression tests cover pending create/step/complete rejection on connection loss, explicit policy close reasons, HTTP upgrade and GraphQL failures, early handshake close, and normal success. Run `npm run test:connection-errors` in `threadify-sdk`.

The shared Registry runtime suite passed with PostgreSQL and the race detector.
A new streaming regression admits one request at a rate of one per second,
delivers all twenty SSE messages, reconciles their output bytes/message counts,
and confirms no outgoing rate bucket exists. The full Engine and API suites and
focused Registry contract/configuration tests also passed.

The disposable Engine, Registry fixture, and test containers were stopped after
the audit. The user's separate local Registry on port 59903 remains healthy.

## Independent usage audit

The final read-only audit passed **9/9 assertions** after SDK traffic stopped:

| Metric | Threadify | Registry |
|---|---:|---:|
| Input bytes | 825,674 | 825,674 |
| Input requests | 442 | 442 |
| Output bytes | 1,659,925 | 1,659,925 |
| Output messages | 420 | 420 |

Outgoing one-second rate buckets: **zero**. Pending outbox reports, legacy credit accounts, and billing snapshots: **zero**. The database held 53 threads and two profiles. Entitlement values were not persisted in the Engine database. Totals include the first attempt and the final successful run. See [current audit evidence](evidence/incoming-rate-accounting.json).

## Scope

All Engine workflow operations and archived queries use SDK methods. Direct HTTP controls only the isolated Registry fixture and inspects policy synchronization; `psql` reads persisted outcomes. Registry uses production licensing handlers and repository persistence, with a no-delivery identity stub and one-second heartbeat/three-second advertised freshness for test speed. The user's existing Registry at port 59903 and its database were not modified.

This covers the listed licensing paths, not every feature, limit combination, or production load condition. Storage capacity remains a database bound without commercial thread-count or per-thread-size quotas. No package was published or production service deployed.

## Historical evidence

The earlier five-allowance run included the now-retired outgoing rate limit. Its [report](evidence/historical-validation-before-incoming-only-rate.md), [results](evidence/node-sdk-count-rates-live-result.json), and [audit](evidence/count-rates-accounting.json) remain unchanged as historical evidence.

The preceding seven-allowance run also included retired byte-per-second limits; see [that historical report](evidence/historical-validation-before-count-only-rates.md). Neither historical run is presented as proof of the current four-allowance model.

See [README.md](README.md) for reproduction instructions.
