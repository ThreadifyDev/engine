# Threadify

### Know what happened to every customer request.

Threadify turns activity across your services, partners, and agents into a shared
execution history. Follow an order from payment to delivery, validate each step
against a contract, and see where a customer's experience breaks down.

[Website](https://threadify.dev) · [Documentation](https://docs.threadify.dev) · [Get a license](https://threadify.dev/signup)

## What you can do

- **Follow the whole journey.** Bring steps from multiple systems into one thread.
- **Define successful delivery.** Use contracts to validate required steps, order, and timing.
- **Understand each customer.** Entity profiles connect their threads into a delivery history.
- **React while work happens.** Subscribe to events and use permission waits to coordinate services.
- **Give agents the full context.** Let support agents inspect threads through Threadify MCP.
- **Use your existing telemetry.** Send OpenTelemetry traces or instrument with JavaScript, Python, and Go SDKs.
- **Verify the record.** Check activity integrity with hash-chain verification.

Self-host with embedded NATS, database writers, and bundled Valkey on Linux/macOS.
Bring PostgreSQL and keep your execution data in your infrastructure. Windows
connects to an external Valkey server.

## Quick start

### 1. Get your license

[Create your Threadify account](https://threadify.dev/signup), verify your email,
and save your Threadify license key.

### 2. Install

```sh
curl -fsSL --proto '=https' --proto-redir '=https' \
  https://github.com/ThreadifyDev/engine/releases/latest/download/install.sh \
  -o install.sh
sh install.sh
export PATH="$HOME/.local/bin:$PATH"
```

Available for **macOS ARM64**, **Linux AMD64**, and **Windows AMD64** (Git Bash).
[Download release archives](https://github.com/ThreadifyDev/engine/releases) for manual installation.

### 3. Configure with YAML

Edit the installed `~/.config/threadify/config.yaml` (or
`$XDG_CONFIG_HOME/threadify/config.yaml`). Update these sections in the supplied file:

```yaml
registry:
  license_key: "YOUR_THREADIFY_LICENSE_KEY"

server:
  host: "0.0.0.0"
  port: 8081
  public_url: "http://localhost:8081"

postgres:
  url: "postgres://threadify:YOUR_PASSWORD@localhost:5432/threadify?sslmode=disable"

security:
  hash_chain_secrets:
    v1: "YOUR_PERSISTENT_SECRET"
  hash_chain_current_version: "v1"
```

Use a running PostgreSQL database. Valkey starts automatically on Linux/macOS;
see [managed and shared Valkey](threadify-go/docs/MANAGED_VALKEY.md) for external
servers, Windows, or multiple Engines. Generate the secret once
with `openssl rand -hex 32` and retain it across restarts. The database URL above
is for local development; configure TLS for a remote database.

Keep the rest of the template. Registry supplies plan allowances; no
`subscription.yaml` is needed. YAML is the
main configuration file; `$ENV_VAR` references are also supported for secrets.

### 4. Start

```sh
threadify --config "${XDG_CONFIG_HOME:-$HOME/.config}/threadify/config.yaml"
```

Open `http://localhost:8081` for the built-in dashboard. Point your SDK's Engine URL here,
or send OTLP/HTTP traces to `/v1/traces` with a Threadify API key.

## Build your first workflow

| Start with | Guide |
| --- | --- |
| Trace an order across services | [JavaScript SDK](https://github.com/ThreadifyDev/node-sdk) · [Python SDK](https://github.com/ThreadifyDev/python-sdk) · [Go SDK](https://github.com/ThreadifyDev/go-sdk) |
| Send existing traces | [OpenTelemetry](docs/OTLP_INGESTION.md) |
| Create contracts and entity profiles | [Threadify CLI](https://github.com/ThreadifyDev/cli#readme) |
| Give an agent access to your threads | [Harnest + Threadify MCP example](examples/threadify-mcp-agent/README.md) |
| Connect the dashboard | [Browser setup](threadify-go/docs/BROWSER_AUTH.md) |
| Deploy with Docker or configure storage | [Self-hosting guide](threadify-go/SELF_HOSTING.md) |

The dashboard ships inside the Engine binary. The CLI is installed separately.
The Docker image is
`ghcr.io/threadifydev/engine:latest`.

[Engine development and tests](threadify-go/README.md)

The [public website](homepage/README.md) builds and deploys from `homepage/`.
The [dashboard](web/README.md) lives in `web/`, separately from marketing and
Registry signup. CI builds its static assets and embeds them in the Engine.

The SDKs and CLI are maintained in their linked repositories. Local checkouts in
`threadify-sdk`, `threadify-sdk-go`, `threadify-sdk-python`, and `threadify-cli` are
ignored by this repository; clone and commit them separately when developing those projects.
