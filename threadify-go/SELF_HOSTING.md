# Self-hosting Threadify

`threadify` runs the engine, an embedded NATS JetStream broker, and PostgreSQL
persistence workers in one process. Linux/macOS releases also start a bundled
Valkey child process. PostgreSQL remains external. Windows connects to an external
Valkey server. See [managed and shared Valkey](docs/MANAGED_VALKEY.md).
The dashboard is embedded in release binaries and served on the Engine port.
Fused Registry provisions
the licensed account and supplies Threadify bandwidth, rate, and entity-profile
limits. Thread creation, event size, and token counts do not consume credits.

## Install a release binary

Download the installer from the latest Engine release, then run it:

```sh
curl --fail --show-error --location --proto '=https' --proto-redir '=https' \
  https://github.com/ThreadifyDev/engine/releases/latest/download/install.sh \
  --output install.sh
sh install.sh
```

The installer selects Apple Silicon macOS, x86-64 Linux or x86-64 Windows
(Git Bash), downloads the release archive and verifies its SHA-256 checksum
before installing. It requires `curl`, `tar` (or `unzip` on Windows), and
`sha256sum` or `shasum`. Other architectures must build from source.
For native PowerShell, download and extract the Windows ZIP as described below.

The default binary location is `$HOME/.local/bin/threadify` (`threadify.exe` on
Windows). Configuration templates go into `${XDG_CONFIG_HOME:-$HOME/.config}/threadify`.
Existing config files are preserved, including on upgrades. The installer needs
no `sudo`, does not edit your shell profile, and does not start the Engine.
Add the binary directory to `PATH` if you want to run `threadify` by name.

To choose a published version or installation directories:

```sh
sh install.sh --version v1.2.3 \
  --bin-dir "$HOME/.local/bin" \
  --config-dir "$HOME/.config/threadify"
```

Replace `v1.2.3` with your desired release. Rerun the installer to upgrade;
restart the Engine to use the new executable. Stop a running Windows Engine
before replacing its binary.

Follow the configuration requirements under **Build and configure** below:
supply your Registry license, PostgreSQL settings, and a persistent
hash-chain secret. Start using the command printed by the installer, normally:

```sh
"$HOME/.local/bin/threadify" --config "${XDG_CONFIG_HOME:-$HOME/.config}/threadify/config.yaml"
```

Embedded NATS data defaults to `data/jetstream` beside the executable. Set an
absolute `nats.store_dir` in YAML if you want a separate persistent data location.
The Linux/macOS installer also installs `libexec/valkey-server` beside the Engine.
Managed Valkey data defaults to `data/valkey`; an absolute `redis.store_dir` can
select a persistent volume. External Redis/Valkey uses `redis.url` (or `REDIS_URL`),
for example `rediss://default:ENCODED_PASSWORD@redis.example.com:6379/0`.
The old separate `redis.host`, `port`, `password`, and `db` fields are rejected;
replace them before upgrading. Open `http://localhost:8081` to use the bundled dashboard.

## Manage an Engine from the CLI

Install the separate `threadify-cli` client from `ThreadifyDev/cli` for login and resource commands:

```sh
threadify-cli config set api-url https://threadify.example.com
threadify-cli login
threadify-cli contracts create --file contract.yaml
threadify-cli profile-types create --file customers.yaml
```

Use `threadify-cli help` or the [CLI guide](docs/CLI.md) for API-key setup, explicit
profiles, thread references, integrity verification and logout. Client commands
connect to your Engine without starting the server. Browser login requires the
bundled dashboard and the correct `registry.browser_origin`.

## Public Engine URL

Set the address clients use to reach the Engine in `config.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8081
  public_url: "https://threadify.example.com"
```

The distributed configs also accept `THREADIFY_PUBLIC_URL`. An administrator can
set the same address in **Settings → Engine → Engine URL**. The UI setting
overrides the YAML default, is stored per installation in PostgreSQL, and survives
restarts. **Use config default** removes that override. YAML changes take effect
after restarting the Engine; UI changes take effect immediately.

