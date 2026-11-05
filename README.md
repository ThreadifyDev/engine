# Threadify

### The referee for your business processes.

Your services, partners, people and AI agents each do part of the work. Threadify
follows every step, checks it against your rules, stops the actions that would
break them, reacts when something goes wrong, and keeps a track record for every
customer, partner and agent.

[Website](https://threadify.dev) · [Documentation](https://docs.threadify.dev) · [Get a license](https://threadify.dev/signup)

## What it does

- **Watch every player.** Record steps from your services with the JavaScript,
  Python or Go SDK, send the OpenTelemetry traces you already produce, capture
  actions from your web app, or invite a partner into the same thread.
- **Know the rules.** Write a contract in plain sentences: which steps a process
  needs, in what order, who may perform them and what data they must carry.
  Every step is checked against it, the same way every time.
- **Stop the foul.** Before an action you can't undo, your code asks Threadify
  and only continues if the contract allows it. Agents can ask what they're
  allowed to do next, and human approvals are part of the contract.
- **Blow the whistle.** Subscribe to broken rules, outcomes and finished threads,
  and have your own services pause an account, halt a process or alert a team.
- **Keep every player's record.** Entity profiles connect the threads that share a
  customer, partner or agent, with their metrics and delivery health.
- **Trust the record.** History is signed and chained, so you can verify it hasn't
  been changed. Assistants can investigate it over MCP.

Your systems still do the work. Threadify checks it, gates it when you ask, and
tells you when something breaks.

## Run the Engine

The Engine is one process that serves the dashboard, the SDK and OpenTelemetry
endpoints, and the GraphQL and MCP APIs. Bring PostgreSQL; on Linux and macOS
the Engine starts its own Valkey and NATS.

### 1. Get a license

[Create your Threadify account](https://threadify.dev/signup) and save your
license key. You can also choose Threadify Cloud there instead of self-hosting.

### 2. Install

```sh
curl -fsSL --proto '=https' --proto-redir '=https' \
  https://github.com/ThreadifyDev/engine/releases/latest/download/install.sh \
  -o install.sh
sh install.sh
export PATH="$HOME/.local/bin:$PATH"
```

Available for macOS ARM64, Linux AMD64 and Windows AMD64 (Git Bash), or as the
Docker image `ghcr.io/threadifydev/engine`. Release archives are on the
[releases page](https://github.com/ThreadifyDev/engine/releases).

### 3. Configure

The installer writes `~/.config/threadify/config.yaml` (or
`$XDG_CONFIG_HOME/threadify/config.yaml`). Set these values and keep the rest of
the template:

```yaml
license_key: "$THREADIFY_LICENSE_KEY"

server:
  host: "0.0.0.0"
  port: 8081
  public_url: "http://localhost:8081"
  browser_origin: "http://localhost:8081"

postgres:
  url: "$POSTGRES_URL"

security:
  hash_chain_secrets:
    v1: "$HASH_CHAIN_SECRET_V1"
  hash_chain_current_version: "v1"
```

Values starting with `$` are read from the environment. Generate the hash-chain
secret once with `openssl rand -hex 32` and keep it: older history can only be
verified with the secret it was signed with. Configs that use the earlier
`registry:` keys keep working.

### 4. Start

```sh
threadify --config "${XDG_CONFIG_HOME:-$HOME/.config}/threadify/config.yaml"
```

Open `http://localhost:8081`, sign in, and create an API key. Point an SDK at that
address, or send OTLP/HTTP traces to `/v1/traces`.

For Docker, HTTPS, replicas and external Valkey or NATS, see
[Run the Engine](https://docs.threadify.dev/self-hosting).

## Next steps

| To | Read |
| --- | --- |
| Record your first thread | [Quick start](https://docs.threadify.dev/quickstart) |
| Send existing traces | [OpenTelemetry](https://docs.threadify.dev/opentelemetry) |
| Write a contract | [Write a contract](https://docs.threadify.dev/core-concepts/creating-contracts) |
| Check before acting | [Check before acting](https://docs.threadify.dev/core-concepts/execution-waits) |
| React to broken rules | [Subscribe to events](https://docs.threadify.dev/core-concepts/notifications/subscribing) |
| Follow a customer, partner or agent | [Entity profiles](https://docs.threadify.dev/core-concepts/entity-profiles) |
| Record from a web app | [Browser SDK](https://docs.threadify.dev/core-concepts/browser) |
| Give an assistant access | [MCP](https://docs.threadify.dev/core-concepts/mcp-integration) |
| Manage from a terminal | [CLI](https://docs.threadify.dev/cli) |

SDKs: [JavaScript](https://github.com/ThreadifyDev/node-sdk) ·
[Python](https://github.com/ThreadifyDev/python-sdk) ·
[Go](https://github.com/ThreadifyDev/go-sdk) ·
[CLI](https://github.com/ThreadifyDev/cli)

## Developing the Engine

See [Engine development and tests](threadify-go/README.md). The
[dashboard](web/README.md) lives in `web/`; CI builds it and embeds it in the
Engine binary.

The SDKs and CLI live in their own repositories. Local checkouts in
`threadify-sdk`, `threadify-sdk-go`, `threadify-sdk-python` and `threadify-cli`
are ignored by this repository; clone and commit them separately.
