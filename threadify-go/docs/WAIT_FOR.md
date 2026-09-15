# Optional execution waits

The Engine continues validating reported events through its asynchronous worker
pool. The JavaScript SDK offers two explicit waits. Both use the existing
WebSocket connection and the same compiled contract.

## Before execution

```js
const grant = await thread.waitFor('charge_payment', { timeout: 10000 });
// Execute only after this promise resolves.
const outcome = await chargePayment();
await thread.step('charge_payment').addContext(outcome).success();
```

`waitFor(stepName)` waits for flow eligibility and atomically claims one
invocation. It checks authenticated thread write access, the step's owner role,
thread status and maximum duration, entry points, successful prerequisites,
and explicit transitions. Known pending validations prevent a new permission.
It needs no repeated action payload. Future input checks and execution-duration
checks still need the actual event and cannot be guaranteed by a name-only wait.

The next report for that step on the same `ThreadInstance` carries the grant's
`invocationId` automatically. A grant does not record a successful step. One
unreported invocation per step is allowed; contracts with explicit transitions
serialize guarded invocations across the thread until validation completes.
Different independent steps in dependency-only contracts can be claimed together.

A missing prerequisite or outstanding invocation waits. Unknown steps, wrong
roles, ended threads, unavailable state and expired thread duration reject.
A rejected promise must not be treated as permission.

## After reporting an outcome

```js
const result = await thread.step('charge_payment')
  .addContext({ amount: 100, currency: 'GBP' })
  .success('Charged', { waitFor: true, timeout: 10000 });

console.log(result.stepId, result.validation.decision); // passed

await thread.step('notify_customer')
  .failed('Provider unavailable', { waitFor: true });
```

The payload is sent once. The Engine holds that submission's response until the
worker produces the validation result for its **event ID**. It returns the event
ID and validation together, never an unrelated same-name notification. The worker
runs validation once. Normal reports without `waitFor` return their recording
acknowledgement immediately.

`passed` means the reported outcome satisfies the contract. A failed execution
can pass validation. A violation rejects with `THREADIFY_VALIDATION_VIOLATED`
and an attached `validation` result. Missing/unavailable results and threads
without a contract reject with `THREADIFY_VALIDATION_UNAVAILABLE`; they are not
reported as passed. Waiting after execution does not undo the external action.

Context stays in `.addContext(...)`; the first `.success()`/`.failed()` argument
retains its existing meaning as message/metadata. Existing content rejections
happen before the Engine records an event.

If the Engine accepted the event but its wait deadline expires, the response
includes `stepId` and a timed-out validation decision; the SDK rejects with that
`stepId` attached. Resume without reporting the event again:

```js
await thread.waitForValidation('charge_payment', error.stepId, { timeout: 30000 });
```

## Fresh prerequisites for repeated calls

```gherkin
Rule: Charge
  When step "charge_payment" is submitted
  Then owner must be "processor"
  And step "approval" must succeed before each invocation
```

This differs from `step "approval" must have succeeded`, which accepts any prior
successful approval. The new clause requires approval validation **after the
previous invocation of charge_payment**. Ordering uses the Engine's atomic
sequence, not timestamps supplied by callers. The clause is compiled as
`fresh_depends_on` and also implies a normal dependency.

Each grant consumes eligibility immediately. Concurrent callers cannot share
one approval. Failed outcomes and abandoned grants consume it too. A fresh
approval must be recorded before another charge can proceed. Unguarded reported
calls are checked against the same rule and can be marked violated.

A repeated step must not be a terminal step. Declare a separate explicit terminal
step when a workflow permits several charges. The existing GraphQL `proposeStep`
is advisory and directs callers to `waitFor` for per-invocation prerequisites.

## Timeout, cancellation and uncertain outcomes

Both waits accept `timeout` (1–300000 milliseconds, default 10000) and an
`AbortSignal`. The Engine enforces the wait deadline. The SDK allows one additional
second for the final response to arrive, then treats a missing response as a
transport timeout. Abort and local timeout send a cancellation for that pending
request; disconnect also cancels all waits on the socket. Listeners and server
subscriptions are released. Cancellation stops waiting; it does not retract an
already recorded event or undo a grant. No timeout grants permission implicitly.
If transport fails before the final response arrives, its event ID may be unknown;
retain the original idempotency key and reconcile the uncertain submission.

