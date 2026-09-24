# Threadify Agent

The workspace assistant uses the pinned Harnest 0.23.0 snapshot in
[`runtime/vendor`](runtime/vendor/README.md), managed ADK, and the Engine YAML
model gateway configuration. The docked sidebar streams responses and executes typed
`client_tool` requests while users continue working in the page.

## Runtime flow

```text
Browser cookie + CSRF -> Engine /api/harnest -> Threadify Agent
                           |                    |
                    verified bearer             +-> Engine /v1/agent/identity
                                                +-> authenticated GraphQL/REST reads
                                                +-> client_tool request -> browser
                                                    -> result -> resume same turn
```

The Engine validates the browser session and forwards only the required headers.
Harnest verifies opaque, revocable sessions through `/v1/agent/identity` on every
request. History is scoped to company and user. Bearer credentials remain private
and are resolved through audience-scoped credential providers, never tool arguments,
page context, or model messages. The browser does not handle bearer tokens.

## Available tools

| Tools | Capability |
| --- | --- |
| `get_page_context` | Read the current route, resource identifiers, and local contract draft when page context is enabled. |
| `navigate_ui` | Navigate to allowlisted Threadify screens and selected resources. |
| `open_contract_draft` | Place Gherkin into the visible contract editor, requiring the latest draft revision. |
| `preview_contract_draft` | Run the existing Engine compiler preview and show its diagnostics. |
| `list_contracts`, `get_contract`, `get_contract_graph` | Read contract source, versions, and the authored graph. |
| `search_threads`, `get_oldest_thread`, `get_thread`, `get_contract_violations` | Inspect observed execution and validation failures. |
| `list_entity_profile_types`, `list_entity_profiles`, `get_entity_profile` | Discover profile types and inspect entities. |

Server tools use fixed GraphQL documents or REST paths; they do not expose arbitrary
queries or mutations. Frontend tools use an allowlist of shared handlers, not DOM
selectors or arbitrary JavaScript. Concurrent manual edits invalidate stale agent
writes and preview results. Saving uses the existing user-operated contract button.

### Action tools and approval

The model calls a normal business action tool. Its implementation can query
Threadify's `next` and `can` decisions internally; those decisions are not
model-facing tools. An action with a Contract approval prerequisite needs a
host-managed reviewer prompt, a validated approval step recorded by an
authorized reviewer, and an atomic `waitFor` claim before an external side
effect. Threadify does not open the Harnest prompt itself.