The JavaScript SDK accepts this one address as `engineUrl` and derives its
WebSocket and GraphQL routes. OTLP/HTTP and MCP addresses appear in collapsed
setup sections. Reverse-proxy path prefixes are supported;
the proxy must strip the prefix before forwarding requests to the Engine.
The public URL is an advertised client address. It does not configure DNS, TLS,
the listener or browser authentication origins. The embedded dashboard uses its
serving origin; set `registry.browser_origin` to the public HTTPS origin behind a
TLS-terminating proxy. The dashboard requires hosting at the root of its hostname. See [browser deployment](docs/BROWSER_AUTH.md).

`GET /v1/engine/settings` returns the effective address, its config default,
source (`config`, `ui`, or `unset`) and client endpoints. Administrators use
`PUT /v1/engine/settings` with `{"public_url":"https://threadify.example.com"}`
to save, or `DELETE /v1/engine/settings` to restore the config default.

## Engine CI and releases

The engine uses `.github/workflows/engine-ci.yml` and `engine-release.yml`.
CI builds the Vite/React dashboard once on Node 24 and shares the static artifact
between validation and publication. Go embeds those files in all three binaries;
the Docker wrapper inherits the same UI. Generated assets are not committed.
GoReleaser rejects a release without a complete dashboard. Dashboard changes
participate in Engine release versioning; the public homepage stays independent.
For local source/Docker builds, prepare assets using [the dashboard instructions](../web/README.md).

Engine pull requests run the shared and engine unit suites, release-version
script and installer tests, GoReleaser validation, and compiled-binary E2E tests against
throwaway PostgreSQL and Valkey containers. The E2E suite includes entity
profiles, contract creation and use, OTLP ingestion/completion, recorded trace
timestamps, and hash verification.

Validation and publishing share a Go module/build cache through
`.github/actions/setup-engine-go`. Cache entries refresh per commit and fall back
to previous compatible entries. Both jobs use `CGO_ENABLED=0` and `-trimpath`, so
publishing can reuse compiled packages while linking the final release metadata.
All three archives and the Docker wrapper are still built and checked. Disposable
E2E scenarios run up to three at a time, each with its own database, Valkey, broker,
and Engine. Runs against an existing instance remain sequential. README-only
changes do not trigger releases; changes to packaged guides still do.

An engine change pushed to `main` runs the same checks before tagging and
publishing. Versioning follows the Fused engine-release convention: `feat:` bumps
minor, a conventional `!:` or `BREAKING CHANGE:` footer bumps major, and other
changes bump patch. Only engine-related commits determine the bump. Stable
engine tags use `vMAJOR.MINOR.PATCH`; SDK tags and SDK release workflows stay
independent. Pushing an explicit stable engine tag runs the same release gate.
Re-running a failed release reuses its existing tag and replaces uploaded assets.

Each GitHub release contains `install.sh`, three archives and `checksums.txt`:

| Host | Architecture | Archive |
| --- | --- | --- |
| macOS | ARM64 (Apple Silicon) | `threadify_VERSION_darwin_arm64.tar.gz` |
| Linux | AMD64 (x86-64) | `threadify_VERSION_linux_amd64.tar.gz` |
| Windows | AMD64 (x86-64) | `threadify_VERSION_windows_amd64.zip` |

Every archive contains `threadify` (`threadify.exe` on Windows), this guide, and
`config/config.yaml` copied from the self-hosting template. Extract the archive, configure it as described below, then run
`./threadify --config ./config/config.yaml` (PowerShell:
`.\threadify.exe --config .\config\config.yaml`). `--version` prints the version
and source commit without connecting to services.

The workflow also publishes `ghcr.io/threadifydev/engine:vVERSION`
and `:latest`. This Linux AMD64 image wraps the exact Linux release executable;
`Dockerfile.goreleaser` does not compile it again. It runs as UID 65532, exposes
port 8081, and stores NATS in `/data/jetstream` and Valkey in `/data/valkey` on a persistent
`/data` volume. Mount reviewed configuration at `/app/config` and supply the
same environment variables as a native deployment. PostgreSQL remains external.

GoReleaser is pinned to v2.18.0. Builds run one target at a time with two compiler
workers to bound memory use. GitHub Actions uses `GITHUB_TOKEN` with
`contents: write` for tags/releases and `packages: write` for GHCR; repository
rules must permit the workflow to create `v*` tags. No separate registry secret
is required. These workflows publish only after they are committed and pushed
to `main` (or a matching release tag).

