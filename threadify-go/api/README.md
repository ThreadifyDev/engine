# Threadify AI gateway (Web API)

The `cmd/server` binary now runs a **stateless hosted inference gateway**, shared by
multiple customer Engines. It starts without PostgreSQL, NATS, Redis, browser
authentication, RBAC files, an Engine URL, or a gateway-owned customer license.
It does not save prompts, responses, conversations, tool results, credentials, or
usage records. Only connection pools and in-flight request slots live in memory.

```text
Browser → customer Engine → customer Harnest → Threadify AI gateway → Ollama
                                                 ↓
                                          Registry license check
```

Harnest runs the agent and tools. The gateway forwards OpenAI-compatible chat
completions, including SSE streaming and function tool calls, without executing
them. OpenAI-compatible describes the wire protocol; the default deployment
example below uses Ollama.

## Run locally

From this directory, with Registry and Ollama already running:

```sh
export THREADIFY_AI_REGISTRY_URL=http://127.0.0.1:YOUR_REGISTRY_PORT
export THREADIFY_AI_UPSTREAM_URL=http://127.0.0.1:11434/v1
export THREADIFY_AI_UPSTREAM_MODEL=qwen3.5:cloud
export THREADIFY_AI_PORT=18090
go run ./cmd/server
```

Use your actual registry port. `qwen3.5:cloud` needs Ollama cloud sign-in; substitute
an installed tool-capable local model for entirely local inference. A hosted
installation should run behind a TLS reverse proxy with request-body logging
and response buffering disabled. Build with `threadify-go/Dockerfile.api` using
`threadify-go` as the Docker build context. No database or volume is required.
No public gateway URL is provisioned by this change.

| Environment | Meaning |
| --- | --- |
| `THREADIFY_AI_REGISTRY_URL` | Required trusted registry base URL. |
| `THREADIFY_AI_UPSTREAM_URL` | Required inference base URL, including `/v1`. |
| `THREADIFY_AI_UPSTREAM_MODEL` | Required exact model ID served by that upstream. |
| `THREADIFY_AI_UPSTREAM_KEY` | Optional upstream credential, held only by the service. |
| `THREADIFY_AI_MODEL` | Public model ID; default `threadify-agent`. Only this ID is accepted. |
| `THREADIFY_AI_PORT` | Listener port; default `8081`. |
| `THREADIFY_AI_MAX_CONCURRENT` | Global in-flight request cap; default `16`; excess requests return 429. |
| `THREADIFY_AI_TIMEOUT_SECONDS` | Whole request/stream timeout; default `180`. |
| `THREADIFY_AI_CA_FILE` | Optional upstream private CA PEM. |
| `THREADIFY_AI_CERT_FILE`, `THREADIFY_AI_KEY_FILE` | Optional upstream client certificate and unencrypted PEM key; must be paired. |

Registry and upstream URLs must use HTTPS, except loopback development URLs.
Provider certificate options apply only to the provider connection; the registry
uses system trust. For a gateway requiring incoming client certificates, configure
mTLS on the TLS reverse proxy; the Harnest client supports `ai.gateway.tls`.

## Connect an Engine's agent

Set `THREADIFY_CONFIG_PATH` on Harnest to the Engine YAML and select hosted auth:

```yaml
ai:
  enabled: true
  gateway:
    base_url: https://your-threadify-ai-host.example/v1
    model: threadify-agent
    auth: threadify_license
```

The example hostname is a placeholder. Local testing can use
`http://127.0.0.1:18090/v1`. Harnest reuses `registry.license_key` and
`registry.installation_id` from that file (including `$VAR:default` syntax), falling
back to `THREADIFY_LICENSE_KEY` and `THREADIFY_INSTALLATION_ID`. If installation ID
is absent, Registry uses its existing API-key identity fallback. The license stays
server-side: users need no additional model key. Hosted auth is explicit so a
customer's unrelated custom gateway never receives their Threadify license.

Customers can continue to bypass this service by configuring their own
`ai.gateway.base_url`, model, optional `api_key_env` and TLS settings; omit `auth`
or use `auth: custom`. See [agent configuration](../../threadify-agent/README.md).

## API and authorization

- `GET /health`: liveness, no authentication; does not assert provider readiness.
- `GET /v1/models`: authenticated single public model listing.
- `POST /v1/chat/completions`: authenticated JSON or streaming inference; 2 MiB request limit.

Each inference/list request verifies `Authorization: Bearer <existing license>`
with Registry's existing `/api/threadify/handshake`. Optional
`X-Threadify-Installation-ID` goes to Registry only. Verification is per request;
there is no local credential cache or stale-license grace. Registry outages deny
new requests with 503. Revocation/suspension denies access with 401/403. Already
running streams end on completion, cancellation, or timeout.

Registry owns the durable license/installation records; its handshake may refresh
that installation record. The gateway never starts the single-company Registry
runtime. The current Registry has **no AI-specific entitlement or token allowance**:
this release grants inference to active Threadify licenses. Concurrency is a
service protection, not per-customer billing. No new metering protocol, token
billing, or classifier migration is introduced. Those policies belong in Registry
if added later; the gateway should remain stateless.

Only deployment-configured upstream routing is allowed. Customer authorization,
cookies and forwarded identity headers never reach the model provider. Redirects
are not followed. Non-success provider bodies and Registry diagnostics are replaced
with stable error codes. Request cancellation propagates upstream. No retries are
performed, avoiding duplicate model work.

## Legacy Web API migration

The old `app`, handlers, services, repositories, middleware, database setup, email
templates, code samples and their tests have been removed. The gateway module uses
only the Go standard library; it has no third-party module dependencies. Old Web
API configuration and management routes are no longer supported.
The current UI already calls the Engine:

| Former responsibility | Current owner |
| --- | --- |
| Contracts, preview, versions | Engine `/v1/contracts` |
| Graph, threads, entity profiles | Engine `/graphql` and Engine ingestion routes |
| Profile types/views, roles, service accounts, API keys, user profile | Engine `/v1/*` management routes |
| Browser identity | Engine's Registry-backed browser authentication |
| Pricing and plan readout | Engine `/v1/pricing`, `/v1/billing/plan`, backed by Registry |
| Agent sessions and client tools | Engine `/api/harnest` → local Harnest |
| Old chat history and credit checkout/spending-limit mutations | Retired; not part of this gateway |

Deploying the new image replaces the old Web API route surface. Keep the browser
pointed at its Engine, not at this service. The old management compose service has been removed. Deploy the gateway separately
using the environment settings above.

## Verification

```sh
go test -race ./gateway ./cmd/server
go vet ./gateway ./cmd/server
go build ./cmd/server
```

Tests cover independent tenants, revocation, registry outage/invalid responses,
credential/header isolation, real SSE delivery, cancellation, payload bounds,
model allowlisting, legacy-route removal, overload rejection, and upstream mTLS.
