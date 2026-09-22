# Standalone binary E2E

`make test-e2e` in `threadify-go` builds the engine and runs the tests with
disposable PostgreSQL/Valkey and embedded NATS. The Registry fixture is local.
Prepare the static dashboard first using [the dashboard build instructions](../../../web/README.md),
or set `THREADIFY_E2E_BINARY` to a release binary. CI supplies the dashboard artifact.

`TestEmbeddedDashboard` checks static assets, deep-link refreshes, API route
isolation, same-origin cookie sign-in, authenticated GraphQL and CSRF rejection.
It also exercises profile-type configuration, profile/company updates, API keys,
service accounts, immediate key revocation, and management permission checks. Workflow tests verify that company users can
open contracts created by a different service identity.
Frontend route matching and session clients are also tested in `web/tests`.


Disposable top-level scenarios run concurrently, with separate services and data.
CI uses `-parallel=3`; pass `-parallel=1` to limit local resource use. Existing
instances selected with `THREADIFY_LIVE_DIR` and runs writing to an explicit
`THREADIFY_E2E_EVIDENCE_FILE` remain sequential.

To run only the multi-system test against a compiled binary, from `threadify-go/tests`:

```sh
THREADIFY_E2E_BINARY=/absolute/path/to/threadify \
go test ./e2e -run '^TestStandaloneMultiSystemJoin$' -v -count=1 -timeout=5m
```

`TestStandaloneMultiSystemJoin` creates two standard service identities with
separate expiring API keys and independent WebSocket connections. It covers:

- Direct same-company joining, repeated joining and reconnecting.
- Joining with an invitation token; rejection of an invalid token.
- Alternating Orders/Warehouse contributions to one thread (four steps).
- Both connections writing concurrently to one thread (five steps each).
- Explicit completion, Postgres persistence of both participants, service-account
  and service-name attribution, and integrity verification from both identities.

The original `TestStandaloneMultiSystemJoin` uses contract-free threads. It does
not simulate separate OS processes, cross-company access, or distributed OTEL
context propagation. Contract and OTEL coverage lives in `TestStandaloneWorkflows`.

For an existing **local** combined engine, set `THREADIFY_LIVE_DIR` to its config
directory instead. `THREADIFY_E2E_COMPANY_ID` optionally selects an existing test
company. This creates persistent test threads and service accounts; generated
keys are revoked after the test. A passing live run saves non-secret IDs in
`multi-system-verification.json` in that directory. Do not run against production.

The repeated-join persistence regression also uses real Postgres:

```sh
# From threadify-go, using an explicitly supplied test database:
THREADIFY_TEST_DATABASE_URL='<test-dsn>' \
go test ./internal/archiver -run '^TestWriteThreadAccess_RepeatedJoins$' -v
```

That regression creates and drops a private schema. It checks duplicate updates,
multiple threads/participants, latest-state preservation and batch retries.

## Contract-based multi-system joins

```sh
# From threadify-go/tests, using the compiled binary containing the join fixes:
THREADIFY_E2E_BINARY=/absolute/path/to/threadify \
go test ./e2e -run '^TestStandaloneContractMultiSystemJoin$' -v -count=1 -timeout=5m
```

This companion test uses an additional temporary admin setup identity solely to
create two contracts through the engine API. Orders and Warehouse retain separate
standard service accounts. All temporary keys are revoked, and the setup identity's
admin role is removed afterwards.

- The sequential contract assigns `order_received` and `dispatch_requested` to
  Orders, and `inventory_reserved` and `dispatched` to Warehouse. It is tested
  with both direct and invitation-token joins, repeated joining and reconnects.
- Invalid contract-party roles and Warehouse writing an Orders-owned step must
  be rejected before they create recorded events.
- The parallel contract starts with Orders, runs four operations per system
  concurrently, then finishes with Warehouse: ten total steps, five per system.
  Each operation uses `depends_on: [order_received]`; `dispatched` depends on
  all eight operations. Explicit `transitions` enforce immediate sequence and
  are intentionally omitted from this partial-order contract.
- The terminal step completes each thread automatically. The test does not send
  an explicit close command for contract-based threads.
