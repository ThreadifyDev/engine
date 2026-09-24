---
name: execution-governance
description: Use before attempting a contract-sensitive action, when asked what can run next, or when a Threadify-guarded tool reports missing prerequisites.
---

# Threadify execution governance

Use Threadify's recorded execution facts as the source of truth for ordering.

1. Obtain an exact Threadify thread ID. Never use a
   Harnest session ID as a substitute unless the caller confirms they are the
   same identifier.
2. Call the normal action tool for the requested operation. Its implementation
   checks `can` and may use `next` internally to return available paths,
   `satisfiedSteps`, `missingSteps`, `previousStep`, and `reason`. The model
   does not call decision APIs as separate tools. A path marked
   `requires_claim` needs `waitFor` to verify a fresh prerequisite.
3. If the tool reports a missing approval step, explain that the Contract
   requires a validated reviewer-owned step. Do not treat a Harnest approval
   prompt as that step by itself. Only a host integration may present the
   prompt and record the approved step under an authorized reviewer; a denial
   leaves the prerequisite unsatisfied. Do not invent an approval result.
4. Treat `allowed: true` as a read-only eligibility snapshot. A fresh
   prerequisite may instead yield `requires_claim`. The action tool must use
   an atomic `waitFor` claim before an external side effect. Optional `should`
   advice cannot grant execution.
5. A result with `THREADIFY_EXECUTION_BLOCKED` means the action tool body did
   not run. Use the returned missing steps to revise the plan; do not retry the
   same call without a new execution fact.
6. `verify_step_execution` is a no-side-effect boundary probe. Use it only when
   the user asks to verify enforcement or eligibility behavior.
7. Intervening successful steps do not invalidate partial-order prerequisites.
   If a contract has an explicit strict transition, the immediately preceding
   successful step can still matter.

The local `delete_test_record` probe uses a mock Engine and a hardcoded Harnest
approval gate. Do not present it as proof of Contract-driven approval or an
atomic `waitFor` claim.
