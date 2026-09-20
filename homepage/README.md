# Threadify public site

Standalone marketing site for `threadify.dev`. The dashboard lives in `../web`
and is embedded in Engine. This package builds independently
and needs no Engine, database, or legacy web API to render its public pages.

## Public positioning

Threadify is execution intelligence: a shared referee for work across services
and AI agents. Explain the product through recorded work, explicit rules, and
the evidence behind each result. Teams keep their existing workflows and choose
where their applications must wait for permission.

Keep this story consistent across the landing page, audience cards, licensing,
signup, metadata, and public AI guides. Describe entity profiles as connected
execution history; avoid promises of automatic customer-health conclusions.
Telemetry alone does not stop actions or execute responses. Preserve the existing
theme and section layouts when updating the copy.

## Run

```sh
nvm use
npm ci
npm run dev  # http://localhost:3004
```

```sh
npm run typecheck
npm test
PORT=3004 npm start
```

`npm test` builds the production app and tests page isolation and Registry signup.
Homepage CI runs independently when this directory changes.

## Registry signup

`/signup` uses the same Registry `POST /internal/signup` contract as Fused's
homepage: send an email code, verify it, then display the one-time license key.
Threadify owns its signup form and styling. It collects the account name, email,
and verification code; the server fixes `products` to `["threadify"]` and rejects
requests for other products or cloud provisioning. No product selection is shown.
For existing accounts, operators add Threadify through Registry's
`PUT /admin/accounts/{account_id}/products` with `products: ["threadify"]`.
This preserves existing Fused access and the account's license key.

Set these **runtime** environment variables on the homepage server:

| Variable | Purpose |
| --- | --- |
| `BACKEND_URL` | Fused Registry URL; defaults to `https://registry.usefused.com` |
| `FUSED_SIGNUP_TOKEN` | Registry's scoped signup credential, server only |
| `RECAPTCHA_SITE_KEY` | Public reCAPTCHA v3 site key, registered for the homepage domain |
| `RECAPTCHA_SECRET_KEY` | Server-only reCAPTCHA verification credential |
| `RECAPTCHA_MIN_SCORE` | Minimum accepted score; defaults to `0.5` |

Use the same scoped signup integration as Fused, not an administrator or user
license key. Secrets never enter client bundles. Production requires reCAPTCHA;
local development allows it to be omitted. Missing signup configuration disables
the submit button while the form and marketing pages remain visible.

License responses use `Cache-Control: no-store`; the key stays in page memory.
Users add it to `registry.license_key` in their Engine's `config.yaml` and sign in
to the dashboard at their own Engine URL. The homepage does not authenticate
dashboard sessions.

## Deploy separately

Configure the public site's build root as `homepage`, build with `npm ci && npm run
build`, and start with `npm start`. Supply the variables above to that deployment.
The dashboard is served by the Engine; this container serves the public site.

From the repository root:

```sh
make -C threadify-go build-web-ui
docker run --rm -p 3004:3000 --env-file /path/to/homepage.env intelijence/th_web_ui:latest
```

The Make target tags both the current version and `latest`. Override the image
name with `WEB_UI_IMAGE=your-namespace/th_web_ui` (or set `DOCKER_NAMESPACE`).
The existing `push-web-ui` target pushes those local tags; `publish-web-ui`
builds and pushes Linux amd64/arm64 images with Docker Buildx.
The optional `th_web_ui` example in the root `docker-compose.yml` uses the same
homepage build context and runtime Registry settings.

Public assets (`AI.md`, SDK guidance, robots.txt, sitemap) are served by this site.
