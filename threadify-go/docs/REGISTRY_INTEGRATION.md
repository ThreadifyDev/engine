# Threadify and Fused Registry

## Ownership and provisioning

A Registry account selects `fused`, `threadify`, or both. One license key can
serve both products; each product has separate access, suspension, entitlements,
and runtime verification. Existing Registry accounts default to Fused access.
The existing account `plan` selects allowances for both products; there is no
separate Threadify plan selector.

Threadify connects directly to Registry, using:

- `GET /api/threadify/handshake`
- `POST /api/threadify/heartbeat`
- `POST /api/threadify/usage-reports`

Requests carry the license as a Bearer token and a stable
`X-Threadify-Installation-ID`. POST bodies are signed using HMAC-SHA256 with the
license key and `X-Threadify-Signature: sha256=<hex>`. Reports have stable IDs;
Registry accepts identical retries without incrementing totals and rejects
conflicting reuse of an ID.

## Configure a deployment

Provision a Threadify-enabled key in Registry. Supply the same values to the
Engine, management API, and any standalone archiver sharing its database:

```sh
export THREADIFY_REGISTRY_URL='https://your-registry.example'
export THREADIFY_LICENSE_KEY='<the provisioned license>'
```

Equivalent configuration:

```yaml
registry:
  url: https://your-registry.example
  license_key: $THREADIFY_LICENSE_KEY
```

For an existing company, set `THREADIFY_COMPANY_ID` (or `registry.company_id`)
explicitly on the first startup to bind that company. Otherwise, the local
company uses Registry's account ID. Once bound, a database rejects changes to
its account/company identity. Back up an existing deployment before migration;
this integration does not merge existing companies or move their data.

The management API lets Registry's owner email register into the provisioned
company using the existing email-verification flow. Additional people need a
team invitation. Existing local identities for other companies cannot use the
licensed deployment.

## Limits and measurements

The full handshake and heartbeat snapshots supply:

| Field | Meaning |
| --- | --- |
| `input_bandwidth_bytes` | Admitted input body/frame bytes per UTC calendar month |
| `output_bandwidth_bytes` | Admitted output body/frame bytes per UTC calendar month |
| `input_requests_per_second` | HTTP requests and incoming WebSocket messages per second |
| `entity_profile_limit` | Current stored entity-profile count |

Bandwidth measures bytes per month; rate limits count incoming requests/messages per second.
There is no outgoing response/message rate limit or per-second byte allowance.

`-1` means unlimited; `0` means no allowance. Values are configured through
Registry plan definitions. They are held only in process memory, refreshed by
heartbeat, and never written to Threadify's database. Missing production
licensing configuration fails startup. Heartbeat failures never expire a running
Threadify instance: it continues enforcing its last verified in-memory limits
and retries Registry in the background. Registry's grace/freshness status is
informational. Explicit suspension or license revocation still denies access.
A fresh process needs one successful handshake because limits are not persisted;
a failed subsequent initial heartbeat does not invalidate that handshake.
Health and metrics endpoints remain available for diagnosis.

Bandwidth counters are reserved atomically in PostgreSQL before application
admission or output, together with a durable reporting outbox. Multiple Engine,
API, and archiver processes sharing one database share that accounting. Separate
databases have separate accounting; this does not implement a global spending
reservation service across independent installations. Counters survive restarts
and license-key rotation. Rejected operations do not refund already admitted
traffic. HTTP headers and transport overhead are not included.

Every management API request is metered, including local validation errors.
Forwarded Engine requests carry a signed, body-bound, single-use proof so Engine
can avoid counting the same traffic twice. Direct Engine traffic is metered there.
The proof does not bypass Engine authentication, account binding, or license checks. Streaming output can stop at a limit after headers/body chunks
have already been delivered; the initial denial returns 429 when no response has
been committed. Request-rate checks and payload-byte checks are separate.

Profile creation is serialized against the count in the same database transaction
as insertion. Existing profiles can be updated after a plan downgrade. Explicit
creation is rejected at the cap. Automatic profile materialization skips new
profiles when full while retaining the thread and updating existing profiles.

## Removed billing gates

Thread creation, event insertion, token usage, seats, and contract operations no
longer debit local credits. There is no thread-count or commercial per-thread-size
quota. Database capacity limits retained data; self-hosted capacity is managed by
the operator, while a cloud deployment must provision its purchased database
capacity. Transport parsing/safety limits remain separate from pricing.

Historical credit tables are retained without serving as entitlement sources.
Signup no longer seeds credits, the Engine no longer runs its local billing cron,
and licensed management API checkout/spending-limit endpoints direct users to
Registry. `GET /api/billing/plan` returns live Registry entitlements. Payment
collection and a Threadify cloud storage-provisioning service are not added by
this integration.

## Validation

Run shared, Engine, and API suites in their respective Go modules. The
`shared/registry` database integration tests use an explicitly supplied disposable
PostgreSQL URL and isolated schema. Compiled binary tests in `cmd/server` and
`tests/e2e` use a local signed Registry fixture and no credit-account seed.
The standalone restart test asserts persisted threads and non-reset bandwidth
counters. It also supports an isolated real Registry handler fixture through
`THREADIFY_SMOKE_REGISTRY_URL`; its fixed test account/key must match the fixture.
