# JetStream implementation review — 15 September 2026

## Findings and fixes

1. **Local retry amplification.** Every admission fetched the shared KV document
   and raced to replace it. Callers within one process repeatedly invalidated each
   other's revisions. At 32 concurrent callers the isolated benchmark measured
   25.9–27.3 reads and 25.9–27.3 write attempts per successful admission. Fused
   serializes access locally; the first Threadify implementation omitted this.
   The corrected meter queues local writers with cancellation support.

2. **Repeated reads and decoding.** Every admission fetched and decoded the entire
   cumulative document. The meter now reuses the last acknowledged state/revision.
   Every accepted debit still requires a successful conditional JetStream write.
   Another process's write causes a conflict and authoritative reload. Any
   uncertain acknowledgement clears the cache. Rejected batches mutate only a
   copy, preserving atomicity. No local allowance leases or unrecorded admissions
   are introduced.

3. **Unnecessary background SQL work.** Every checkpoint issued separate SQL
   reads/writes for every retained month, including unchanged counters. Projection
   now reads prior totals once, skips unchanged totals, and batches changed
   counters, outbox deltas and checkpoint revision within the same transaction.

4. **Startup/checkpoint race.** Startup could read KV, then compare it with a SQL
   checkpoint advanced by a concurrent projector and incorrectly reject valid
   broker state. Startup now locks the checkpoint row through its KV read and
   consistency checks. This never adds a SQL lock to traffic admission.

## Isolated metering benchmark

Two repetitions of 1,000 successful admissions at each concurrency, using real
JetStream and PostgreSQL for startup. Each admission counts one request and 256
input bytes. SQL projection and application validation are outside this benchmark;
the purpose is to isolate admission and count broker operations. Both versions use
the same disposable broker. Startup is excluded. No other tests from this task ran
alongside these measurements.

| Callers | Before, ms/op | Corrected, ms/op | Before reads/writes per op | Corrected reads/writes per op |
|---:|---:|---:|---:|---:|
| 1 | 1.29–3.12 | 0.23–0.31 | 1 / 1 | 0 / 1 |
| 8 | 2.13–4.12 | 0.23–0.27 | 7.5–7.8 / 7.5–7.8 | 0 / 1 |
| 32 | 2.17–2.59 | 0.27–0.31 | 25.9–27.3 / 25.9–27.3 | 0 / 1 |

`ms/op` is elapsed time divided by completed operations (inverse throughput), not
each caller's queue-inclusive latency at concurrent loads. The corrected runs had
zero local CAS conflicts. Independent processes can still conflict, and the shared
key still limits aggregate write throughput. The eliminated retry amplification is
directly measured; it does not establish the cause of every full-workflow outlier.

Raw measurements: [before](jetstream-meter-before.txt), [corrected](jetstream-meter-after.txt).

## Verification

- Shared Registry suite passed with the Go race detector and real PostgreSQL/NATS.
- Regression coverage asserts one durable write per local admission, cancellation
  while queued, and exact finite quotas across separate Runtime instances with
  independent caches and writer queues.
- A controlled startup/projector race verifies that a newer SQL checkpoint cannot
  overtake startup's KV snapshot. A later admission refreshes the stale cache via CAS.
- Existing failed projection, lost acknowledgement, restart, missing/stale broker state,
  calendar rollover, suspension and heartbeat-outage coverage remains passing.
- Full Engine regression suite and API Registry integration tests passed.
- Compiled Engine plus the real Node SDK passed enforcement and restart recovery.

The original performance report is retained at
[JetStream metering](JETSTREAM_METERING_2026-09-15.md).

## Full SDK benchmark after correction

Two full runs repeat the original 1,024-cycle-per-concurrency workload, including
6,985 submitted reports and 256 expected timeout cases per run. Both completed
with all SDK assertions and graceful shutdown passing. Disposable PostgreSQL,
Valkey and embedded NATS were used; existing developer services remained running.
No other tests were launched by this task during measurement.

### Ready permission — median / p95 / p99, milliseconds