To validate packaging locally without publishing, from `threadify-go` run:

```sh
goreleaser check
GORELEASER_CURRENT_TAG=v0.0.0 GORELEASER_PREVIOUS_TAG=v0.0.0 \
  goreleaser release --snapshot --clean --parallelism 1
```

Docker must be running for the wrapper build. Output goes to `dist/`;
snapshot images remain local. To build archives without Docker, also pass
`--skip=docker,publish`. CI executes the packaged Linux binary for its E2E tests.

## Build and configure

Build with the Go version declared in `go.mod` (currently Go 1.26):

```sh
cd threadify-go
make build
sh scripts/build-valkey.sh bin/libexec
```

The output is `bin/threadify`. GraphQL generation is an explicit development step
(`make generate-graphql`); a release build uses the checked-in generated code.
RBAC definitions, Lua scripts, and the GraphQL schema are embedded in the binary.
No Go toolchain, source checkout, or Node runtime is needed on the target host.

Create a deployment directory and copy the configuration template into it:

```sh
mkdir -p "$HOME/threadify/config" "$HOME/threadify/data/jetstream"
cp bin/threadify "$HOME/threadify/threadify"
cp -R bin/libexec "$HOME/threadify/libexec"
cp config/config.selfhost.yaml "$HOME/threadify/config/config.yaml"
```

Edit the deployed configuration before starting:

- No NATS settings are required. The binary starts embedded NATS and uses
  `data/jetstream` beside its resolved executable, independent of the working
  directory. The directory is created at startup and must be writable by the
  service user. An explicit absolute `nats.store_dir` overrides this location.
- Set `POSTGRES_URL`. Valkey requires no configuration for a new Linux/macOS
  deployment. Existing Redis connection fields must be replaced with `redis.url`
  pointing to the same server. See [sharing and migration](docs/MANAGED_VALKEY.md).
- Set `THREADIFY_BROWSER_ORIGIN` to the exact public UI origin (for example,
  `https://threadify.example.com`). Browser sign-in uses Fused Registry identity;
  Supabase and JWKS settings are no longer required. See [browser authentication](docs/BROWSER_AUTH.md).
- Generate your own `HASH_CHAIN_SECRET_V1` once with `openssl rand -hex 32` and
  retain it securely across restarts. Preserve all previous secret versions when
  rotating keys so historical activity chains remain verifiable.
- Set `THREADIFY_LICENSE_KEY` to a license provisioned for Threadify (or both
  products). The production Registry URL is built in; `THREADIFY_REGISTRY_URL`
  is an optional local-testing override.
  The same key may also be used by Fused Engine. Local billing configuration is
  unnecessary; Registry limits remain in memory.
- Review HTTP host and port. The standard configuration has no CORS or local
  rate-limit fields; the incoming request allowance comes from Registry.
  The Engine serves its bundled dashboard and API routes on the same listener.

YAML strings of the form `$NAME:default` use the named environment variable or
the supplied default; `$NAME` requires you to supply that value. Environment
variables must be set by your shell, container runtime, or service manager; the
binary does not automatically load a deployment `.env` file.

Start from any working directory using an absolute config path:

```sh
"$HOME/threadify/threadify" --config "$HOME/threadify/config/config.yaml"
```

Or set `CONFIG_PATH` to that file and launch the binary. Registry supplies the
plan allowances; `subscription.yaml` is no longer loaded or required. Existing
copies can be removed.
The engine uses its existing schema initialization on startup, so its PostgreSQL
role needs the required DDL permissions. Account/company records, API keys, and
product access comes from Fused Registry. The handshake reconciles the local
company; user authentication still uses the configured identity provider.

## Runtime modes

| Mode | Engine HTTP/WebSockets | Persistence workers |
| --- | --- | --- |
| `--mode combined` (default) | Yes | Yes, when `archiver.enabled: true` |
| `--mode engine` | Yes | No |
| `--mode writer` | No | Yes |

Use combined mode with `archiver.enabled: true` for the single-process deployment.
Writer mode allows the same executable to operate as a separate writer when
needed. Split processes must connect to the same external broker; separate
embedded brokers do not share messages.

