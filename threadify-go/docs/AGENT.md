# Optional Threadify Agent

The Engine embeds the agent artifact. Its runtime is installed only when local AI
is enabled; external agents do not install dependencies in the Engine.

| Configuration | Behavior |
| --- | --- |
| No local flag or external URL | Agent disabled; all agentic UI features hidden |
| `--with-agent` | Install missing runtime, then run the local agent |
| `ai.agent.url` | Connect to an existing agent |

Selecting local and external modes together is an error. `ai.enabled: false`
disables the agent and cannot be combined with `--with-agent`.
When configured, the browser shows one agent launcher on every signed-in page.

## Local agent

Add a gateway to the Engine YAML:

```yaml
ai:
  gateway:
    base_url: http://127.0.0.1:11434/v1
    model: your-installed-tool-capable-model
```

```sh
threadify serve --config ./config.yaml --with-agent
```

The Engine downloads the platform runtime from its own GitHub release, verifies
the embedded SHA-256 checksum, and reuses the cached runtime on later starts.
The gateway must support Chat Completions, streaming and function tools. No
OpenAI account or separate Threadify service credential is required for local mode.

`--agent-cache-dir` overrides `THREADIFY_AGENT_CACHE_DIR` and the default user
cache directory. The cache contains executable runtime files; give it only to
trusted processes. The Engine manages agent startup and shutdown. A failed
runtime download or agent startup leaves the rest of the Engine available.
After fixing an installation/startup failure, restart the Engine to try again.
UI retry only rechecks status. Private agent diagnostics are in the cache
`logs/` directory.

For Docker, append `serve --with-agent` after the image name. Keep `/data` on a
persistent writable volume; the image sets the cache to `/data/agent`. Configure
gateway hosts that are reachable from inside the container.

## Preinstall and offline use

The provisioner can populate the cache without starting the Engine or connecting
to its database:

```sh
threadify agent install-runtime --agent-cache-dir /var/lib/threadify/agent
```

For offline hosts, copy `runtime-<os>-<arch>.tar.gz` from the **same Engine release**:

```sh
threadify agent install-runtime \
  --agent-cache-dir /var/lib/threadify/agent \
  --agent-runtime-archive /path/to/runtime-linux-amd64.tar.gz
threadify serve --config ./config.yaml --with-agent \
  --agent-cache-dir /var/lib/threadify/agent
```

The same archive option can be supplied directly to `serve --with-agent`.
Checksums remain mandatory for offline archives. Runtime versions use separate
cache directories so an Engine upgrade cannot replace a running installation's
runtime. Do not remove a runtime while an agent is using it.

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

## Gateway credentials and certificates

```yaml
ai:
  gateway:
    base_url: https://ai.example.com/v1
    model: your-model-id
    api_key_env: THREADIFY_AI_GATEWAY_KEY
    tls:
      ca_file: certs/ca.pem
      cert_file: certs/client.pem
      key_file: certs/client-key.pem
```

All key and TLS options are optional. Set the named key in the Engine environment
for local mode, or in the external agent environment. Use both client certificate
and key for mTLS; use only `ca_file` for a private server CA. Files must be PEM,
with an unencrypted private key. Relative paths resolve beside the Engine YAML.
Restart after changing gateway configuration or rotating credentials.

See the [model configuration guide](https://github.com/ThreadifyDev/engine/blob/main/docs/PERSONAL_AI_GATEWAY.md).

## Source builds

Release CI builds all three native runtime archives and stages their checksums
before compiling the Engine. Direct GoReleaser runs reject missing or mismatched
release inputs. Ordinary `go build` does not need all platform runtimes.

A development binary or source Docker build has version `dev`; use a preinstalled
runtime or `--agent-runtime-archive` matching that build's embedded platform
checksum. Development builds do not download an arbitrary latest runtime.
