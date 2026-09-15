# Registry integration validation

Validated on 2026-09-15 using disposable local PostgreSQL/pgvector and Valkey.
No production account, service, billing ledger, or deployment was changed.

## Passed

- Full Threadify shared, Engine, and management API Go test suites.
- Registry API, repository, configuration, models, schema, and command tests;
  focused race tests and `go vet` on touched packages.
- Real PostgreSQL Registry persistence tests: product selection, independent
  heartbeat/expiry, atomic report batches, duplicate retries, conflicting retry
  rollback, installation isolation, and product updates.
- Threadify Registry race tests with real PostgreSQL: concurrent bidirectional
  allowances, rate windows, month rollover, zero/unlimited semantics, restart,
  stable installation identity through key rotation, reporting retry, profile
  concurrency, and verification failure/recovery.
- HTTP denial tests preserve 429 before output/application bytes are admitted.
- Signed forwarding tests reject replay and modified bodies, credentials, paths,
  content type, and signatures. API never trusts incoming forwarding proof.
- API-to-Engine proxy test uses actual handlers and shared durable accounting:
  input/output bytes, request counts, and response counts are recorded once;
  unknown proxy-like paths remain metered.
- Automatic profile creation test preserves thread references at the profile cap
  and updates existing profiles without creating duplicates.
- Compiled Engine workflow E2E: contract creation, threads, entity profiles, OTLP
  conversion/completion/replay, persistence, and health with a signed fixture.
- Compiled Engine against actual Registry handlers and separate test databases:
  handshake, heartbeat, all four usage metrics persisted in Registry, outbox
  acknowledgement, then restart with threads and bandwidth counters retained.
- Fused homepage signup tests (15 cases/subtests) and production build.
- Threadify dashboard production build with live Registry allowance display.

The binary restart test waits for the CLI readiness probe during recovery;
transient initial persistence readiness no longer causes a false-negative test.

## Unhappy-path validation

The user approved the Fused protected-API product guard on 2026-09-15. It is now
installed in the existing authenticated management/catalogue route group, and
its compatibility and failure tests pass. No approval blocker remains.

- Registry: 51 additional Threadify cases cover missing, invalid and revoked
  keys (including cache invalidation), independent product suspension, malformed
  or oversized requests, bad signatures, stale/future timestamps, invalid usage,
  conflicting retries, unavailable storage, and failed writes without success
  acknowledgement.
- Fused guard: 13 cases cover legacy, Fused, combined-license and anonymous
  discovery compatibility, plus denied Threadify-only licenses, suspended
  products, revoked keys and unavailable credential/account storage.
- Real PostgreSQL: a forced constraint failure after an earlier valid report
  insertion rolls back the entire Registry batch. Threadify counter changes
  likewise roll back if the durable outbox insertion fails.
- Threadify startup: failed handshake or explicit license denial never returns a
  usable runtime; invalid licenses cannot provision a company. A transient initial
  heartbeat failure retains a successful handshake. Restart rejects
  changed account, company or installation bindings without altering old data.
- Verification: malformed/trailing/oversized JSON, invalid limits, duration
  overflow and changed identity cannot replace a valid snapshot. Heartbeat
  failures retain the last verified in-memory limits indefinitely; explicit
  suspension or revocation still denies access. Recovery replaces the limits.
- Usage acknowledgement: missing, null, negative, impossible and malformed
  acknowledgements retain queued reports. A duplicate retry with zero newly
  accepted reports remains valid.
- Management API: foreign/missing company identities, invalid invitations,
  unauthorized owner signup, suspension and tampered/replayed proxy
  proofs reject before application mutation. Rejected proxy proofs remain
  metered. Recovery restores valid requests.

These tests exposed and fixed four classes of issue:

1. Credential database failures were incorrectly reported as invalid licenses;
   Registry now returns 503, preserving the client's last verified policy.
2. An invitation to a deleted company could proceed toward a nil dereference;
   signup now returns a company-not-found error.
3. Threadify accepted a valid JSON prefix, overflowing verification durations
   and unexpected workspace changes; response validation now rejects them.
4. Incomplete usage acknowledgements could delete durable reports; the client
   now requires a valid explicit accepted count before deletion.

Validation reran the full Engine suite, full shared suite with the race detector, full management
API suite (14 tested packages and 29 compile-only packages), focused API/app and
signup race tests, Registry API/middleware/repository/command race suites,
Registry `go vet`, and live PostgreSQL persistence tests.

Registry plan allowances still require operator configuration; unspecified
Threadify plans intentionally return zero allowances. This validation used only
disposable local databases and test licenses and did not deploy either product.

## Live follow-up

See `REGISTRY_LIVE_VALIDATION.md` for the subsequent compiled-Engine and real
Registry-handler HTTP runs. Those checks also exposed and fixed a missing
account-wide suspension condition in the new Fused product-access middleware.
Immediate restart during a reporting outage also exposed a persistence shutdown
ordering race; the fix passed four race-tested regressions, the full Engine
suite and both live restart scenarios. Final Registry and Engine usage totals
matched exactly, and the disposable test services were removed.
