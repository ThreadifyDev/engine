# Historical validation — superseded outgoing rate model

This records the completed five-allowance run before the user removed the outgoing response/message rate limit. Its JSON evidence is preserved unchanged.

# Node SDK licensing validation

## Current model: rates count requests and responses

Validated 15 September 2026 against the rebuilt five-allowance Engine and Registry. **16/16 live scenario groups passed.** The real Node HTTP server used local **@threadify/sdk 0.1.24**, connected to the compiled Engine, PostgreSQL/Valkey, and a separate Registry licensing handler fixture.

The five allowances are input monthly bandwidth in bytes, output monthly bandwidth in bytes, input requests per second, output responses/messages per second, and entity profile count. Rates do not impose a byte-per-second throttle.

| Allowance or flow | Passing checks |
|---|---|
| SDK lifecycle | Create → step success → complete; persisted metadata and successful step read through SDK GraphQL |
| Input monthly bandwidth | Zero rejects SDK WS and GraphQL requests; finite remaining allowance permits a small request and denies a larger payload |
| Output monthly bandwidth | Zero denies SDK WS acknowledgement and GraphQL output; finite remaining allowance permits a small response and denies a larger response |
| Input request rate | Zero rejects; two requests per second permits two operations and denies the third |
| Output response rate | Zero rejects; two messages per second permits two responses and denies the third |
| Large payload with count rates | 128 KiB input and output succeed under two requests/responses per second when monthly bandwidth is available |
| Entity profile limit | Zero preserves threads and refs without profiles; upgrades to one and two materialize the corresponding number |
| Explicit suspension and revocation | Each denies SDK mutations and recovers after restoration |
| Registry outage | 13 small SDK creates continue beyond the former grace interval using a finite monthly bandwidth allowance; oversized input is still denied; recovery applies new limits |

Every zero transport-limit case covered both SDK WebSocket and SDK GraphQL behavior, followed by recovery. Input rejection also checked that the database thread count did not increase. Output denial does not imply that the mutation was rolled back: it may have succeeded before its acknowledgement was blocked. The demo never automatically retries mutations.

Finite monthly tests use 65,536 remaining bytes and a 131,072-byte oversized payload. Policy synchronization requests are themselves metered. The outage test similarly retains a finite monthly input allowance, rather than a per-second byte limit.

## Execution and evidence

The test's Node listener was `127.0.0.1:51995`, Engine `127.0.0.1:28191`, and isolated Registry `127.0.0.1:62484`. The disposable Engine database used port 61189. Run ID: `1e1c1e78-273a-492c-9399-262b5839eb67`. The test process exited successfully and closed its Node server and SDK connection before the independent usage audit. No additional SDK traffic was sent afterward.

[Current machine-readable results](node-sdk-count-rates-live-result.json) record all 16 groups. The SDK transport error fixes remain unchanged; their previously completed 10 regression tests are described in the historical report and runnable with `npm run test:connection-errors` in `threadify-sdk`.

## Independent usage audit

The final read-only audit passed **8/8 assertions** after SDK traffic stopped:

| Metric | Threadify | Registry |
|---|---:|---:|
| Input bytes | 548,355 | 548,355 |
| Input requests | 313 | 313 |
| Output bytes | 588,130 | 588,130 |
| Output messages | 280 | 280 |

There were 43 persisted threads and two profiles, zero pending outbox reports, zero legacy credit accounts, and zero billing snapshots. The audit also confirmed that entitlement values were not persisted in the Engine database. See [current accounting evidence](count-rates-accounting.json).

## Scope

All Engine workflow operations and archived queries use the SDK. Direct HTTP calls control only the isolated Registry fixture and inspect Engine policy synchronization; `psql` reads persisted outcomes. Registry uses its production licensing handlers and repository, with a no-delivery identity stub and one-second heartbeat/three-second advertised freshness for test speed. The user's existing local Registry at port 59903 and its database were not changed.

This covers the listed licensing paths, not every product feature, combination of limits, or production load condition. Storage capacity remains a database bound, without commercial thread-count or per-thread-size quotas. No package was published or production service deployed.

## Preserved historical evidence

The earlier 19/19 run included byte-per-second restrictions, which were subsequently removed at the user's request. Its report and JSON evidence are preserved unchanged as historical results, **not evidence of the current five-allowance model**:

- [Historical report](historical-validation-before-count-only-rates.md)
- [Historical Node results](node-sdk-live-result.json)
- [Historical accounting audit](accounting.json)
- [Historical Registry allowance audit](registry-allowances.json)

See [README.md](../README.md) for reproduction instructions.