`nats.mode: embedded` uses an in-process connection and exposes no NATS TCP port.
Keep its storage directory on a persistent volume that is writable by the process
and dedicated to one broker instance. An explicit `nats.mode: external` connects
to `nats.url`. Embedded mode is the default; deployments using an existing broker
must select external mode explicitly, even when a NATS URL is already present.

Use external NATS when running multiple engine replicas, splitting engine and
writer processes, or when the separate Web API must exchange NATS messages with
the engine. Configure every participant to use the same broker and compatible
stream/consumer names:

```yaml
nats:
  mode: external
  url: "nats://your-shared-nats:4222"
  archival_max_age_hours: 0
  archival_max_bytes: 1073741824
```

The remaining NATS settings use built-in defaults. Supply broker authentication
and transport protection through your deployment's existing NATS configuration.
Do not point multiple embedded processes at the same JetStream directory.

## Durability and capacity

Persistence remains asynchronous: a queued write may not yet be visible in
PostgreSQL. Archival streams retain pending messages until their durable consumer
acknowledges successful processing. The default archival maximum age is zero, so
pending writes do not expire during an extended database outage. Retries can
redeliver messages. Existing writer deduplication and billing semantics are
unchanged; this release does not provide exactly-once persistence or billing.
Some existing publication paths run after the response or log publication errors
without failing the operation. Consequently, an API success is not a universal
guarantee that its archival event has reached JetStream, and a process crash or
full queue before publication can still lose that archival event.

The template limits each archival stream to 1 GiB and the embedded broker's total
file storage to 8 GiB, with a 64 MiB broker memory budget. These are separate
budgets, not a guarantee that every stream can fill its quota simultaneously.
Archival streams reject new messages when full instead of evicting older pending
writes. Monitor free disk, stream bytes, pending acknowledgements, database errors,
and publisher errors; size capacity for your expected outage window. A full broker
or disk makes archival publication fail and requires recovery before normal
persistence can resume. Do not treat the queue as an unlimited backup or assume
all engine operations are a transaction spanning Valkey, JetStream, and PostgreSQL.

Use normal SIGTERM/SIGINT shutdown and allow the configured service/container stop
grace period to cover draining. Unacknowledged work remains in JetStream for
redelivery after restart. Retain and back up the broker volume alongside the
PostgreSQL database and the hash-chain secret history. Valkey still contains live
engine state and should be configured for the recovery guarantees you require.

## Upgrading an existing engine and writer deployment

Keep a copy of the old deployment configuration and preserve the original NATS
persistent store. This release uses work-queue retention for archival streams and
reuses the existing durable consumer names; it does not silently delete or recreate
incompatible existing streams. A stream with the old limits retention policy needs
an explicit migration before this release can use it.

For a move to a fresh embedded broker:

1. Stop new requests and all producers that publish to the old archival streams.
2. Leave the existing writer running until all pending and in-flight messages have
   been acknowledged and expected data is present in PostgreSQL. Inspect all
   archival streams and the separate step-state consumer, not just one queue.
3. Stop the old engine and writer cleanly. Preserve the original broker storage
   and configuration for recovery; do not copy raw JetStream files into a live
   embedded store or delete the old store as part of rollout.
4. Start the combined binary with a new, empty, persistent embedded store and the
   same PostgreSQL/Valkey connections, then verify health and persistence before
   reopening traffic.

If any old queue cannot drain, keep the old writer/broker available and resolve
that backlog before cutting over. For an external-broker upgrade, arrange an
explicit, verified stream migration after draining; the binary refuses an
incompatible retention policy instead of performing a destructive migration.
Do not run old and new consumers against an incompatible stream configuration.

## Container deployment

The engine Dockerfile builds the same combined executable and includes sanitized
configuration. It does not include a source tree or existing local
credentials. The runtime user is uid/gid 65532. A named `/data` volume
persists JetStream; host bind mounts must be writable by that user.

```sh
docker build -t threadify:local .
docker volume create threadify-data
docker run --name threadify --stop-timeout 60 \
  -p 8081:8081 \
  --env-file /absolute/path/to/threadify.env \
  -v /absolute/path/to/threadify/config:/app/config:ro \
  -v threadify-data:/data \
  threadify:local
```

