# JetStream traffic metering — 15 September 2026

This report records the initial implementation. A subsequent
[implementation review](JETSTREAM_REVIEW_2026-09-15.md) identified and corrected
local retry amplification, redundant reads and checkpoint work, and a startup race.

## Implementation

Incoming request rates and monthly input/output bandwidth now use file-backed
JetStream KV. One revision-conditional update admits a batch and records its
cumulative usage. PostgreSQL is no longer queried by traffic quota accounting.
The existing embedded broker is the default; external-broker deployments share
the same bucket across the installation's processes.

This follows Fused's use of a KV coordinator and asynchronous SQL persistence.
Threadify records exact observed usage per admission instead of reserving local
allowance leases. It polls the cumulative KV document every 250 ms for SQL
checkpointing; it does not introduce a separate event stream or consumer.
Intermediate KV revisions can be compacted without losing reporting deltas.
The SQL checkpoint, cumulative counters and reporting-outbox deltas commit
together, with revisions and row locking preventing duplicate projection.

Registry entitlements remain only in memory. Heartbeat failure retains the last
verified policy. Entity-profile caps remain part of the SQL transaction that
creates a profile. Authentication, forwarded-request replay protection, and
application persistence can still use PostgreSQL; this change removes only
traffic accounting from its hot path. Flow validation remains Valkey/Lua and Go.

## Failure and upgrade behavior

- A running meter continues admitting within its limits when PostgreSQL is
  unavailable. Unprojected usage remains in JetStream for recovery.
- A broker failure or missing/corrupt counter state denies traffic accounting.
  An uncertain acknowledgement can consume allowance despite returning an error;
  it is never retried automatically as a second debit.
- First startup imports previous SQL totals without reporting them again.
  Later startup refuses a missing KV binding or a broker snapshot behind the SQL
  checkpoint. PostgreSQL can lag admissions, so it cannot safely reconstruct lost
  broker state as a fresh allowance.
- Upgrade every process for an installation together. Old SQL-metering binaries
  do not coordinate with the new KV counters. Preserve and back up JetStream data
  alongside PostgreSQL. See [Registry integration](../REGISTRY_INTEGRATION.md).

## Benchmark method

Two runs use the same workload as the [earlier SQL optimization](WAIT_OPTIMIZATION_2026-09-15.md):
1,024 cycles each at 1, 8 and 32 concurrent flows, one shared SDK WebSocket,
approval validation, ready permission, then alternating success/failed validation.
Additional phases measure asynchronous acknowledgements, prerequisite wake-up,
and 256 missing-prerequisite timeouts per run. Each run submits 6,985 reports,
including warm-up and auxiliary phases. SDK assertions check decisions and
correlation; the runner also requires clean Engine shutdown.

The compiled Engine uses a signed local Registry fixture, real durable metering,
disposable PostgreSQL/Valkey and embedded NATS. Registry fixture limits are
unlimited during timing; finite caps are exercised separately in integration tests.
This is a local closed-loop measurement, not a whole-server capacity or WAN test.
The host is an Apple M4 with 10 CPUs and 16 GiB RAM, Node 22.11.0 and Go 1.26.0.
Existing developer services remain running. No other tests were launched by this
task during measurement; shared-host load is not controlled.

## Results

All figures are milliseconds unless labelled otherwise. SQL columns show the previous two optimized runs; JetStream columns show this change.

### Ready permission — median / p95 / p99

| Concurrent flows | SQL run 1 | SQL run 2 | JetStream run 1 | JetStream run 2 |
|---:|---:|---:|---:|---:|
| 1 | 2.5 / 6.9 / 11.5 | 2.5 / 8.1 / 24.5 | 1.5 / 6.0 / 15.3 | 2.0 / 7.6 / 22.4 |
| 8 | 24.1 / 50.0 / 120.6 | 28.5 / 102.1 / 298.5 | 24.8 / 64.2 / 93.6 | 29.2 / 82.1 / 180.2 |
| 32 | 108.6 / 186.2 / 455.7 | 115.1 / 325.4 / 2058.0 | 105.4 / 280.1 / 359.3 | 168.8 / 739.9 / 1080.9 |

### Success validation — median / p95 / p99

