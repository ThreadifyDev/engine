# Dashboard runtime configuration

The embedded dashboard uses `window.location.origin` for Engine requests.
A binary built in CI works at different deployment addresses without rebuilding
its JavaScript. There is no server-side JavaScript configuration injection.

Serve the Engine at the root of its hostname. Set `registry.browser_origin` to
the public HTTPS origin when a reverse proxy terminates TLS, and set
`server.public_url` to the advertised client address. See
[Engine connection](ENGINE_CONNECTION.md).

For local UI development, set `THREADIFY_DEV_ENGINE_URL` when starting Vite.
The development proxy forwards authentication, REST, GraphQL and WebSocket
requests while keeping browser cookies on the UI origin.

A custom separately served shell can explicitly set `window.__ENV__.ENGINE_URL`
before loading the app. This is an optional integration override,
not required by the embedded app. Cross-host cookie authentication still requires
a same-origin proxy; see [browser authentication](../threadify-go/docs/BROWSER_AUTH.md).