The release image sets `NATS_STORE_DIR=/data/jetstream` for its mounted volume. The
configuration directory only needs `config.yaml`.
External service addresses must be reachable from the container; `localhost`
inside it refers to the container itself. Existing Compose deployments that mount
configuration for an external broker must explicitly set `nats.mode: external`.

Check a running installation with the same configuration/environment:

```sh
"$HOME/threadify/threadify" --config "$HOME/threadify/config/config.yaml" --healthcheck
docker exec threadify /app/threadify --healthcheck
```

The healthcheck command exits nonzero when the service is unavailable. Container
healthchecks use `CONFIG_PATH=/app/config/config.yaml`. Upgrading the binary also
upgrades the dashboard. The dashboard calls Engine management routes directly;
no separate Web API is required.

## Verification

Run engine unit tests with `go test -race ./...`. Broker and persistence tests
include real embedded JetStream restart/replay and storage-capacity checks.

The opt-in executable smoke test launches the compiled binary from a temporary
folder, provisions isolated test identities, creates threads over WebSockets,
shuts down with a connection open, restarts, and checks the PostgreSQL rows.
Use disposable PostgreSQL and Valkey instances only (the test initializes schema
and writes test records):

```sh
make build
THREADIFY_SMOKE_BINARY="$PWD/bin/threadify" \
THREADIFY_SMOKE_POSTGRES_URL='postgres://test:test@localhost:5432/threadify_test?sslmode=disable' \
THREADIFY_SMOKE_VALKEY_ADDR='localhost:6379' \
go test ./cmd/server -run TestStandaloneBinaryPersistenceAndRestart -v -count=1
```

The smoke test uses a local signed Registry fixture and no seeded credits. It
starts without NATS settings from a different working directory, verifies the
broker store beside the executable, and checks thread persistence and bandwidth
accounting across a restart. Set
`THREADIFY_SMOKE_REGISTRY_URL` to an isolated Registry fixture to exercise actual
Registry handlers; never point this test at a production Registry.

## OTLP execution completion

`POST /v1/traces` automatically completes engine-created, contract-free trace threads after accepting an ended root span (no parent ID). Exporters that omit an upstream parent can mark the final invocation span with the boolean attribute `threadify.run.complete=true`. Only a span attribute is accepted as this marker; a resource-wide value cannot accidentally close every span. The root's end timestamp becomes `completedAt`.

Completion means execution ended, including failed executions. Individual span outcomes remain recorded independently. It does not mean a support ticket was resolved or every distributed span has been delivered. Accepted late spans for the same trace extend the existing hash chain without reopening the thread. Root/completion retries do not duplicate recorded spans. An absent root/marker leaves a thread active; the engine does not guess completion from idle time.

Contract-linked threads and explicit targets outside the engine-created trace namespace keep their existing lifecycle. Their normal terminal-write restrictions still apply. A replay of an already-recorded root can automatically complete a trace imported before this behavior was enabled.

OTLP execution timestamps come from the producer: span `start_time_unix_nano` and `end_time_unix_nano` supply step start/end, and the ended root/marked invocation supplies run completion. The run starts at the earliest span in its initial received batch. Activity `recorded_at` uses the span's end time; storage `created_at` on step records remains arrival time. PostgreSQL stores microsecond precision. Historical ingestion does not retimestamp execution to the present. Hashes retain ingestion order while authenticating each event's producer timestamp.

## Billing integration

Fused Registry owns product access and live allowances. Threadify uses its dedicated handshake, heartbeat, and signed usage-report endpoints. The limits are never persisted in the local database; account binding, stable installation identity, usage counters, and the reporting outbox are durable. Local payment processing stays disabled. See [Registry integration](docs/REGISTRY_INTEGRATION.md) for setup, migration, and enforcement semantics.

## Build resource use

`make build` limits package compilation to two concurrent jobs and sets `GOMAXPROCS=2` for build tools only. The resulting server has no runtime CPU limit from these settings. Larger build machines can use `make build BUILD_JOBS=4 BUILD_PROCS=4`. Docker builds accept the same names as build arguments. Preserve the Go build cache; an unchanged cached build should be fast. Embedded NATS and HTTP/serialization dependencies still make a cold build larger than a small Go service.
