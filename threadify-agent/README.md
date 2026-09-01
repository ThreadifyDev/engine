# Threadify Harnest agent

This is a standalone Harnest 0.9 agent. It does not replace or modify the Go
services. Authenticated clients call Harnest directly; its typed tools forward
the same verified bearer header to the existing Threadify GraphQL endpoint.

## Runtime flow

```text
Client -> Harnest :8090 -> Threadify GraphQL :8081
          sessions       existing data and authorization
          skill loading
          model/tool loop
             |
             +-- executed-tool OTLP exhaust -> Threadify /v1/traces
```

The root agent handles general questions. It progressively loads one of three
filesystem skills when live Threadify work is requested:

- `thread-analysis` for searches, failures, validation, and debugging;
- `contract-design` for creating partial-order Threadify YAML contracts from
  observed runs; and
- `execution-governance` for proposing or safely attempting a
  contract-sensitive action.

The public data tool surface mirrors Threadify's stable MCP operations:
`search_threads`, `get_oldest_thread`, `get_thread`, `get_entity_profile`, and
`get_contract_violations`. Their GraphQL documents are private, authored
constants. The model cannot submit arbitrary GraphQL, mutations, or
subscriptions.

The `threadify_guard` runtime plugin adds `propose_step` for planning and
intercepts tools declared with `@guarded_step(...)` at Harnest's `before_tool`
boundary. It asks Threadify's `proposeStep` query using the authenticated
caller's bearer token. A denial returns `THREADIFY_EXECUTION_BLOCKED` with the
required, satisfied, and missing steps without entering the tool body. The
`verify_step_execution` tool is a no-side-effect probe for testing this boundary.

Process prerequisites declared with `@observed_step(...)` are sent to
Threadify synchronously after the agent executes them. The result is not
returned to the model until GraphQL confirms that Threadify persisted the exact
successful step. Guarded tools are observed the same way after authorization;
a blocked result is never recorded as a completed step.

The same plugin attaches a telemetry exporter to Harnest's runtime exhaust. It
exports only completed `execute_tool` spans, normalizes each tool name to a
Threadify step, and correlates the resulting Thread with the Harnest session and
agent through refs. Model calls and HTTP/server spans are excluded, so
Threadify receives the tool process the agent actually performed.

Guard a real action tool by declaring its authored contract step:

```python
from harnest.plugins.threadify_guard import guarded_step
from harnest.tool import tool

@tool
@guarded_step("charge", thread_id_argument="thread_id")
async def charge_customer(thread_id: str, amount: int):
    ...
```

## Configuration

Export these variables before running the standalone server:

```bash
export LITELLM_MODEL=openai/gpt-4o-mini
export OPENAI_API_KEY=...
export THREADIFY_GRAPHQL_URL=http://127.0.0.1:8081/graphql
export THREADIFY_OTLP_ENDPOINT=http://127.0.0.1:8081/v1/traces
export THREADIFY_OTLP_API_KEY=...
export THREADIFY_JWKS_URL=...
export THREADIFY_JWKS_AUDIENCE=authenticated
export THREADIFY_JWKS_ISSUER=...
```

`THREADIFY_JWKS_URL` falls back to `JWKS_URL`, and
`THREADIFY_JWKS_ISSUER` falls back to `JWKS_ISSUER`, matching the existing
Threadify configuration names. The issuer may be omitted when the deployment
does not configure issuer validation.

`THREADIFY_OTLP_API_KEY` is a service credential for the asynchronous exhaust
sink; it is not the caller's bearer token. GraphQL tools and execution guards
continue to use the verified caller token, preserving per-user authorization.

By default, Harnest sessions and checkpoints use `MemoryStore`. Set
`HARNEST_DATABASE_URL` to a PostgreSQL DSN for durable sessions and resumable
execution.

## Validate and run

From the repository root:

```bash
harnest env sync threadify-agent
harnest test threadify-agent
harnest test threadify-agent --smoke
harnest serve threadify-agent --reload
```

The server listens on `http://127.0.0.1:8090`. Create a session and send a turn
using the same Threadify bearer token for both calls:

```bash
curl -sS -X POST http://127.0.0.1:8090/sessions \
  -H "Authorization: Bearer $THREADIFY_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"id":"threadify-demo","state":{}}'

curl -sS -X POST http://127.0.0.1:8090/responses \
  -H "Authorization: Bearer $THREADIFY_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"input":"Analyze the last two threads.","sessionId":"threadify-demo"}'
```

Add `"stream":true` to the response request for Harnest's neutral SSE event
stream. Session listing, messages, patching, and deletion use Harnest's standard
`/sessions` endpoints and are scoped by the verified Threadify identity.