- Both identities verify the stored contract ID/name/version, participant records,
  attribution, step hashes and whole-thread integrity. Returned validation results
  must contain no critical, warning or minor violations.

A passing run writes `contract-multi-system-verification.json`. Set
`THREADIFY_E2E_EVIDENCE_FILE` to an explicit path outside the disposable directory
when you want to retain its non-secret contract/thread IDs after cleanup.

## Browser sessions across Engine and Web API

Run the compiled Engine and external Web API with the same local database,
license, installation and `registry.browser_origin`, then run:

```sh
THREADIFY_LIVE_DIR=/absolute/path/to/local/config-directory \
THREADIFY_E2E_WEB_API_URL=http://127.0.0.1:3003 \
go test ./e2e -run '^TestBrowserAuthentication$' -v -count=1
```

The test creates a temporary human principal and key, exchanges the key for a
cookie session, then reads Engine GraphQL/contracts and Web API profile/type
configuration. It checks missing CSRF rejection, retired password endpoints,
logout and source-key revocation across both services, and removes its fixtures.
Its `EngineUserLifecycle` subtest creates and lists invitations through the Engine,
changes roles, cancels invitations, checks that first activation cannot be forced,
and verifies suspension/reactivation/archival of an existing member against both
services using a personal key. The legacy Web API team route must return 410.
The `EnginePublicURL` subtest saves and reads a public URL, checks proxy-prefixed
endpoints and CSRF/invalid-URL rejection, then restores the original setting.
Both service URLs must be loopback. It skips when the Web API URL is omitted.
Registry email/SSO assertions and concurrent polling are tested separately with
isolated PostgreSQL schemas in `shared/auth`; those checks run in Engine CI.

## Management CLI

`TestStandaloneWorkflows/management_cli` runs the independently compiled client
selected by `THREADIFY_E2E_CLI_BINARY` against a disposable Engine selected by
`THREADIFY_E2E_BINARY`. It skips when the client path is not provided. It creates contracts and profile types, explicitly
creates/updates profiles, derives a profile from thread refs, finds and closes a
thread, and verifies its hash chain. It also completes a CLI login with real
cookie/CSRF approval requests, rejects approval replay, and checks logout.
See [CLI guide](../../docs/CLI.md).

### Python and Go SDK parity

The `sdk_parity` subtest starts a contract thread using Python, grants a separate
Go service account permission to report a charge, validates that exact event
from both SDKs, and completes the thread from Python. Both clients also exercise
reference-map lookup against the Engine. Service identities are seeded by the
fixture; contracts, joins, steps, waits and queries use public APIs.

```sh
# Build the Go participant from the sibling SDK checkout:
(cd ../../threadify-sdk-go && go build -o /tmp/threadify-go-parity ./tests/live)
# Use a Python >=3.10 environment with the Python SDK's dependencies installed.
THREADIFY_E2E_BINARY=/absolute/path/to/threadify-engine \
THREADIFY_E2E_PYTHON=/absolute/path/to/venv/bin/python \
THREADIFY_E2E_GO_SDK_BINARY=/tmp/threadify-go-parity \
  go test ./e2e -run '^TestStandaloneWorkflows$/^sdk_parity$' -v -count=1
```

Run these commands from `threadify-go/tests`. The test uses the Python source
checkout through `PYTHONPATH`, and skips when either SDK executable setting is
missing. The same disposable PostgreSQL, Valkey and Registry fixture used by the
other standalone tests is required. The parent repository must have its SDK
submodules checked out at revisions containing the parity clients.

## Engine-managed OTLP filters

From `threadify-go/tests`, using freshly built Engine and CLI binaries:

```sh
THREADIFY_E2E_BINARY=/absolute/path/to/threadify \
THREADIFY_E2E_CLI_BINARY=/absolute/path/to/threadify-cli \
go test ./e2e -run '^TestStandaloneWorkflows$/^ingestion_filters$' -v -count=1 -timeout=4m
```

This disposable-only scenario saves and previews rules through the CLI, checks
stale-save rejection, exports mixed and wholly excluded Protobuf batches, verifies
that excluded traces create no threads, confirms SDK WebSocket events bypass the
rules, and disables filtering to ingest a previously excluded trace. It never
changes ingestion policy on a `THREADIFY_LIVE_DIR` target.
