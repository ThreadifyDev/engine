# Live Registry integration validation

The historical runs below include the former seven-field contract. The current
contract has four allowances: monthly input/output bytes, incoming request
count per second, and entity profile count. Outgoing message rates and
byte-per-second limits have been removed. Current validation is recorded in
`examples/license-sdk-server/VALIDATION.md` at the repository root.

Date: 2026-09-15. The initial run below preceded the user's correction that
heartbeat failures must not interrupt Threadify. The updated runtime retains
last verified limits indefinitely; the old expiry-denial expectation is superseded.
All services, accounts, credentials and data were disposable
local test resources. No production deployment or external email delivery was
performed.

## Heartbeat continuity correction — passed

The updated compiled live test passed in 32.60 seconds. It kept HTTP requests and
WebSocket thread creation working throughout a four-second Registry outage,
beyond the fixture's former three-second cutoff. All 15 outage-created threads
persisted. After recovery, a new zero input allowance produced 429; restoring
the allowance restored access. Explicit suspension still denied access.

Registry and Engine totals matched exactly after delivery resumed: 4,477 input
bytes, 335 input requests/messages, 54,958 output bytes and 307 output
responses/messages. Zero reports remained pending. The rest of the live profile,
quota, suspension and reporting-outage restart scenarios also passed.

Shared race tests cover a year-old verified policy retaining its finite limits,
successful startup despite an initial heartbeat failure after a valid handshake,
and applying new limits upon recovery. The full Engine and API suites and focused
API/auth race tests passed. The fixture and disposable containers were cleaned
up afterward.

This correction keeps limits in memory only. A fresh process still needs one
successful handshake; heartbeat age never disables an already verified runtime.

## Service coverage

Threadify ran as a newly compiled Engine executable with embedded NATS, pgvector
PostgreSQL and Valkey. Registry ran its production provisioning, licensing,
heartbeat, reporting and authorization handlers with its cached PostgreSQL
repository in a local test harness. External identity provisioning used a
no-delivery stub. This was not a full production Registry deployment.

The normal restart test used unmodified Registry timing: a 60-second heartbeat
and 300-second verification grace period.

## Restart and accounting

`TestStandaloneBinaryPersistenceAndRestart` passed in 64.80 seconds initially,
then in 64.12 seconds after the live-discovered shutdown fix, with the compiled
Engine calling the real Registry handlers over HTTP. Both runs used fresh
disposable databases; the final run used the updated Fused product guard.

- License handshake and signed startup/periodic heartbeat succeeded.
- Five threads created over WebSocket persisted across a process restart.
- Reporting acknowledgement drained the durable outbox.
- Usage counters survived the restart and matched Registry exactly:

| Meter | Engine | Registry |
| --- | ---: | ---: |
| Input bytes | 439 | 439 |
| Input requests/messages | 7 | 7 |
| Output bytes | 845 | 845 |
| Output responses/messages | 6 | 6 |

Database inspection confirmed five persisted threads, zero credit accounts,
zero pending reports, and no entitlement/limit/license columns in the
`threadify_registry_*` tables.

Test entry point: `cmd/server/main_integration_test.go`. Build with
`go build -o /tmp/threadify-registry-live ./cmd/server`, then supply
`THREADIFY_SMOKE_BINARY`, `THREADIFY_SMOKE_POSTGRES_URL`,
`THREADIFY_SMOKE_VALKEY_ADDR` and `THREADIFY_SMOKE_REGISTRY_URL` and run:

```sh
go test ./cmd/server -run '^TestStandaloneBinaryPersistenceAndRestart$' -count=1 -v -timeout=180s
```

The Registry harness must use the documented fixed smoke account and test key;
the PostgreSQL database and Valkey instance must be fresh disposable services.

## Registry provisioning and authorization

The retained HTTP driver passed 39 checks against the updated Registry handlers:
Fused-only, Threadify-only and combined provisioning; both products resolving the
same account from the same license; protected/public access; signed heartbeat;
all four usage meters; idempotent retries; conflicting-report rejection; and
independent product suspension and restoration.