| Concurrent flows | SQL run 1 | SQL run 2 | JetStream run 1 | JetStream run 2 |
|---:|---:|---:|---:|---:|
| 1 | 7.3 / 18.7 / 29.3 | 7.5 / 19.7 / 40.2 | 8.8 / 35.6 / 67.7 | 12.8 / 41.8 / 77.5 |
| 8 | 32.3 / 69.0 / 118.3 | 37.6 / 142.7 / 255.9 | 34.0 / 82.1 / 117.8 | 40.3 / 120.8 / 221.8 |
| 32 | 115.1 / 226.8 / 285.6 | 131.5 / 420.2 / 497.6 | 124.8 / 319.6 / 388.1 | 165.8 / 756.7 / 1390.1 |

### Throughput — operations/sec

| Concurrent flows | SQL run 1 | SQL run 2 | JetStream run 1 | JetStream run 2 |
|---:|---:|---:|---:|---:|
| 1 | 145.9 | 125.7 | 102.2 | 78.0 |
| 8 | 222.3 | 155.7 | 204.4 | 159.6 |
| 32 | 233.2 | 159.3 | 226.3 | 122.2 |

### Auxiliary phases — JetStream runs

| Measurement | Run 1 median / p95 / max | Run 2 median / p95 / max |
|---|---:|---:|
| Asynchronous acknowledgement | 4.9 / 21.7 / 1775.3 | 5.6 / 13.4 / 27.2 |
| Prerequisite to permission, 1 flow | 9.9 / 45.9 / 175.6 | 9.2 / 19.4 / 28.6 |
| Prerequisite to permission, 8 flows | 35.6 / 179.0 / 303.4 | 35.7 / 101.2 / 145.5 |
| 100 ms timeout, 32 flows | 101.6 / 125.6 / 127.9 | 101.4 / 104.0 / 104.7 |

The deliberate 50 ms prerequisite hold is excluded from wake-up timings. All
512 missing-prerequisite waits across the two runs returned the expected timeout,
and subsequent work succeeded. Timeout notification still depends on Node's event
loop; it does not imply all server-side cancellation finishes within 100 ms.

Single-flow ready permission improved from about 2.5 ms median to 1.5–2.0 ms.
**End-to-end throughput did not consistently improve.** In particular, both
JetStream runs had lower single-flow throughput and slower success validation
than the preceding SQL runs. At 32 flows, JetStream throughput was 122–226 ops/sec
versus 159–233 previously; permission p95 was 280–740 ms versus 186–325 ms.
The second run was worse under this load. Long delays remain: run 1 had a
1.78-second asynchronous ACK and run 2 had a 1.38-second ready permission check.
Moving quota accounting to JetStream establishes independent durable admission;
it does not demonstrate that SQL metering caused the earlier tail spikes.
Profiling the remaining validation/queue/persistence work under controlled load
is still needed. Raw samples retain all phases and outliers.

## Verification

- Full Engine Go regression suite passed.
- Shared Registry integration suite passed under the race detector with real
  PostgreSQL and JetStream. Tests cover concurrent finite quotas, atomic rejection
  across metrics, monthly rollover, failed SQL projection, restart, missing broker
  state, stale backup rejection, and idempotent projection across independent runtimes.
- A closed PostgreSQL pool does not prevent KV admission within the exact quota;
  reconnecting and racing projection workers produces the exact reporting total.
- A simulated lost acknowledgement after a real KV update returns an error,
  performs one debit, retains that usage, and never retries it as a new increment.
- API Registry enforcement and archiver profile-cap integration tests passed.
- Compiled Engine plus the real Node SDK passed contract enforcement, unhappy
  paths, and restart recovery. Both complete performance runs passed.
- No SDK or validation behavior was changed by this migration.

## Artifacts

- [JetStream run 1](wait-2026-09-15-jetstream.json)
- [JetStream run 2](wait-2026-09-15-jetstream-repeat.json)
- [Previous SQL optimization and raw samples](WAIT_OPTIMIZATION_2026-09-15.md)
- [Original baseline](WAIT_PERFORMANCE_2026-09-15.md)

## Reproduction

From the repository root:

```sh
GOCACHE=/private/tmp/threadify-go-build \
  threadify-go/scripts/performance-wait.sh /private/tmp/threadify-jetstream-results.json
```

The runner creates and removes only its own disposable containers.
