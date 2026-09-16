# Threadify Engine

Threadify is a Go engine for contracts, threads, validation, real-time events, and
activity history. The `threadify` executable can run the engine, embedded NATS
JetStream, and PostgreSQL persistence workers together. PostgreSQL and Valkey
remain external services.

The dashboard is embedded in release binaries and served at the Engine URL. CI
builds its static assets before Go compilation. Fused Registry
provides identity, Threadify licensing, and live allowances; the Engine manages
its local users and invitations.

## Self-hosting

Download a verified release binary without a Go toolchain:

```sh
curl --fail --show-error --location --proto '=https' --proto-redir '=https' \
  https://github.com/creativeJoe007/ThreadifyEngine/releases/latest/download/install.sh \
  --output install.sh
sh install.sh
```

The installer supports Apple Silicon macOS, x86-64 Linux and Windows through
Git Bash. It preserves existing config files; use `--version vX.Y.Z` to pin a
published release. Follow the [quick start](../README.md#quick-start)
to supply your license, database settings, and persistent hash-chain secret, then start:

```sh
"$HOME/.local/bin/threadify" --config "${XDG_CONFIG_HOME:-$HOME/.config}/threadify/config.yaml"
```

See [SELF_HOSTING.md](SELF_HOSTING.md) for configuration, persistent storage,
external-broker deployments, container commands, and migration from a separate
engine and archiver.

To build from source instead:

```sh
make build
# Configure copies of config/config.selfhost.yaml and
# config/subscription.selfhost.yaml first; deploy the latter as subscription.yaml.
./bin/threadify --config /absolute/path/to/config.yaml
```

The binary embeds RBAC definitions, Lua scripts, and the GraphQL schema. It does
not need a checkout or Node runtime at deployment. `--mode combined` is the
normal single-process mode; `--mode engine` and `--mode writer` retain split
operation with an external broker.

## CLI

```sh
threadify-cli config set api-url https://threadify.example.com
threadify-cli login
threadify-cli whoami
threadify-cli contracts create --file contract.yaml
```

The management CLI lives in the separate `ThreadifyDev/cli` repository (local checkout: `../threadify-cli`). See [CLI.md](docs/CLI.md)
for profiles, thread queries and automation with service-account keys.

## Repository structure

```text
cmd/server/          Combined executable entry point
cmd/archiver/        Legacy standalone writer entry point
config/              Runtime configuration and subscription templates
internal/app/        Application assembly and HTTP routing
internal/archiver/   Persistence consumers and batching
internal/database/   PostgreSQL schema initialization
internal/graphql/    GraphQL schema, generated server, and resolvers
internal/service/    Engine business logic
internal/repository/ PostgreSQL and Valkey repositories
shared/              Shared module: authentication, billing, RBAC, configuration
api/                 Separate Web API module
tests/               Integration test module
```

## Development

Use the Go version in `go.mod` (currently Go 1.26). Release builds use the checked-in
GraphQL generated code; regenerate it only when changing the schema:

```sh
make build
make generate-graphql
make test-archiver
make test-engine
make test-shared
```

Some tests need PostgreSQL, Valkey, or NATS; see [tests/README.md](tests/README.md)
for the integration setup. Run `make help` for the existing API, archiver, Docker,
and multi-architecture publishing targets.

## OpenTelemetry traces

`POST /v1/traces` accepts binary OTLP/HTTP trace requests with
`Content-Type: application/x-protobuf` and `X-API-Key: <threadify-api-key>`, including
gzip-compressed requests. See [OTLP Trace Ingestion](../docs/OTLP_INGESTION.md) for
mapping, configuration, custom `threadify.*` attributes, and signal support.

See [WebSocket documentation](../docs/WEBSOCKET.md) for engine client connections.
