# Threadify CLI

The CLI is maintained in its own repository, `ThreadifyDev/cli`, checked out
alongside the Engine and SDKs at `threadify-cli/` in the Threadify workspace.
See [the CLI guide](../../threadify-cli/README.md) in a source checkout, or the
[standalone repository](https://github.com/ThreadifyDev/cli) once published.

The Engine binary runs the server. The client-only binary is `threadify-cli`:

```sh
threadify-cli config set api-url https://threadify.example.com
threadify-cli login
threadify-cli contracts create --file contract.yaml
```

The Engine retains `/auth/cli/*`, profile management, contracts, GraphQL and
WebSocket endpoints. The CLI owns its Go module, unit tests, native-platform
smoke tests, packaging and release CI. It builds without Engine source.

Engine-side compatibility checks accept a separately built client:

```sh
# From threadify-go/tests:
THREADIFY_E2E_BINARY=/absolute/path/to/threadify \
THREADIFY_E2E_CLI_BINARY=/absolute/path/to/threadify-cli \
  go test ./e2e -run '^TestStandaloneWorkflows$/^management_cli$' -v -count=1
```

The CLI workflow performs this compatibility test by checking out an explicit
Engine repository/ref. Interactive login also requires the updated external UI
and the Engine's configured `registry.browser_origin`.

## Optional agent commands

```sh
threadify serve --config ./config.yaml --with-agent
threadify agent install-runtime --agent-cache-dir /var/lib/threadify/agent
```

`--with-agent` enables local AI. `agent install-runtime` only prepares its cache
and requires no Engine configuration or database. Both accept
`--agent-runtime-archive` for a matching offline release archive.
See [agent setup](AGENT.md) for external agents and configuration.
