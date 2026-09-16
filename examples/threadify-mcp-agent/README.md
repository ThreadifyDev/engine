# External Threadify analyst with Harnest MCP

AI runs in this optional Harnest agent. Threadify's engine needs no model,
provider key or AI toggle. This example uses managed ADK, session history and
Harnest's MCP discovery, with no custom database tools or GraphQL HTTP client.

## Run the included example

Use a Harnest CLI supporting `harnest add mcp`. The installed 0.18.0 CLI used in
the original local demo predates that command; the current Harnest source
supports it and uses a newer project format. Upgrade the CLI and its matching
runtime together; check `harnest add mcp --help` before creating a connection.

```sh
export THREADIFY_MCP_URL=http://127.0.0.1:8081/sse
export THREADIFY_API_KEY='<service-account-api-key-with-read-permissions>'
export OPENAI_BASE_URL=http://127.0.0.1:11434/v1
export OPENAI_MODEL='<your-installed-model>'
# Also export OPENAI_API_KEY if your model provider requires it.

harnest env sync examples/threadify-mcp-agent --profile development
harnest test examples/threadify-mcp-agent
harnest compile examples/threadify-mcp-agent --output ./threadify-mcp-agent-build
harnest serve examples/threadify-mcp-agent --host 127.0.0.1 --port 8122
```

Harnest serves its own chat playground. For the dedicated local test engine,
use `http://127.0.0.1:8083/sse`. Despite the `/sse` name, this endpoint uses
**Streamable HTTP**. It accepts `X-API-Key`, not the browser's local bearer JWT.
The key stays in the agent process environment, never in prompts or chat.

The model endpoint is OpenAI-compatible; the example URL above is Ollama's
OpenAI-compatible endpoint. Select your own available model. A remote model
provider receives questions and returned telemetry; choose a local model if
those should stay local. Session/checkpoint storage is in memory and resets
when the agent stops. No Threadify Web API or UI is needed.

Ask “Find failed threads and explain their recorded errors,” “Inspect thread
<UUID> and cite its steps,” or “Show delivery health for customer <reference>.”
The agent exposes only `search_threads`, `get_thread`, `get_entity_profile`,
and `get_contract_violations`, prefixed with `threadify`. The engine still
enforces the API key's permissions.

## Create the connection using Harnest

The example was scaffolded with these real CLI commands:

```sh
harnest init my-threadify-agent --minimal
harnest add mcp threadify \
  --project my-threadify-agent \
  --url http://127.0.0.1:8081/sse \
  --transport streamable-http \
  --token-env THREADIFY_API_KEY \
  --token-header X-API-Key \
  --token-prefix=
```

The command stores `${THREADIFY_API_KEY}`, not its value. The checked-in
`mcp/threadify.py` adds a `THREADIFY_MCP_URL` override and the four-tool allowlist.
Use this example's `agent.py`, `config.yaml` and instructions when adapting the
scaffold: model settings come from the process rather than scaffold placeholder
URLs. Harnest discovers the MCP connection without manual tool registration.

## Live verification without calling a model

```sh
# Optional: also retrieve a thread visible to this API key.
export THREADIFY_SMOKE_THREAD_ID='<existing-thread-uuid>'
harnest test examples/threadify-mcp-agent --smoke
```

Unit tests check deferred credentials, endpoint overrides and the tool allowlist.
The opt-in smoke test discovers tools through Harnest's actual MCP adapter,
searches threads, and optionally retrieves one. It checks GraphQL errors inside
MCP responses as well as transport success. It neither invokes a model nor
creates threads or exports telemetry.

Local verification used the current Harnest source CLI/runtime: three unit tests,
one live MCP smoke test, and standalone compilation passed. The older installed
0.18.0 runtime could not load the new project schema, so its incompatible generated
dependency lock is not shipped. Generate runtime/development locks with the matching
updated release before deploying or using frozen CI.

## Verify an actual model answer

With the model environment above configured and a known thread visible to the key:

```sh
export THREADIFY_SMOKE_THREAD_ID='<existing-thread-uuid>'
THREADIFY_LIVE_MODEL_TEST=1 harnest test examples/threadify-mcp-agent --smoke
```

This opt-in test calls the configured model, requires an actual `get_thread` tool
result without GraphQL errors, and checks that the answer cites the thread ID,
recorded status and a recorded step. Set `THREADIFY_MODEL_EVIDENCE_FILE` to an
optional local JSON output path to retain the answer and tool names. Treat that
file as telemetry data. All five tests passed against the local engine using the
configured Ollama-compatible model endpoint.

## Test real agent traces against the updated Engine

The opt-in Engine binary test can run this agent with its configured model,
make real `get_thread` MCP calls against synthetic fixture data, and verify
OTLP ingestion in PostgreSQL. It exercises two Engine replicas and compares:

- Default `workflow.run_id` correlation: two agent traces share one thread.
- `?use_workflow_run_id=false`: each trace keeps its own thread.
- Custom span names, tool spans, and persisted trace IDs.

Use the existing disposable binary-test settings (`THREADIFY_SMOKE_BINARY`,
`THREADIFY_SMOKE_POSTGRES_URL`, `THREADIFY_SMOKE_MANAGED_VALKEY_BINARY`, and
`THREADIFY_SMOKE_SDK_DIR`), then enable the agent test:

```sh
export THREADIFY_HARNEST_TEST_PYTHON='<Python executable with this Harnest runtime>'
export THREADIFY_HARNEST_EVIDENCE_DIR='<local output directory>'
# From threadify-go:
go test ./cmd/server -run '^TestTwoEnginesShareManagedValkey$' -count=1 -v
```

The test runner reads the existing sample agent's model settings from
`~/.config/threadify-mcp-agent/environment.json`. It replaces the Threadify URL
and API key with the disposable fixture's credentials. The configured model
receives synthetic fixture data only. The running sample agent and installed
Engine are left unchanged.

Each test process represents one synthetic workflow and supplies its
`workflow.run_id` through `OTEL_RESOURCE_ATTRIBUTES`. For a server handling
multiple workflows, attach the appropriate run ID to each span instead of
setting one process-wide value. The test does not assume Harnest generates
this attribute automatically.

Evidence files contain model answers/tool names and Engine-side thread, trace,
and step counts. Logs redact the fixture and model keys.
