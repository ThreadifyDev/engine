# Gherkin-style contracts

Threadify accepts a small, deterministic Gherkin-style language for authoring
contracts. Gherkin is the primary format. YAML is retained temporarily during
validation and migration; it is intended to be removed as an authoring format
in a subsequent change. Existing stored contracts are not converted by this change.

This is a Threadify rule language, not a Cucumber test runner. Only the exact
sentences below are supported. Unknown sentences, scenarios, tags, tables,
duplicate declarations, and trailing text produce errors. Core flow rules are
deterministic; an optional classifier can advise during preview.

## Example

```gherkin
Feature: payment_processing
Version: 1
Description: Record approved payments with valid details.

Rule: Record approval
  When step "approval" is submitted
  Then owner must be "reviewer"
  And this step is an entry point

Rule: Record a charge
  When step "charge" is submitted
  Then owner must be "payment_processor"
  And step "approval" must have succeeded
  And content "amount" must be a number greater than 0
  And content "currency" must be one of "GBP", "USD", "EUR"
  And content "reference" must not be empty
  And this step is terminal
```

`Feature` is the contract identifier (letters, digits, underscores). `Version`
defaults to 1 and must be increased when updating. `Description` defaults to the
Feature name. Metadata precedes all blocks. Each `Rule` has a descriptive title,
one `When` naming its step, then a `Then` clause and zero or more `And` clauses.
Each step must have exactly one owner; parties are derived from those owners.
One Rule per step: all of its constraints are required together. A Rule title is
a label, not an executable condition.

Blank lines and whole-line `#` comments are allowed. Quoted values use double
quotes with escapes (`\"`, `\\`); identifiers cannot be empty or multiline.
Inline comments and arbitrary prose are rejected rather than ignored.

## Supported step clauses

Use `Then` for the first clause and `And` for subsequent clauses.

| Clause | Meaning |
| --- | --- |
| `owner must be "processor"` | Required Threadify party/role |
| `step "approval" must have succeeded` | Earlier successful prerequisite; intervening work is allowed |
| `step "approval" must succeed before each invocation` | Fresh approval after the previous invocation, including failed/abandoned invocations |
| `this step is an entry point` | May be the first successful step |
| `this step is terminal` | Successful validation completes the thread |
| `next step must be one of "settled", "failed"` | Strict immediate transitions from this step |
| `this step must finish within "30s"` | Existing per-step duration validation |
| `the next step must start within "5m"` | Existing transition timeout; requires a next-step clause |
| `this step may be retried at most 2 times` | Existing retry check; requires a next-step clause and a positive limit |
| `content "reference" must be present` | Key must exist; an empty value is permitted |
| `content "note" is optional` | Documents an optional field; cannot have a required content check |
| `content "reference" must not be empty` | Key must exist and contain non-whitespace text |
| `content "amount" must be a number` | Finite JSON-style numeric string |
| `content "amount" must be a number greater than 0` | Numeric lower bound |
| `content "amount" must be a number greater than or equal to 0` | Inclusive lower bound |
| `content "amount" must be a number less than 100` | Numeric upper bound |
| `content "amount" must be a number less than or equal to 100` | Inclusive upper bound |
| `content "confirmed" must be a boolean` | Exactly `"true"` or `"false"` |
| `content "currency" must be one of "GBP", "USD"` | Case-sensitive literal membership |
| `content "status" must equal "approved"` | Case-sensitive exact match |
| `content "tracking_number" must match regex "^TRK-[0-9]{8}$"` | Match a literal regex pattern against the submitted string |
| `content "tracking_number" must equal order_shipped.tracking_number` | Match the same thread's latest successful shipment |

## Human approval as a Contract step

Declare a reviewer-owned approval step and make the protected action depend on
it. `must have succeeded` accepts any earlier validated success. `must succeed
before each invocation` requires a fresh validated approval after the previous
invocation, even if that invocation failed or was abandoned. Ask the user which
reuse rule they intend; do not infer it from an observed Harnest approval prompt.
The host application presents the prompt and records the reviewer-approved
step. Threadify checks the step, its owner role, and an atomic `waitFor` claim
before the protected side effect. A denial does not satisfy the prerequisite.
`can` and `next` are read-only decisions used inside action tools; neither
records approval or opens a prompt.

Each content comparison requires the submitted field to exist in the step's
`context` map, whose values remain strings. Dotted field names are literal keys,
not nested paths. Unlisted fields remain allowed. The right side of an equality
can reference another step as described below. Expression execution,
semantic text checks, and content-based branch selection are not implemented.

Numbers reject whitespace, NaN and Infinity. Decimal comparisons are exact;
values must fit the finite float64 range, with at most 4096 characters and an
explicit exponent between -308 and 308. This bounds parser work while avoiding
rounding money comparisons.

Dependencies do not generate strict transitions. If any explicit next-step
clauses are present, the existing strict transition policy applies to the whole
contract: every permitted immediate edge must be declared. Entry/terminal steps
are derived when omitted, as in existing contracts.

## Regex content validation

