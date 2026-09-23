# Threadify dashboard

A Vite/React single-page app embedded in the Threadify Engine. Open the Engine's
URL (default `http://localhost:8081`) to sign in. Production needs no Node server.
The public website and Registry signup live separately in [homepage](../homepage/README.md).

## Develop

Requires Node.js 24 LTS and a running Engine:

```sh
nvm install
nvm use
npm ci
THREADIFY_DEV_ENGINE_URL=http://127.0.0.1:8083 npm run dev -- --host 127.0.0.1
```

Open `http://127.0.0.1:3000`. Vite proxies API, authentication and WebSocket
requests to the Engine. Set `registry.browser_origin` to that exact development
origin. Keep browser and Engine hostnames consistent.

## Build and test

```sh
npm run test:routes
npm run test:engine-routing
npm run test:auth
```

`test:auth` builds `build/client` and checks the static shell and browser session
client. Engine CI runs these checks once, uploads the assets, and reuses that
artifact for validation and release compilation. UI changes trigger Engine releases.

For a local binary containing your UI changes, from the repository root:

```sh
(cd web && npm run build)
cp -R web/build/client/. threadify-go/internal/dashboard/dist/
python3 threadify-go/scripts/check-dashboard.py
(cd threadify-go && make build)
```

Generated assets are ignored by Git. Plain Go development builds still compile
without them; UI routes then return 503 with instructions. GoReleaser and source
Docker builds require prepared assets. The Engine Docker image runs the same
binary; there is no separate dashboard container.

See [Engine connection](ENGINE_CONNECTION.md) and
[browser authentication](../threadify-go/docs/BROWSER_AUTH.md) for deployment.

## Profile configuration

The profile type edit action opens `/u/profile-views/:type?tab=data`. The compact
view action opens the Presentation tab of the same editor. Data & metrics retains
the deterministic Custom Metric and From Template forms; saving a metric automatically gives it a presentation, including on entity
Overviews without a separate presentation save. Single values default to cards,
time groups to line charts, and other groups to tables. Existing choices remain
unchanged. Presentation blocks reference the saved metric
ID and choose card, table, line, or bar display without copying the calculation.
Both configurations belong to the profile type and apply to every entity of that
type. Previewing an entity does not change this scope.

The sidebar can propose a complete `profile-metrics` array for the data draft or a
`profile-view` definition for presentation. Proposals are validated and tied to the
current draft revision. Applying a proposal does not persist it. Data and
presentation have explicit save actions; presentation cannot be saved while data
changes are pending. Existing built-in views remain supported.

Computed metric responses retain legacy named results and include
`__metricResults`, an ID-to-rows map used by individual presentation blocks. All
blocks share a query scoped by workspace, type, current entity and date range.
The server rejects presentation references outside the same profile type. Removing a metric removes its default presentation. An unavailable result never
silently binds to another metric.

Run `npm run typecheck`, `npm run test:profile-views`, and `npm run build` for the
frontend checks. Backend reference and revision tests live in
`threadify-go/shared/domain` and `threadify-go/shared/repository`.
