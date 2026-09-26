# Contract decisions for agents

Threadify exposes three read-only GraphQL decisions over the current Thread and
its pinned Contract version. The Engine remains the execution authority;
`waitFor` is still the atomic permission claim before a side effect.

| Query | Purpose | Classifier use |
| --- | --- | --- |
| `can(threadId, action, goal, context)` | Explain whether one Contract action is eligible. Provide exactly one of `action` (authored ID) or `goal` (natural-language action request). Optional `context` validates content rules against prior successful step content. | Maps a goal to one authored action. An ambiguous or low-probability mapping returns `uncertain`. |
| `should(threadId, action, goal)` | Advise whether an eligible candidate fits the current process state. | Optional; returns `unavailable` when no classifier is configured. It never grants permission. |
| `next(threadId)` | Return multiple currently eligible starts and bounded conditional paths through the Contract. Each path has `status: allowed` or `requires_claim`. | Not required; all starts are derived deterministically. Later actions must be checked again when reached. |

`can` uses validated successful steps, strict transitions, partial-order
prerequisites, thread status, maximum duration, and caller role. It is a
read-only snapshot. The Engine's `waitFor` claim remains authoritative for
outstanding invocations and fresh prerequisites, and `recordEvent` enforces
content rules on submitted output. A `can` result must never be treated as a
durable grant.

## Contract-driven human approval

An approval requirement belongs in the Contract as a step owned by the
reviewer, with the protected action depending on that step. For a fresh approval
before every invocation:

```gherkin
Feature: record_deletion
Version: 1

Rule: Approve deletion
  When step "delete_approval" is submitted
  Then owner must be "reviewer"
  And this step is an entry point

Rule: Delete record
  When step "delete_record" is submitted
  Then owner must be "agent"
  And step "delete_approval" must succeed before each invocation
```

Before a validated approval, `can(threadId, action: "delete_record")` reports
the missing prerequisite. `next` can show `delete_approval` as an available
start and `delete_record` as a conditional later action; it does not rank paths
or trigger a prompt. The host application must present the request to an
authorized reviewer and record a successful `delete_approval` step only after
approval. A denial must not be recorded as a successful approval step.

After approval, `can` is still only a snapshot. For the fresh clause above it
reports `requires_claim`, not an execution grant. The action tool must obtain
`waitFor("delete_record")` before the side effect and report its outcome on the
same invocation. An earlier approval cannot be reused after a prior invocation
consumes it. Threadify enforces the Contract and claim; the host owns the human
interface and the mapping from an approval step to a reviewer prompt. Merely
naming a step `approval` does not create a Harnest approval gate.

For agents, expose the business action as a normal tool. Its implementation can
call `next` and `can` internally to return paths or a blocked reason. The model
does not need separate decision tools. The local Harnest
`delete_test_record` demo uses a mock Engine and a hardcoded Harnest prompt; it
does not demonstrate this Contract-driven approval sequence.

Contracts may declare `semantic_rules` on a step. Each rule names a candidate
`field`, a yes/no `question`, optional `context_steps`, and an optional
`min_probability` (default `0.8`). Context steps become prerequisites. The
Engine sends only the candidate field and the latest validated content for
those steps to a configured classifier using Jev's `noul` question. An
unavailable classifier, missing context, or result below the threshold fails
the rule. For example:

```yaml
steps:
  - id: issue_refund
    owner: agent
    semantic_rules:
      - field: refund_note
        question: Does this note describe a refund for the verified customer?
        context_steps: [identity_verified]
        min_probability: 0.8
```

When `business_context` is declared, the Engine uses only its `required` and
`optional` fields as contract facts. Optional fields can be omitted. Extra
submitted context is retained as step metadata, but does not affect validation
or validated content sent to the classifier. Steps without `business_context`
retain their existing context behavior.

When `can` has no candidate `context`, it evaluates flow eligibility only.
The final submitted context is checked again when the step is recorded.

For a self-hosted Jev-compatible classifier:

```yaml
classifier:
  base_url: https://api.typesafe.ai/v1
  model: jev-latest
  auth: custom
  api_key_env: JEV_API_KEY
  timeout_ms: 5000
```

The Engine calls `POST {base_url}/systemone` with Jev `state`, `model`, and a
typed `choice` question. Credentials stay in the Engine process. The model may
only select labels supplied from the Contract. A low-probability, unlisted, or
unavailable result never authorizes an action. With no classifier configured,
exact authored action IDs still work.

Hosted Threadify can provide the same protocol through its Web API. Set the
Engine to `auth: threadify_license`, `model: threadify-classifier`, and the
hosted API's `/v1` base URL. The Web API verifies the existing Registry license
on each request and forwards `/v1/systemone` to its deployment-configured
classifier. The hosted classifier must be configured by the deployment; merely
enabling `classifier` in the Engine does not provision one.

The Jev-compatible wire format follows the [TypeSafe API reference](https://docs.typesafe.ai/api).
