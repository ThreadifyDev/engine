# Synchronous SDK wait performance — 15 September 2026

## Result

The single-flow path is approximately 8 ms median for permission and 13 ms for synchronous success validation. Queuing on one shared WebSocket substantially increases latency: at 32 concurrent flows, p95 is 413 ms for permission and 439 ms for success validation. This run does not establish production capacity or a latency guarantee.

All SDK assertions passed. The measured profiles contain 3,072 flow cycles (9,216 measured operations), plus baseline, wake-up and timeout checks. Including warm-up and auxiliary checks, the SDK submitted 6,985 step reports across 433 threads. All 6,985 were present in PostgreSQL after graceful shutdown, and latest successful contexts were complete and current.

## Ready-flow latency

Each cell is **median / p95 / p99 in milliseconds**. One operation is an approval report, a permission check, or a charge report. Charge reports alternate success and failure; both must pass the content and fresh-prerequisite rules.

| Concurrent flows | Permission | Success validation | Failed-report validation | Operations/sec |
|---:|---:|---:|---:|---:|
| 1 | 7.7 / 20.6 / 35.8 | 12.6 / 30.3 / 42.8 | 12.7 / 33.4 / 52.3 | 74.3 |
| 8 | 66.5 / 125.9 / 198.8 | 78.0 / 139.9 / 191.3 | 81.5 / 146.1 / 306.9 | 94.6 |
| 32 | 263.7 / 412.6 / 558.1 | 261.9 / 438.8 / 534.4 | 277.1 / 408.5 / 697.6 | 104.4 |

Each concurrency level contains 1,024 permission measurements, 1,024 approval validations, 512 success validations and 512 failed-report validations. Complete approval statistics and raw samples are in [the JSON results](wait-2026-09-15.json).

## Other paths

| Scenario | Concurrency | Samples | Median ms | p95 ms | p99 ms | Max ms |
|---|---:|---:|---:|---:|---:|---:|
| async-baseline | 1 | 256 | 7.8 | 14.7 | 30.0 | 34.2 |
| blocked-wakeup | 1 | 128 | 14.7 | 41.4 | 69.1 | 129.8 |
| blocked-wakeup | 8 | 128 | 60.4 | 115.8 | 131.7 | 136.6 |
| missing-prerequisite-timeout | 32 | 256 | 260.4 | 492.4 | 573.5 | 605.9 |

- **Async baseline:** ordinary SDK report acknowledgement. Exact validation is awaited afterward, outside the measured interval. This is a separately sampled baseline, not a paired measurement of the synchronous overhead.
- **Blocked wake-up:** permission request sent first; after a deliberate 50 ms hold, time is measured from prerequisite submission to permission returned. The 50 ms hold is excluded. The test does not instrument the exact instant the server subscription starts.
- **Timeout:** all 256 missing-prerequisite waits returned the expected timeout error. A 100 ms Engine budget took as much as 606 ms at the SDK under 32-way queuing. The budget is not a strict caller-visible deadline; queue time and response delivery add latency. The same connection successfully validated and granted another invocation afterward.

## Findings

1. **Shared-connection queuing warrants optimization.** Throughput rises only from 74 to 104 operations/sec while concurrency rises from 1 to 32. The handler processes incoming commands serially before deferring pending responses (`internal/handlers/thread.go`). This is consistent with request serialization contributing to the latency, but a CPU/trace profile is needed to attribute the exact cost. These are single-connection workload results, not whole-Engine maximum throughput.
2. **Timeout budgets include a practical overrun at the caller.** Applications should account for this before treating a short timeout as a hard deadline.
3. **Archival ordering produces retries under load.** A partial Engine log snapshot recorded 185 missing-thread-metadata retries and 87 successful-context foreign-key retries. All benchmark step rows and latest successful contexts were present after shutdown. The retry noise remains a separate issue; successful SDK responses do not imply error-free background processing.
4. **No pending validation barriers or wait Pub/Sub channels remained after shutdown.** This is an end-of-run check, not a long-duration leak test.

## Method and limits

- Actual source SDK over one persistent WebSocket to a compiled native Engine, with real PostgreSQL and Valkey in disposable Docker containers and embedded NATS. No mocked validation path.
- Apple M4, 10 CPU cores, 16 GiB host RAM. Docker had 10 CPUs and about 7.65 GiB RAM. Go 1.26.0; Node 22.11.0. Existing developer services stayed running; CPU and network were not isolated.
- `config.selfhost.yaml` plus `subscription.selfhost.yaml`; info logging, performance-monitoring logs disabled, normal validation and persistence workers enabled. The standalone harness sets archival block and step-state flush intervals to 100 ms.
- Signed local Registry fixture grants unlimited request, bandwidth and profile allowances. Normal runtime checks/metering stay enabled. This measures runtime latency, not plan throttle behavior or the live Registry service.
- Three-rule Gherkin contract: approval, charge requiring a fresh successful approval and positive amount, and an unused terminal finish step. Each report contains `amount: 1` and a 256-character ASCII payload, plus normal SDK metadata/protocol overhead.
- Four warm-up cycles per worker, then 1,024 measured cycles at each concurrency. Threads rotate after 32 measured cycles, keeping history sizes comparable. Thread creation is excluded from operation latency and throughput timing. Threads remain attached to the shared connection during the run.
- Closed-loop workload: each worker waits for its previous operation before issuing the next. This does not simulate an unbounded arrival rate, WAN latency, long histories, large contracts/payloads, many independent clients, or a soak test. Profiles ran sequentially in ascending concurrency order; no repeated-trial confidence intervals were calculated.
- Engine binary SHA-256 and repository revisions are recorded in the JSON. The binary includes uncommitted workspace changes. No production implementation changes were made for this benchmark.
- The first setup attempt used a contract that automatically completed after charge. That fixture was corrected by adding an explicit terminal finish step before the measured run; the failed setup contributed no latency samples.

## Reproduce

Requires Go, Node, installed SDK dependencies and Docker with PostgreSQL 16 Alpine and Valkey 9 Alpine images available. From the repository root:

```bash
GOCACHE=/private/tmp/threadify-go-build \
  threadify-go/scripts/performance-wait.sh /private/tmp/threadify-wait-results.json
```

The runner builds the Engine, creates disposable containers on random loopback ports, provisions a test API key and Gherkin contract, runs the SDK workload, writes raw JSON and a test log, and removes only its own containers. `THREADIFY_PERF_SAMPLES` can change ready-profile sample count (default 1,024; use multiples of 32).

Benchmark code: `threadify-sdk/tests/performance-wait.mjs`. The opt-in `THREADIFY_PERF_OUTPUT` branch reuses the existing standalone integration harness.

The reusable runner was also verified separately with 64 ready cycles per concurrency. Its complete SDK suite and cleanup passed; those smaller-run timings are excluded from the tables above.
