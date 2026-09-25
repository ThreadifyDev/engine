# Execution waits

This resource is bundled with the contract-design skill. Load it only when a
user asks about guarding an action or waiting for validation. The authoritative
Engine guide is `threadify-go/docs/WAIT_FOR.md` in the Threadify source tree.

An SDK `waitFor(stepName)` claim checks flow eligibility and atomically reserves
one invocation before an external side effect. A read-only `can` or `next`
decision is advisory and does not grant permission. A successful claim does not
record a step; report the action's actual outcome afterwards.

In the JavaScript SDK:

```js
const grant = await thread.waitFor('charge_payment', { timeout: 10000 });
const outcome = await chargePayment();
await thread.step('charge_payment').addContext(outcome).success();
```

Use `.success(message, { waitFor: true })` or `.failed(message, { waitFor: true })`
to await validation of that exact reported event. If that response times out
after the event was accepted, recover with
`thread.waitForValidation(stepName, error.stepId)`; do not report the event again.
Cancel an unreported grant when abandoning it. A cancellation does not restore a
consumed fresh prerequisite.

The Python and Go SDKs have their own method names and timeout units. Load the
SDK guidance skill and its language reference before giving those examples.
