# Configure a personal AI gateway

The Threadify Agent supports Ollama and gateways with **OpenAI-compatible Chat
Completions, streaming and function tools**. An OpenAI account is not required.

## Enable the agent

Configure a model below, then start the Engine with:

```sh
threadify serve --config ./config.yaml --with-agent
```

The agent is bundled with the Engine. The first local start downloads and verifies
its matching runtime; later starts reuse the cache. No separate agent service or
service credential is needed.

For Docker, append `serve --with-agent` after the image name and keep `/data` on a
persistent volume. Runtime files are cached at `/data/agent`.

| Engine configuration | Result |
| --- | --- |
| No `--with-agent`, no external agent URL | Agent disabled; AI features hidden |
| `--with-agent` | Start the bundled agent locally |
| `ai.agent.url` | Connect directly to an external agent |

Local and external modes cannot be combined. `ai.enabled: false` disables AI and
cannot be combined with `--with-agent`. An enabled agent that becomes unavailable
shows a retry state; the Engine continues serving other features.

## Configure the model

Add one of these sections to the **Engine's `config.yaml`**, keeping its existing settings.

### Ollama

```yaml
ai:
  enabled: true
  gateway:
    base_url: http://127.0.0.1:11434/v1
    model: your-installed-tool-capable-model
```

Replace the model placeholder with its exact Ollama name. Use a locally installed
model for local inference; cloud models send requests to their provider.

### Gateway with a key

```yaml
ai:
  enabled: true
  gateway:
    auth: custom
    base_url: https://ai.example.com/v1
    model: your-model-id
    api_key_env: THREADIFY_AI_GATEWAY_KEY
```

Set `THREADIFY_AI_GATEWAY_KEY` in the **Engine environment** for a local agent, or
the external agent environment, through your service manager or secret store. Omit `api_key_env` if no bearer key is required.

### Gateway with certificates

```yaml
ai:
  enabled: true
  gateway:
    base_url: https://ai.internal.example.com/v1
    model: your-model-id
    # Optional if the gateway also requires a bearer key:
    # api_key_env: THREADIFY_AI_GATEWAY_KEY
    tls:
      ca_file: certs/gateway-ca.pem
      cert_file: certs/client.pem
      key_file: certs/client-key.pem
```

| Requirement | Settings |
| --- | --- |
| Private server CA | `ca_file` |
| Client certificate / mTLS | Both `cert_file` and `key_file` |
| Publicly trusted HTTPS, no mTLS | Omit `tls` |

Use PEM files and an unencrypted client private key. Paths are relative to the
Engine YAML, or absolute. The Threadify agent must be able to read them. TLS settings require
HTTPS; certificate and hostname verification stay enabled.

## Agent sessions

Each Engine started with `--with-agent` owns its agent. Conversations and pending
tool state are temporary and reset when that agent restarts. Saved contracts,
rules and other completed changes remain in Threadify.

Keep a conversation’s requests on the same Engine. Use a dedicated Engine address
or sticky routing behind a load balancer. Engines do not expose their agents for
other Engines to share.

## External agent

To connect directly to an independently managed compatible agent, omit
`--with-agent` and set:

```yaml
ai:
  agent:
    url: https://agent.example.com
```

Use an HTTP(S) origin without a path, credentials, query or fragment.
`THREADIFY_AGENT_URL` overrides YAML. Configure the agent’s Engine connection and
model gateway on that service; the Engine does not transfer model keys or
certificate files.

## Preinstall or work offline

```sh
threadify agent install-runtime --agent-cache-dir /var/lib/threadify/agent
threadify serve --config ./config.yaml --with-agent \
  --agent-cache-dir /var/lib/threadify/agent
```

For offline installation, copy the matching platform runtime archive from the
**same Engine release** and add `--agent-runtime-archive /path/to/runtime.tar.gz`
to the install command. The Engine checks it against its embedded checksum.
`THREADIFY_AGENT_CACHE_DIR` also sets the cache directory.

Restart after changing settings or rotating keys/certificates. If local setup fails,
fix the cause and restart the Engine; UI retry only checks status again. Private
agent diagnostics are under `logs/` in the runtime cache.


In containers, mount the YAML and certificates using container paths. For the
bundled agent they belong in the Engine container. `127.0.0.1` refers to that
container; use a reachable hostname for services elsewhere.


## Configuration rules

- `base_url` includes the API prefix, usually `/v1`, without `/chat/completions`.
- `model` is the gateway's exact model ID, including any namespace.
- `auth: custom` is the default. Personal gateways do not receive your Threadify license.
- With no `api_key_env`, no bearer header is sent. With it, the variable must be nonempty.
- YAML settings override legacy model environment settings when `THREADIFY_CONFIG_PATH` is set.
- Invalid configuration fails without switching providers. Redirects and environment HTTP proxies are disabled.
- Gateway configuration alone does not enable the agent. Select local or external mode above.

## Verify

Open a thread detail page and ask the agent:

> Read this page context and fetch this thread's details. Summarize its status. Do not change any data.

Confirm the page-context tool completes and a summary appears. Conversation text
and tool results are sent to your chosen gateway.

| Problem | Check |
| --- | --- |
| Missing `ai` section | `THREADIFY_CONFIG_PATH` points to the Engine YAML. |
| Connection refused | Host and port are reachable from the Threadify agent. |
| HTTP 404 | Correct API prefix; no duplicated `/chat/completions`. |
| HTTP 401/403 | Gateway key, client certificate and gateway access policy. |
| TLS error | CA, hostname, expiry, file permissions and certificate/key pair. |
| Tools fail | Model supports function calls and tool-result messages. |
| Agent unavailable | Runtime download/cache permissions for local mode; service address and reachability for external mode. |
| AI features hidden | Enable `--with-agent` or configure `ai.agent.url`; check `ai.enabled`. |
