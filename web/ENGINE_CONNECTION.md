# Direct engine connection

The UI uses two public runtime URLs:

- `ENGINE_URL` (default `http://localhost:8081`) serves browser authentication at
  `/auth`, GraphQL at `/graphql`, contracts at `/v1/contracts`, and user/invitation
  management at `/v1/users`.
- `API_URL` (default `http://localhost:3001`) serves profile and service-account management,
  entity-profile type configuration, and other Web API routes under `/api`.

Both services must use the same PostgreSQL database, Registry license, and
installation ID. Set `registry.browser_origin` (or `THREADIFY_BROWSER_ORIGIN`)
to the exact UI origin on both services. The Web API's CORS configuration must
also allow that origin.

Open `/login` and choose email/SSO through Fused Registry, or exchange an API key
for a browser session. An API key retains its user or service-account permissions;
the configured license key bootstraps the account owner. Sessions last eight hours,
are stored in HttpOnly cookies, and use a separate readable CSRF cookie for
mutations. No bearer token is stored in browser localStorage.

For production, serve the UI, Engine routes and Web API routes behind one HTTPS
origin. Local development can use different ports on the same hostname; do not
mix `localhost` with `127.0.0.1`. Cross-host UI/API deployments require a same-origin
proxy because session cookies are host-only.

```sh
npm run build
API_URL=http://127.0.0.1:3003 ENGINE_URL=http://127.0.0.1:8083 \
  HOST=127.0.0.1 PORT=3002 npm start
```

Set `registry.browser_origin: http://127.0.0.1:3002` in the local Engine and Web API
configuration. Run `npm run test:engine-routing` and `npm run test:auth` to check
request routing, cookie/CSRF transport and retired password routes.

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
the current browser session or replace the UI deployment's `ENGINE_URL` setting.

## External AI

The UI has no built-in AI assistant. Old `/u/assistant` links redirect to the
dashboard. Use [`examples/threadify-mcp-agent`](../examples/threadify-mcp-agent/README.md)
for conversational investigation through the engine's Streamable HTTP MCP
endpoint at `/sse`. Model settings and sessions belong to that external agent;
the self-host engine config has no AI settings. Legacy chat code in the separate
Web API is not part of the engine binary or this UI flow.