```gherkin
Rule: Record shipment
  When step "order_shipped" is submitted
  Then owner must be "warehouse"
  And content "tracking_number" must match regex "^TRK-[0-9]{8}$"
  And content "amount" must match regex "^[0-9]+\\.[0-9]{2}$"
  And this step is terminal
```

Patterns use Go's RE2-style syntax. Matching is case-sensitive by default and
may match any part of a value; use `^` and `$` to require the entire value.
Inline flags such as `(?i)` (case-insensitive) and `(?s)` (dot matches newlines)
are supported. Lookaround and backreferences are not supported. Matching does
not use backtracking, including for nested repetitions such as `(a+)+`.

Double backslashes inside Gherkin quoted strings: `"^\\d+$"` supplies `^\d+$`
to the regex engine. `[0-9]` avoids that extra escaping. Patterns are literals,
not references to other steps. Missing fields always fail; an empty string
passes only if the pattern matches it. Multiple content rules must all pass.

Malformed patterns and patterns outside 1–4096 bytes are rejected during
preview, creation, and updates. Compiled patterns use a bounded in-memory cache;
regex checks add no database calls. The stored rule is `{field, operator:
"matches", value: "pattern"}`, so checks survive graph serialization and restart.
Regex rules use the same submission-time validation as other content checks.

## Cross-step references

An **unquoted** `step_name.field_name` on the right of `must equal` reads a
previous step's content:

```gherkin
Rule: Confirm delivery
  When step "delivery_confirmed" is submitted
  Then owner must be "carrier"
  And content "tracking_number" must equal order_shipped.tracking_number
  And this step is terminal
```

- The referenced step must be defined elsewhere in this contract. Step and field
  identifiers in references use letters, digits and underscores, beginning with
  a letter or underscore. Self-references, unknown steps and dependency cycles
  are rejected when the contract is validated.
- A reference implies a prerequisite, so a separate `step "order_shipped" must
  have succeeded` clause is optional. It does not create an immediate transition.
- Only the **same thread's latest validated successful occurrence** is read.
  Latest means Engine validation order, not a caller-supplied timestamp. Failed,
  pending, or violated attempts never replace a successful snapshot.
- A missing successful step, missing field in its newest successful context, or
  unavailable storage rejects the submission. There is no search for a field in
  an older successful occurrence.
- Multiple fields read from one step during a submission use one snapshot.
  Values are compared as exact original strings (`"1.00"` differs from `"1"`).
- Quoted `"order_shipped.tracking_number"` remains literal text. A dotted name
  inside `content "..."` remains a literal field key, not a nested JSON path.
- Success snapshots are kept with live validation state and archived durably to
  PostgreSQL. References are available after the source step passes validation;
  receiving its event-recorded acknowledgement alone does not establish success.

A reference checks already observed facts. It does not reserve a value against
concurrent future events or authorize an external side effect. Optional execution
waits (skill resource `references/wait-for.md`) can claim flow eligibility
before an invocation; checks on future input still require that input.

## Thread-wide rules

An optional `Background` before all Rules accepts:

```gherkin
Background:
  Given the thread must finish within "1h"
  And multiple terminal steps are allowed
```

Both clauses are optional. Existing duration units are `s`, `ms`, `us`, `m`, `h`,
and `d`. Parallel-group execution semantics are not exposed in this first format.

## Preview, publish, and update

The contract editor accepts Gherkin source and provides a starter example.
The existing REST endpoints accept raw `.feature` text with `Content-Type:
text/plain`. Use the existing authenticated contract-management session (the
same authentication used by the UI). The SDK's service API key remains the
credential for recording steps; it is not a Registry license key.

| Operation | Endpoint | Body |
| --- | --- | --- |
| Preview | `POST /v1/contracts/preview` | Raw `.feature` source |
| Create | `POST /v1/contracts` | Raw `.feature` source |
| Update | `PUT /v1/contracts/{id}` | Raw `.feature` source with an increased Version |
| Read version | `GET /v1/contracts/{id}/versions/{version}` | None |

Preview returns `valid`, diagnostics, and the compiled graph. To update, increase
`Version`, change the rules, and `PUT` the source to `/v1/contracts/{id}`.
Original source is preserved in version responses as `source`, with
`sourceFormat: "gherkin"`; the legacy `yamlContent` field remains an alias.
The stored executable content and graph are structured JSON. Content hashes use
compiled rules, so equivalent YAML/Gherkin input does not create a changed
contract merely by changing source format.

## Verification timing

Content checks run during submission alongside existing required-field and
owner checks; invalid content is rejected before the step event is recorded.
Transition, dependency, retry and timing checks retain the existing asynchronous
validation path. An accepted event can subsequently be marked `violated`.
GraphQL `can` checks action eligibility and optionally validates submitted
context; without candidate context it does not validate content rules or reserve
permission to perform external work.
Use the optional SDK wait APIs (skill resource `references/wait-for.md`) to
await flow permission before execution or await the exact validation result
after reporting an outcome.
Default event recording remains asynchronous; Threadify does not execute actions.
