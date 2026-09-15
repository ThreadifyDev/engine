# Synchronous wait optimization — 15 September 2026

## Changes

- Combine each WebSocket input request/byte pair and output message/byte pair into one PostgreSQL transaction. Batch the SQL statements into one exchange. Conditional upserts enforce quotas using the database's unique-row locks, replacing separate advisory-lock, insert and update round trips.
- Keep monthly/request counters and every corresponding reporting-outbox entry atomic. Quotas still come from the in-memory Registry snapshot. Concurrent replicas cannot overrun the allowance. No schema migration or configuration change is required.
- Apply the SDK caller deadline without the former additional one-second transport grace. Check the monotonic clock when a response arrives as well as in the timer, so a late response cannot grant permission after an event-loop stall. Timeout requests cancellation; it does not undo an already granted invocation or accepted event.
- Start the Engine budget when a frame is read, before accounting. Preserve report idempotency keys on uncertain errors for reconciliation. Request execution order and validation rules remain unchanged.

## Results

Two full runs use the same workload, payload, configuration and machine as the [baseline](WAIT_PERFORMANCE_2026-09-15.md). Each run measures 3,072 cycles, with 1,024 permission checks per concurrency and 512 success and 512 failed-report validations per concurrency. All assertions and graceful shutdown passed. Each run submitted 6,985 reports in total, including warm-up and auxiliary scenarios.

### Ready permission (milliseconds)

| Concurrent flows | Baseline median / p95 / p99 | Optimized run 1 | Optimized run 2 |
|---:|---:|---:|---:|
| 1 | 7.7 / 20.6 / 35.8 | 2.5 / 6.9 / 11.5 | 2.5 / 8.1 / 24.5 |
| 8 | 66.5 / 125.9 / 198.8 | 24.1 / 50.0 / 120.6 | 28.5 / 102.1 / 298.5 |
| 32 | 263.7 / 412.6 / 558.1 | 108.6 / 186.2 / 455.7 | 115.1 / 325.4 / 2058.0 |

### Success validation (median / p95 / p99 in milliseconds)

| Concurrent flows | Baseline | Optimized run 1 | Optimized run 2 |
|---:|---:|---:|---:|
| 1 | 12.6 / 30.3 / 42.8 | 7.3 / 18.7 / 29.3 | 7.5 / 19.7 / 40.2 |
| 8 | 78.0 / 139.9 / 191.3 | 32.3 / 69.0 / 118.3 | 37.6 / 142.7 / 255.9 |
| 32 | 261.9 / 438.8 / 534.4 | 115.1 / 226.8 / 285.6 | 131.5 / 420.2 / 497.6 |

### Throughput (operations/sec)

| Concurrent flows | Baseline | Optimized run 1 | Optimized run 2 |
|---:|---:|---:|---:|
| 1 | 74.3 | 145.9 | 125.7 |
| 8 | 94.6 | 222.3 | 155.7 |
| 32 | 104.4 | 233.2 | 159.3 |

### Prerequisite submission to permission (median / p95 / p99 in milliseconds)

The deliberate 50 ms hold before prerequisite submission is excluded.

| Concurrent flows | Baseline | Optimized run 1 | Optimized run 2 |
|---:|---:|---:|---:|
| 1 | 14.7 / 41.4 / 69.1 | 10.1 / 74.6 / 724.2 | 10.3 / 25.0 / 31.5 |
| 8 | 60.4 / 115.8 / 131.7 | 44.2 / 207.1 / 251.8 | 27.2 / 50.2 / 79.5 |

### Caller timeout at 32 concurrent waits

All 256 missing-prerequisite waits per run returned the expected timeout error, and subsequent real work on the connection succeeded.

| Metric | Baseline | Optimized run 1 | Optimized run 2 |
|---|---:|---:|---:|
| median | 260.4 ms | 100.6 ms | 100.8 ms |
| p95 | 492.4 ms | 106.6 ms | 108.6 ms |
| p99 | 573.5 ms | 106.6 ms | 111.0 ms |
| max | 605.9 ms | 107.8 ms | 111.3 ms |

The budget was 100 ms. This improvement changes the caller deadline semantics: the SDK now stops waiting instead of granting a further second for a server reply. These numbers do **not** mean all server-side work or cancellation has finished within that time. Node event-loop stalls can still delay notification to the application. Recover uncertain permissions with their invocation ID; preserve the original payload and idempotency key for uncertain reports.

## Remaining latency issue

Median permission latency improved consistently (about 7.7 ms to 2.5 ms at one flow). At 32 flows, throughput increased from 104 to 159–233 operations/sec. Permission p95 improved from 413 to 186–325 ms.

**Tail latency is not resolved.** Run 1 had a 1.92-second prerequisite wake-up; run 2 had a 3.11-second ready check and a 3.44-second approval validation. Run 2's failed-report p95 at 32 flows was 537 ms versus the baseline's 408 ms. The spikes moved between phases. These tests do not isolate whether the cause is shared-host load, accounting/archival work, or another bottleneck; no production latency guarantee follows from the median improvements. A trace during a spike is the next diagnostic step. Both full runs are retained, including the worse tails.

The earlier background archival retry issue was outside these changes. This optimization does not claim to fix it.

## Verification

- Engine regression suite: passed.
- All 42 SDK tests: passed, including late-response rejection, blocked event loop, cancellation, exact-event correlation, and preservation of report recovery keys.
- PostgreSQL Registry tests under the Go race detector: passed. New coverage races 40 admissions from separate runtimes against finite limits, reverses batch order, verifies rollback across metrics, tests storage failure after partial writes, and rejects integer overflow. Existing rate windows, restart, reporting acknowledgement and entity-profile cap tests also passed.
- Handler/service race tests: passed, including input-accounting time in the Engine deadline.
- Real Valkey claim/subscription race tests: passed, including competing invocations, fresh prerequisites and cleanup.
- Compiled Engine + real Node SDK: unhappy paths and restart recovery passed.
- Two complete final performance runs: passed; 512 expected timeout cases in total.
- `git diff --check` in Engine and SDK repositories: passed.

## Artifacts and reproduction

- [Baseline raw samples](wait-2026-09-15.json)
- [Optimized run 1](wait-2026-09-15-optimized.json)
- [Optimized run 2](wait-2026-09-15-optimized-repeat.json)

Run from the repository root:

```bash
GOCACHE=/private/tmp/threadify-go-build \
  threadify-go/scripts/performance-wait.sh /private/tmp/threadify-wait-results.json
```

The runner uses a signed local Registry fixture with unlimited entitlements, real durable accounting, disposable PostgreSQL/Valkey and embedded NATS. It removes only its own containers. Tests used a single shared WebSocket and a closed-loop workload on an Apple M4 with existing developer services still running. No additional test workload ran alongside the two final benchmarks. See the baseline report for detailed methodology and limitations. Changes are local and uncommitted.