`delete_test_record` is a disposable in-memory demo. It calls `next` and `can`
inside one action-tool call, then uses a hardcoded Harnest approval gate when
the mock Engine allows deletion. The mock does not record a Contract approval
step or grant a `waitFor` claim, so this demo must not be read as a test of
Contract-driven approval. See [Contract decisions](../threadify-go/docs/DECISION_APIS.md#contract-driven-human-approval).

Contract design uses the Engine's supported Gherkin syntax and a bounded preview
repair loop. Trace ingestion filters can be explained and their settings opened;
field extraction authoring remains a future capability. There is no WebMCP adapter
yet; it can later expose these same frontend handlers.

## Configuration and startup

### Local model gateway in Engine YAML

For a walkthrough covering personal gateways, containers, keys, certificates and
troubleshooting, see [Configure a personal AI gateway](../docs/PERSONAL_AI_GATEWAY.md).

For bundled local mode, the Engine supplies its configuration path and connection
settings automatically. For a separately launched agent, set
`THREADIFY_CONFIG_PATH` on that process to the absolute path of the Engine YAML. The agent reads its `ai` section and, only for hosted license authentication, the Registry credential settings. Both processes
must receive the same configuration file; the Engine does not distribute keys or
certificate files to Harnest over HTTP.

```yaml
ai:
  enabled: true
  gateway:
    base_url: http://127.0.0.1:11434/v1
    model: qwen3.5:cloud
    # api_key_env: THREADIFY_AI_GATEWAY_KEY
```

The gateway must support OpenAI-compatible `/chat/completions`, streaming, and
function tools. The model is the gateway's exact model ID, including any namespace.
The protocol adapter does not select OpenAI as a provider: all inference goes to
the explicit `base_url`. A path prefix such as `/custom/v1` is preserved.

For a gateway with a private CA and/or client certificate, use HTTPS:

```yaml
ai:
  enabled: true
  gateway:
    base_url: https://gateway.example.com/v1
    model: preferred-model
    api_key_env: THREADIFY_AI_GATEWAY_KEY
    tls:
      ca_file: certs/gateway-ca.pem
      cert_file: certs/client.pem
      key_file: certs/client-key.pem
```

`api_key_env` is optional and names a variable in the agent's environment. If
specified, an unset/empty variable is an error. Without it, no Authorization
header is sent, even if unrelated provider keys exist in the environment.
CA and client certificates are optional independently; a client certificate and
unencrypted PEM private key must be supplied together. Certificate paths resolve
relative to the YAML file and must be readable by Harnest (mount them in that container when
needed). Server certificate and hostname verification stay enabled. Redirects
and environment HTTP proxies are disabled for this model transport.

The URL, model, and certificate paths support the Engine's `$VAR:default` syntax.
Gateway settings take precedence over `LITELLM_MODEL`/Ollama environment settings.
Invalid configuration fails startup; requests never fall back to another provider.
`ai.enabled: false` disables the Engine's agent proxy and prevents the configured
agent from starting. Restart the Engine and agent after changing this file or
rotating certificates/keys. Without `THREADIFY_CONFIG_PATH`, the legacy Ollama
environment setup below remains supported. Custom gateways bypass the hosted service entirely.

```bash
export THREADIFY_CONFIG_PATH=/absolute/path/to/engine/config.yaml
```

### Hosted Threadify gateway

To use the stateless Threadify Web API gateway, set `ai.gateway.auth: threadify_license`,
`base_url` to the deployed gateway's `/v1` URL, and `model: threadify-agent`.
Harnest reuses the Engine's `registry.license_key` and optional `installation_id`
(or their `THREADIFY_LICENSE_KEY` / `THREADIFY_INSTALLATION_ID` environment fallbacks).
No extra provider key is needed. This mode cannot be combined with `api_key_env`.
It requires HTTPS except for loopback tests. The same TLS settings support private
CAs and incoming mTLS at the gateway's reverse proxy.

This is opt-in: custom gateways never receive the Threadify license automatically.
There is no provisioned public URL yet. See the [gateway deployment guide](../threadify-go/api/README.md).

### Production activation

```bash
threadify serve --config ./config.yaml --with-agent
```

Every Engine embeds the compiled agent. The local flag installs the matching
checksum-verified runtime if absent, then manages the agent process. Without the
flag or `ai.agent.url`, agentic features are hidden. External mode uses:

```yaml
ai:
  agent:
    url: http://127.0.0.1:8090
```

Each Engine using `--with-agent` owns its agent and temporary sessions. Route a
conversation’s requests to the same Engine. The external URL points directly to
a compatible agent service, not another Engine.

`THREADIFY_AGENT_URL` remains an environment override. Local and external mode
cannot be combined. See [Engine agent setup](../threadify-go/docs/AGENT.md) for
persistent caches, offline installation, Docker, and startup diagnostics.

The production artifact excludes `extensions/threadify` and
`extensions/threadify_guard`: they are SDK/governance demonstrations. Production
workspace tools authenticate as the signed-in caller and need no SDK service key.
The proxy exposes only conversation/session/client-tool routes and the Engine
provides `/v1/agent/identity` for caller verification.

### Manual development and SDK examples

The following workflow launches the source tree, including its demonstration
extensions. Use a CLI compatible with the pinned runtime snapshot. Its default uses
Qwen through Ollama; `qwen3.5:cloud` is cloud-backed and requires Ollama sign-in.
Set `LITELLM_MODEL=ollama_chat/<model>` for another installed tool-capable model:

```bash
export LITELLM_MODEL=ollama_chat/qwen3.5:cloud
export OLLAMA_API_BASE=http://127.0.0.1:11434
export THREADIFY_ENGINE_URL=http://127.0.0.1:8081
export THREADIFY_AUTH_MODE=engine
export THREADIFY_GRAPHQL_URL=http://127.0.0.1:8081/graphql
export THREADIFY_WS_URL=ws://127.0.0.1:8081/threads
export THREADIFY_API_KEY=... # Existing SDK extension's service credential.
export THREADIFY_OTLP_ENDPOINT=http://127.0.0.1:8081/v1/traces
export THREADIFY_OTLP_API_KEY=... # Optional separate telemetry credential.
```

Existing `.env` configuration can supply these values. Do not place secrets in
`config.yaml`. Connection settings are inherited from the process environment;
literal `spec.environment` entries can override exported values.
The retained SDK extension opens its service websocket on startup;
its service credential is separate from each caller's authorization. The retained
execution-governance extension exports completed tool spans and enforces authored
guards on its demonstration tools. Those tools do not publish workspace contracts.

For deployments using externally issued JWTs, explicitly set
`THREADIFY_AUTH_MODE=jwks` and configure `THREADIFY_JWKS_URL`,
`THREADIFY_JWKS_AUDIENCE`, and `THREADIFY_JWKS_ISSUER`. Browser sessions require
`engine` mode; there is no fallback from failed Engine verification to JWKS.

From the repository root:

```bash
harnest env sync threadify-agent
harnest test threadify-agent
harnest test threadify-agent --smoke
harnest compile threadify-agent --output /tmp/threadify-agent-compiled
harnest serve threadify-agent --port 8090 --reload
```

The server listens on port 8090. Sign into Threadify and open **Ask Agent**. Try
“List my contracts” or “Help me create a Gherkin contract for a refund workflow.”
The frontend requires no provider key or separate agent login.

## Build the bundled artifact and native runtime

From the repository root, with `uv` and a compatible compiler on `PATH`:

```bash
python3 threadify-agent/runtime/build.py --output /tmp/threadify-agent-build
```

This builds `agent.tar.gz`, `runtime-<os>-<arch>.tar.gz`, and the matching
`manifest-<os>-<arch>.json`. The runtime contains portable Python and hash-pinned
dependencies; only the agent artifact and manifest are embedded in Go.

The runtime uses the production dependency lock. Before packaging, the builder
removes bootstrap installers, tests, caches, headers, static libraries and duplicate
interpreter aliases, then runs the shared import smoke check. Package metadata,
licenses, provider data and native runtime libraries remain intact. Build tools
stay in build stages; the Engine Docker image downloads this optional runtime only
when local agent execution is enabled.

For a local macOS ARM64 development build, for example:

```bash
cp /tmp/threadify-agent-build/agent.tar.gz threadify-go/internal/agentbundle/assets/
cp /tmp/threadify-agent-build/manifest-darwin-arm64.json \
  threadify-go/internal/agentbundle/assets/runtimes.json
(cd threadify-go && go build -o bin/threadify ./cmd/server)
threadify-go/bin/threadify serve --config ./engine-config.yaml --with-agent \
  --agent-runtime-archive /tmp/threadify-agent-build/runtime-darwin-arm64.tar.gz
```

That manifest enables only the local platform. Release CI builds Linux AMD64,
macOS ARM64 and Windows AMD64, then `.github/scripts/stage-engine-agent.py` verifies
and merges all three manifests before compilation. GoReleaser refuses incomplete
release inputs. Compiler source and build tools are pinned in Engine CI; vendor
provenance is documented beside the runtime wheel. Never publish a local-only
manifest as a release.

## Sessions and cancellation

Sessions/checkpoints use MemoryStore by default; configure `HARNEST_DATABASE_URL`
for durable storage. Run one Harnest process for this initial integration, or ensure
session affinity: pending client-tool continuations belong to the serving process.
Durable history alone does not remove that requirement.

Closing the sidebar preserves the conversation and lets the turn continue.
**Stop** aborts the browser request and prevents subsequent frontend actions; the
next message creates a new session. It does not guarantee cancellation of a model
request already running on the server. Unanswered client tools expire in Harnest.
Leaving the signed-in area clears local conversation and draft state.

## Verification

From `web`, `npm run test:agent` checks navigation
allowlists, revision conflicts, cancellation, SSE chunking, and chained client-tool
continuations. `npm run typecheck`, `npm run build`, and `npm run test:routes` cover
the application integration. Go's `TestAgent*` tests cover proxy route/header
boundaries and selected identity output.

Agent unit tests cover discovery, credential audiences, identity verification, and
read-tool inputs. Authentication smoke tests run the compiled Harnest HTTP app with
synthetic Engine identity responses and an isolated SDK websocket, verifying tenant
separation and revoked sessions without calling a model or a live Engine.

Live browser verification on 2026-09-22 passed with `ollama_chat/qwen3.5:cloud`:
contract listing/source/graph, profile type/entity listing and profile reads,
frontend navigation, page context, opening a Gherkin draft, and compiler preview.
The draft was left unsaved. Harnest 0.20's media walker rejects array-valued JSON
Schema `type` fields; affected tool results use lossless JSON text in `data_json`
to preserve the data without changing the managed runtime.

The client-tool-to-server-query regression was also verified live: navigate to a
thread, read page context, then fetch and summarize its authenticated thread data.
This requires the client-tool authentication handoff fix in the local Harnest
checkout. Stock Harnest 0.20 loses request credentials when the initial stream
ends at a client action. The fix binds each continuation to its newly authenticated
request and preserves request-lifetime revocation and user ownership.

Until that fix is in an installed Harnest release, the local test launcher uses
the patched source checkout through `PYTHONPATH` and the CLI's supported explicit
Python override. To use the same development setup, keep the Engine/Ollama
environment above and run from this directory:

```sh
export HARNEST_SOURCE=/Users/martins2/Downloads/OpenSync/Harnest
PYTHONPATH="$PWD/../threadify-sdk-python:$HARNEST_SOURCE/src${PYTHONPATH:+:$PYTHONPATH}" \
  harnest serve . --python "$PWD/.venv/bin/python" --port 8090
```

The SDK source override enables `threadify.thread(thread_key, options)` from this workspace. The locked published SDK 0.2.10 does not provide that API. Keep this override for local development until the updated SDK is released and the dependency locks are refreshed.

This leaves the generated `.harnest/` runtime untouched. Harnest regression tests
cover fresh credentials on repeated HTTP and SSE handoffs, child-agent reads,
cross-user rejection, private credential handling, and revocation.
