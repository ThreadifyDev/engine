# Threadify SDK — Coding Guide

## Product model

Threadify is a shared referee for work across services and AI agents. Keep the
application's workflow in its existing code. Use Threadify to record execution,
check explicit rules, and provide evidence for decisions.

- A thread is a workflow execution, optionally bound to a contract.
- A step records an action and its outcome.
- A contract describes approvals, ordering, timing, and valid submitted details.
- Recording telemetry does not automatically stop an action or mean validation passed.

## Connect to the team's Engine

Use an Engine API key, not its Registry license key. Supply the team's Engine URL;
there is no shared production Engine endpoint to assume.

```javascript
import { Threadify } from '@threadify/sdk';

const connection = await Threadify.connect(apiKey, 'payments', {
  wsUrl: 'wss://your-threadify-engine.example/threads',
  graphqlUrl: 'https://your-threadify-engine.example/graphql'
});

// Track an execution without rules.
const observed = await connection.thread('Refund-4821', { label: 'Refund-4821' });

// Or bind the rules for a workflow that needs validation.
const guarded = await connection.thread('Refund-4822', { label: 'Refund-4822', contract: 'refund_review:1' });
```

Use `connection.thread(threadKey, { label, contract, refs, tags, serviceName, role })`.
The application supplies a durable session or process key; Threadify manages the
internal ID. Later requests call `connection.thread(threadKey)` to resume with
the stored contract and pinned version. Options are creation defaults; conflicting
contracts are rejected. Initialize contracted sessions before telemetry begins,
because a new key without a contract creates a free-form thread. Closed threads
cannot resume or accept writes. Use `threadify.thread_key` for the same identity
in OpenTelemetry instrumentation.

## Record evidence

```javascript
await observed.step('refund_requested')
  .idempotencyKey('refund-4821-request')
  .addContext({ payment_reference: 'PAY-12345678', amount: '19.99' })
  .success();
```

Use `.addContext(...)` for the submitted details. Context is a flat map of string
values; use stable field names that match the contract. Report success, business
failure, or technical error accurately. Do not record a success for an action
that did not complete.

## Require an explicit check where needed

```javascript
// The contract must define this step and the caller's role.
await guarded.waitFor('refund_issued', { timeout: 10000 });

// This is the application's action, not an action run by Threadify.
const outcome = await issueRefund();
await guarded.step('refund_issued')
  .addContext(outcome)
  .success('Refund issued', { waitFor: true });
```

Do not continue after a rejected or timed-out permission request. A name-only
permission check cannot validate future content. Content checks happen on
submission; awaiting validation afterward cannot undo the external action.
Use the same thread instance to report the granted invocation.

Gherkin is the primary contract format. It supports regex checks and comparisons
with earlier successful steps. Require a fresh approval before every invocation
when retries must not reuse an earlier approval. Keep repeatable steps nonterminal.

## Read results and respond deliberately

```javascript
connection.subscribe('rule.violated', 'refund_issued', notification => {
  console.log('Review rule violation:', notification.severity);
  notification.ack();
});
```

The application chooses whether to alert a team, retry, or take another action.
Treat the recorded history as evidence, not as proof of unobserved activity.
Entity profiles can connect history across runs for a customer, agent, or partner.

## Guides

- [Core concepts and Gherkin examples](https://threadify.dev/AI.md)
- [JavaScript SDK syntax](https://threadify.dev/AI-javascript.md)
- [Python SDK syntax](https://threadify.dev/AI-python.md)
- [Go SDK syntax](https://threadify.dev/AI-go.md)
