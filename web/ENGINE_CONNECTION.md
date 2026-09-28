# Direct engine connection

The embedded dashboard calls the origin serving the Engine. Its authentication,
GraphQL, REST and WebSocket paths share one host and listener. Users do not need
to configure a separate dashboard API URL.

Profile-type configuration, metrics templates, API keys, service accounts, roles,
personal/company settings and Registry entitlement display are served directly
by the Engine under `/v1`. No Web API process is needed by the dashboard.

Open `/login` and choose email/SSO through Fused Registry, or exchange an API key
for a browser session. An API key retains its user or service-account permissions;
the configured license key bootstraps the account owner. Sessions last eight hours,
are stored in HttpOnly cookies, and use a separate readable CSRF cookie for
mutations. No bearer token is stored in browser localStorage.

For production, serve the Engine at the root of one HTTPS origin and configure
`registry.browser_origin` to match it. The embedded dashboard currently requires
a root-mounted deployment; SDK-only reverse-proxy path prefixes are independent.

For UI development:

```sh
THREADIFY_DEV_ENGINE_URL=http://127.0.0.1:8083 npm run dev -- --host 127.0.0.1
```

Set `registry.browser_origin: http://127.0.0.1:3000` in the local Engine config.
Vite proxies API requests to it. Run `npm run test:routes`,
`npm run test:engine-routing` and `npm run test:auth` for browser checks.

The **Team** page uses Engine-owned `invited`, `active`, `suspended` and `archived`
users. Administrators create invitations and share the UI sign-in link; Registry
sign-in with the invited email activates the user. The old Web API team and
invitation endpoints are retired. See [user lifecycle](../threadify-go/docs/BROWSER_AUTH.md#engine-owned-users-and-invitations).

## Public Engine address

An administrator can set the advertised address in **Settings → Engine** or use
`server.public_url` in the Engine's `config.yaml`. The saved UI setting overrides
the YAML value until **Use config default** is selected. The JavaScript SDK takes this one URL for both writes and queries. OTEL and MCP
addresses appear in their setup sections. The setting persists in PostgreSQL.
This address is for clients connecting to the installation; it does not switch
the current browser session or change the origin used by the embedded dashboard.

## External AI

The UI has no built-in AI assistant. Old `/u/assistant` links redirect to the
dashboard. Use [`examples/threadify-mcp-agent`](../examples/threadify-mcp-agent/README.md)
for conversational investigation through the engine's Streamable HTTP MCP
endpoint at `/mcp`. Model settings and sessions belong to that external agent;
the self-host engine config has no AI settings. Legacy chat code in the separate
Web API is not part of the engine binary or this UI flow.