A separate live probe exposed a missing account-wide suspension condition in
the new Fused product guard. After the fix, a globally suspended combined
account receives 403 from both products, and restoring the account restores
Fused access. The regression also has a permanent middleware test.

The repeatable Registry harness, instructions and HTTP driver are retained in
the Fused repository under `backend/testutil/threadifyregistry/`. It binds only
to loopback. Its protected probe uses the real middleware chain; it is not the
complete Fused catalogue or GraphQL implementation.

Six further live checks verified key revocation and restoration: revocation
returned 200, the revoked key received Threadify 401 and Fused 403, restoration
returned 200, and the restored key received Threadify 200 and Fused 204. Together
with the three global-suspension/restoration assertions, 48 Registry HTTP checks
passed.

## Live failure and recovery

`TestLiveRegistryFailureScenarios` passed in 27.23 seconds against the rebuilt
Engine and updated Registry handlers. This scenario used the fixture's explicitly
documented 1-second heartbeat / 3-second grace adapter to accelerate failures;
authentication, entitlements, database writes and Engine transports remained real.

- All seven Registry limits were enforced, including HTTP/WS input denial and
  empty 429 responses when output allowance was exhausted.
- At profile cap 1, both threads and their references persisted while only one
  profile materialized. A heartbeat upgrade to cap 2 enabled the second profile.
- Product suspension blocked requests; restoration re-enabled them.
- The initial run tested denial after Registry grace expiry. This behavior was
  subsequently removed: the updated test requires uninterrupted HTTP/WS traffic
  beyond that interval, then verifies new limits apply when Registry recovers.
- A reporting-only outage retained usage and accepted threads across an immediate
  process restart. Restoring delivery drained the outbox.
- Rejected WebSocket mutations remained absent after restart. No credit account
  was created.

Final Engine counters matched Registry's reports for that installation exactly:
4,034 input bytes, 318 input requests/messages, 43,833 output bytes and 292 output
responses/messages. Four accepted threads and two profiles remained; zero usage
reports were pending.

The immediate restart exposed a persistence shutdown race: dependent records
could flush before their thread metadata. Shutdown now waits for received
metadata batches before flushing dependent records, and retries an in-flight
missing-metadata failure once after that dependency completes. It retains the
shutdown deadline, reports real database failures and preserves unread broker
backlog for durable replay. Four deterministic regressions passed with the race
detector; the full Engine suite and both live restart tests passed afterward.

The live test also distinguishes diagnostic health availability from readiness:
temporary persistence-readiness 503 responses must contain the real health JSON;
commercial limits must not turn the probe into a quota rejection.

The opt-in test is retained in `cmd/server/registry_live_integration_test.go`.
Supply all `THREADIFY_LIVE_` settings listed at its entry point, using fresh
disposable services and the Registry fixture's failure-test account, then run:

```sh
go test ./cmd/server -run '^TestLiveRegistryFailureScenarios$' -count=1 -v -timeout=360s
```

Both Registry fixtures were stopped and the disposable PostgreSQL/Valkey
containers and test volumes were removed after final reconciliation. The
repeatable harness and regression tests remain in the repositories.

## Node server and JavaScript SDK validation

The additional Node integration is retained in
`examples/license-sdk-server` at the repository root. It uses the local
`@threadify/sdk` package for WebSocket mutations and GraphQL reads, with a real
compiled Engine, PostgreSQL, Valkey and the production Registry licensing
handlers. The Registry fixture substitutes local identity provisioning; this
does not test hosted identity delivery or a deployed Registry.

This path exposed an SDK transport bug: create, step and completion promises
could remain pending after a quota-driven WebSocket close. Pending operations
now reject on transport closure/error. Engine policy closes carry bounded,
sanitized reasons, and the SDK preserves allowance errors as 429 and unavailable
license/accounting errors as 503. Unknown disconnects remain transport errors.
No mutation is automatically retried: an output denial can occur after a
mutation was accepted but before its acknowledgement was delivered.

The Engine close-frame regression uses real sockets and passed with the race
detector. The full Engine Go suite also passed after this change. See the
example's README and validation report for the SDK commands and live results.
