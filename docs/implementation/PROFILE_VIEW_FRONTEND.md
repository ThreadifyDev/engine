# Shared entity profile Overview views

Profile types retain their deterministic creation and metric configuration forms.
The optional "After saving" choice opens `/u/profile-views/:type` manually or with
the existing AI sidebar. Compact actions on each Entity Profile Type definition
and the type page opened by View profiles link to this designer. Individual
entity details do not show design actions.
A `ref` query parameter selects the preview entity; `ai=1` opens the sidebar without
automatically sending a message.

## Definition and rendering

The version 1 definition contains a title, description, card-column count, default
range, and ordered blocks. The finite source catalog supports profile summary
metrics, details, configured metrics, and delivery health. **Thread history is not
an Overview source** and remains in its dedicated tab.

The frontend and Go domain both validate the definition. The server independently
compiles the parameterized data request plan; clients cannot submit arbitrary
requests, code, credentials, or company/entity bindings. Existing authenticated
GraphQL operations resolve the current entity at render time. The renderer uses
supported components; there is no arbitrary-code or arbitrary-query executor.
Summary cards preserve backend reporting semantics and show unavailable data as
unavailable. The default range applies to computed metrics and delivery health.

The `deliveryHealthChart` block renders an accessible horizontal bar graph of
completion, failure, error, and recovery percentages using the same authenticated
delivery-health operation. It compares aggregate rates for the selected period;
it does not invent daily time-series data. Missing or invalid percentages remain
unavailable, and zero rates remain zero. The block is available both manually and
to AI proposals and is accepted by the server's request-plan compiler.

For a time-series graph, `deliveryTrendChart` plots completed and failed/cancelled
counts by UTC thread creation date, with an explicit zero baseline, both series,
date/count axes, and daily values available on hover or keyboard focus. The
delivery-health response supplies `daily_outcomes` from a fixed, company-scoped
query. It counts each thread once across matching reference keys and fills absent
days with zero over 7/30/90 calendar days, including a partial current day. These
are current outcomes of creation-day cohorts, not completion-event timestamps.
Unavailable daily data is never reconstructed from aggregate percentages.

## Backend contract

Both the combined Engine and standalone management API expose these routes:

- Engine: `GET` and `PUT /v1/entity-profile-views/:id`
- Management API: `GET` and `PUT /api/entity-profile-views/:id`

`:id` is the stable profile type UUID. GET requires `entity_profile_type.read`;
PUT requires `entity_profile_type.update`. Existing authentication, license, and
browser-session CSRF middleware apply. Tenant identity and the updating principal
come exclusively from verified request context. Other tenants, nonexistent types,
and archived types return 404.

GET returns `{ data, can_manage }`. Data contains `profile_type_id`, `definition`,
`requests`, `revision`, `updated_at`, and `updated_by`. A type without a shared view
has a null definition and revision 0; the details page uses its standard Overview.
Responses use `Cache-Control: no-store`.

PUT accepts only `{ definition, expected_revision }`, with a 32 KB body limit.
Unknown fields, unsupported sources (including history), invalid layouts, and
invalid revisions are rejected with 400. The repository validates again and
compiles the request plan. A single PostgreSQL UPDATE checks the expected revision
and increments it atomically; concurrent or stale writers receive 409 with code
`PROFILE_VIEW_CONFLICT`. A conflict never overwrites the current saved view.

The idempotent additive startup migration in `shared/database/profile_view.go`
adds JSONB definition/request columns and revision/update metadata to
`entity_profile_type`. Existing type fields, metrics, and identities are preserved.
The view survives a type rename because it is keyed by ID. Archiving the type
makes its view inaccessible. This stores the latest revision, not a historical
revision ledger.

## Frontend behavior

The designer loads the shared view and the server-provided edit permission. Save
explicitly updates the Overview for all users with access to this profile type.
Readers can preview drafts, but cannot save them. Failed loads do not enable
saving an assumed revision. Failed saves retain the draft. Conflicts offer an
explicit "Discard draft and load latest shared view" action. Edits made while a
save is in flight remain unsaved after that request completes.

Earlier browser-only views can be explicitly imported into the preview. They are
never published automatically or used as a hidden fallback on entity details.
Shared saves do not write localStorage. The old browser copy is retained as a
backup; history blocks in early drafts are removed during import.

## Agent integration

The mounted designer registers its current definition and draft revision with the
existing AgentProvider. With page context enabled, the agent receives the supported
catalog and schema, and returns a complete `profile-view` fenced JSON proposal.
"Apply to preview" validates it and checks its draft instance and revision.
Manual edits, saving, navigating away, or opening a different draft invalidate old
proposals. Application is separate from shared saving; the agent does not publish.

The `navigate_ui` tool advertises `profile_designer`; after navigating, the agent
can read `profileViewDraft.authoringInstructions` from `get_page_context`. With
page context disabled, no profile definition or proposal apply target is attached.

## Verification

Frontend, from `web` with Node 24:

```sh
npm run typecheck
npm run build
npm run test:profile-views
npm run test:agent
npm run test:routes
```

Backend:

```sh
(cd threadify-go/shared && go test ./domain ./repository ./management/handlers)
(cd threadify-go && go test ./internal/app)
(cd threadify-go/api && go test ./app)
# Creates and drops a private test schema in the supplied local test database:
(cd threadify-go/shared && THREADIFY_TEST_DATABASE_URL='<test-dsn>' go test ./repository -run '^TestProfileViewPostgres$' -count=1)
# Uses disposable PostgreSQL/Valkey and a local Registry fixture:
(cd threadify-go/tests && THREADIFY_E2E_BINARY='<built-engine>' go test ./e2e -run '^TestEmbeddedDashboard$' -count=1 -timeout=5m)
```

Verified against PostgreSQL: migration on existing rows and repeated migration,
readback from another repository instance, tenant isolation, concurrent saves,
stale revisions, type rename, and archive behavior. Authenticated Engine E2E checks
shared reads, persistence, reader write denial, server-derived plans, history
rejection, stale saves, and missing-CSRF rejection.

Live browser verification on 2026-09-22 passed: explicit local import, shared save,
reading the saved view in another editor, stale two-tab save rejection and recovery,
and the entity Overview rendering the backend-saved view.

The complete live AI flow also passed on 2026-09-22: the sidebar agent generated a
valid proposal titled "Customer delivery health" with only delivery health and a
30-day default; Apply to preview rendered real entity data; Save shared view
persisted revision 3; a full reload of the entity Overview displayed that title,
description, and live delivery health (score 75, two threads, 50% completion).
Thread history remained exclusively in its separate tab. This is the current
saved view for the local E2E Customers sample.