| Flows | Initial run 1 | Initial run 2 | Corrected run 1 | Corrected run 2 |
|---:|---:|---:|---:|---:|
| 1 | 1.5 / 6.0 / 15.3 | 2.0 / 7.6 / 22.4 | 0.7 / 2.0 / 3.1 | 1.3 / 3.4 / 6.7 |
| 8 | 24.8 / 64.2 / 93.6 | 29.2 / 82.1 / 180.2 | 15.8 / 40.1 / 95.7 | 18.6 / 37.9 / 92.3 |
| 32 | 105.4 / 280.1 / 359.3 | 168.8 / 739.9 / 1080.9 | 58.5 / 88.7 / 109.9 | 81.2 / 146.7 / 196.0 |

### Success validation — median / p95 / p99, milliseconds

| Flows | Initial run 1 | Initial run 2 | Corrected run 1 | Corrected run 2 |
|---:|---:|---:|---:|---:|
| 1 | 8.8 / 35.6 / 67.7 | 12.8 / 41.8 / 77.5 | 5.0 / 11.9 / 17.1 | 7.7 / 18.8 / 28.5 |
| 8 | 34.0 / 82.1 / 117.8 | 40.3 / 120.8 / 221.8 | 22.4 / 53.7 / 110.4 | 25.9 / 50.8 / 153.7 |
| 32 | 124.8 / 319.6 / 388.1 | 165.8 / 756.7 / 1390.1 | 64.6 / 92.0 / 97.9 | 93.9 / 183.1 / 271.8 |

### Throughput — operations/sec

| Flows | Initial run 1 | Initial run 2 | Corrected run 1 | Corrected run 2 |
|---:|---:|---:|---:|---:|
| 1 | 102.2 | 78.0 | 227.9 | 148.7 |
| 8 | 204.4 | 159.6 | 306.1 | 283.4 |
| 32 | 226.3 | 122.2 | 484.7 | 323.3 |

### Auxiliary phases — corrected runs

| Measurement | Run 1 median / p95 / max, ms | Run 2 median / p95 / max, ms |
|---|---:|---:|
| Async ACK | 1.4 / 3.3 / 7.0 | 2.4 / 7.7 / 18.1 |
| Prerequisite to permission, 1 flow | 5.6 / 13.2 / 77.7 | 5.8 / 15.0 / 55.0 |
| Prerequisite to permission, 8 flows | 18.2 / 34.5 / 43.0 | 22.2 / 60.0 / 107.6 |
| 100 ms timeout, 32 flows | 100.9 / 103.3 / 103.7 | 101.4 / 104.3 / 105.4 |

The corrected runs improve ready-permission latency and workflow throughput over
both initial JetStream runs. Results still vary with shared-host load. These are
closed-loop measurements over one SDK WebSocket, not production latency bounds
or whole-server capacity. Local serialization also does not remove cross-process
CAS contention. The microbenchmark directly measures the eliminated retry work;
these full-workflow runs do not isolate the contribution of each individual fix.
The 100 ms timeout measures caller notification, not completion of server cleanup.

The corrected standalone test runs finished in 46 and 58 seconds, versus 92 and
107 seconds for the initial implementation. Long-running behavior across repeated
Registry reporting cycles is not established by these short performance runs;
Registry delivery and failure semantics have separate integration coverage.

- [Corrected run 1 raw samples](wait-2026-09-15-jetstream-reviewed.json)
- [Corrected run 2 raw samples](wait-2026-09-15-jetstream-reviewed-repeat.json)

## Reproduction

From the repository root, run the full SDK benchmark:

```sh
GOCACHE=/private/tmp/threadify-go-build \
  threadify-go/scripts/performance-wait.sh /private/tmp/threadify-reviewed.json
```

For isolated metering, enter `threadify-go/shared`, set
`THREADIFY_REGISTRY_TEST_DATABASE_URL` and `THREADIFY_REGISTRY_TEST_NATS_URL` to
disposable services, then run:

```sh
go test ./registry -run '^$' -bench '^BenchmarkJetStreamAdmission$' -benchtime=1000x -count=2
```