If you decide not to execute after obtaining a grant, close it explicitly:

```js
const grant = await thread.waitFor('charge_payment');
await grant.cancel();
```

Cancellation does not restore a consumed approval. Once the event is reported,
its validation must finish; cancellation cannot discard that event.

A lost permission response has an uncertain outcome. The error includes the
`invocationId`; repeat `waitFor(stepName, { invocationId })` to recover the same
unreported grant, then execute it once or cancel it. Do not generate a new ID and
assume the earlier grant disappeared. Applications recovering after a process
crash should persist the invocation ID before sending the request and supply it
through this option. A repeated permission request does not make an external
side effect idempotent.

Outstanding claims and unresolved validation barriers have no automatic expiry.
A crash before reporting requires recovery/cancellation of the claim. A dropped
worker job, storage failure, or crash during validation cannot turn an
unvalidated event into permission; it must be reconciled before the thread can
safely proceed. Completed validation results are retained in Valkey for seven
days. Missing or incomplete live state fails closed; PostgreSQL's asynchronous
archive is not used as a potentially stale permission source. Preserve Valkey
state alongside the Engine's other live workflow state.

## OpenTelemetry funnel

When an existing exporter records the outcome, correlate the tool/service span
with the permission rather than sending the content again:

```js
const grant = await thread.waitFor('charge_payment');
span.setAttribute('threadify.thread_id', thread.id);
span.setAttribute('threadify.step_name', 'charge_payment');
span.setAttribute('threadify.invocation_id', grant.invocationId);
// Perform the operation; the span carries its normal outcome and context.
```

Both the JavaScript exporter and Engine OTLP ingestion accept
`threadify.invocation_id` as a control attribute, not business content. The
outcome must use the same authenticated owner as the grant. Apply this attribute
to the individual operation span, not a resource or parent span shared by several
invocations. Reporting a span after execution remains observational; the
application's explicit wait before execution supplies the gate.

## Protocol and latency

The SDK sends one `waitFor` WebSocket request with `await: true`, `timeoutMs`,
`threadId`, `stepName`, a UUID `invocationId`, and a response `requestId`.
Adding `stepId` waits for a previously reported validation result;
`cancel: true` closes an unreported grant.
A report with `waitFor: true` receives its validation in the **original submission
response**, with no second validation request.

Pending responses run separately from the connection's reader, so new events can
arrive on the same socket. The Engine subscribes to Valkey changes before rereading
state, avoiding a lost wake-up if validation completes during registration. It
rechecks authoritative state on a change and sends one final response when ready.
There is no SDK polling or periodic server-side state polling. Thread expiry uses
a single deadline timer. A failed notification connection returns unavailable.

Waits are bounded to 32 pending requests per WebSocket and 256 per Engine; excess
requests are rejected before an event is accepted. `cancelWait` with a
`targetRequestId` cancels only a pending request on that same connection.
Ready permission checks return immediately without waiting for archival or
notifications. All wire messages use normal Registry accounting.

Legacy protocol clients can omit `await` to request a single immediate snapshot.
The JavaScript SDK requires final responses and reports a clear error if an older
Engine returns a pending snapshot; it never falls back to polling.

Verification includes a live Node SDK/compiled Engine test with disposable
PostgreSQL, Valkey and embedded NATS, plus atomic competing-claim, invalid-state,
contract round-trip, timeout, cancellation and request-correlation tests.

### Deadline and accounting behavior

The SDK applies `timeout` to the caller's full wait, without transport grace,
and requests server cancellation when it expires. JavaScript event-loop stalls
can still delay timer delivery. Late responses are ignored. An unknown outcome
must be reconciled using the retained invocation ID or report idempotency key;
local timeout never undoes an accepted event or already granted invocation.
The Engine's budget starts when the frame is read, before input accounting.

WebSocket input request/byte increments and output message/byte increments each
use one atomic accounting transaction. Conditional bucket updates serialize
against concurrent writers in PostgreSQL; usage counters and reporting entries
commit together. Entitlements remain in memory. Request execution order remains
unchanged.
