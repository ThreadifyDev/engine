# Entity Profile workload — 20 September 2026

## Result

Passed against two compiled local Engines sharing managed Valkey, external JetStream and a disposable PostgreSQL database. Registry entitlements came from the integration fixture with unlimited quotas. No payment provider or model was called.

| Check | Verified result |
| --- | ---: |
| Concurrent SDK connections | 32 |
| Target entity runs | 1,024 |
| Archived target steps | 4,864 |
| Runs with a recorded approval violation | 256 |
| Failed refund attempts, followed by fresh approval and retry | 256 |
| Rejected invalid-content submissions | 1,024 |
| Missing/fresh approval waits that timed out as expected | 512 |
| Unrelated control runs excluded from profile | 32 |

All target runs ultimately completed. A violation remains in history after recovery. The violation count is the number of flagged step notifications, not the number of individual rule findings inside each notification. Invalid content was rejected before recording, so those submissions are not counted as archived steps.

The workload took 30.68 seconds, excluding setup and final verification. Permission checks had median 20.29 ms and p95 85.20 ms. Submission/validation round trips had median 117.53 ms and p95 195.11 ms; that distribution includes expected rejections and controls. These are local mixed-workload measurements, not a capacity or production latency promise.

Before reading cacheable metrics, the runner waited for PostgreSQL counts to match the expected runs, completion statuses, steps, violations and failed attempts. It then checked every target thread through GraphQL, resolved the automatically materialized profile, named it without changing its ID, and verified both fresh and cached metric responses. The homepage contains a clearly labelled snapshot, not a live connection to this test Engine.

Evidence: [exported profile and representative runs](../../homepage/public/examples/entity-profile-workload.json).

## Bugs found and fixed

- Per-thread notification queries used snake_case JSON keys while live archival wrote camelCase. Reads now recognize the actual payload names (and existing older payloads).
- Validation activity logs did not populate the notification projection used by global queries and profile metrics. The archive consumer now writes it before acknowledging the activity batch, retaining notification and step IDs. Database failures are returned for retry; notification IDs prevent duplicate projection rows on replay.
- A mixed reference batch could acknowledge references whose thread metadata had not yet arrived from another replica. Reference batches now return a retryable error and roll back until all their threads exist.

These changes were tested in isolated Engines. The installed Engine was not upgraded, and historical data was not backfilled or repaired.

## Repeat

Build the Engine and supply a **disposable** PostgreSQL database and a complete SDK checkout with installed Node dependencies. The runner uses Docker/psql for an independent archive check. Set `THREADIFY_PROFILE_PG_CONTAINER` if its container name differs from `threadifyengine-threadify_storage-1`.

From `threadify-go`, with `THREADIFY_SMOKE_POSTGRES_URL` already set securely:

```sh
go build -o /private/tmp/threadify-profile-engine ./cmd/server
export THREADIFY_SMOKE_BINARY=/private/tmp/threadify-profile-engine
export THREADIFY_SMOKE_MANAGED_VALKEY_BINARY="$HOME/.local/bin/libexec/valkey-server"
export THREADIFY_SMOKE_SDK_DIR=/absolute/path/to/complete/threadify-sdk
export THREADIFY_PROFILE_WORKLOAD_OUTPUT=/private/tmp/entity-profile-result.json
export THREADIFY_PROFILE_RUNS=1024
go test ./cmd/server -run '^TestTwoEnginesShareManagedValkey$' -count=1 -v -timeout 15m
```

The fixture creates temporary Engine configurations, credentials and Registry services. It shuts down the test Engines afterwards; the supplied database remains available for inspection. The output contains synthetic IDs and measured results, but no API key or database password.

## Validation

- Full 1,024-run compiled-Engine workload: passed.
- New reusable workload entry point, 32-run check: passed.
- PostgreSQL mixed-reference arrival regression and notification projection failure regression: passed.
- Full `internal/archiver`, `internal/repository/postgres`, and `internal/service` package tests: passed (environment-gated integration cases require their own opt-in configuration).
- Homepage TypeScript check, production build and 13 existing tests: passed.

## Preserved browser demonstration

The original workload database remains intact. A private PostgreSQL backup and a separate working database were created for the dashboard demonstration. The working copy retains the old test broker checkpoint under a separate table name and uses its own persistent JetStream state. No thread, step, profile or validation record was deleted.

The revised [71-second walkthrough](../../homepage/public/media/entity-profile-walkthrough.mp4) records actual in-app browser interactions: profile metric configuration, the failed-refund metric editor, metric results, delivery health, run history, and a recovered failed step with its context. It includes the simplified profile header. Captions and a cursor overlay following the recorded interaction positions make the tour easier to follow. No metric configuration was saved or changed during recording. Synthetic workload data remains preserved.

Live inspection also exposed two display/notification issues:

- Health scoring did not handle PostgreSQL numeric values. Numeric conversion and a new cache version produce 93.75 (displayed as 94, Healthy), rather than 55 (Degraded). The UI now calls eventual success **Completion Rate**. This formula does not score contract compliance; rule violations remain a separate metric.
- Grouped violations omitted their aggregate severity. New grouped notifications now inherit the highest finding severity. The dashboard includes unclassified historical notifications and no longer calls an empty notification response a successful validation. Existing records were preserved unchanged.

Validation: GraphQL and service package tests passed; three dashboard rendering regressions passed; the dashboard production build passed. The video and visible dashboard were checked against the preserved records.

## Hosted walkthrough

The homepage embeds the [Mux-hosted walkthrough](https://player.mux.com/qX87EoG4o8zpQULQJ3OPYJPGU2K2GATmFyCLzaLPPus). Playback ID: `qX87EoG4o8zpQULQJ3OPYJPGU2K2GATmFyCLzaLPPus`. Mux asset ID: `rjmc01V700yjJcVcI5qtE01q98YFwNMoJkn7rKDAkg9KGE`. The original local MP4, captions and poster are retained in `homepage/public/media/`.
