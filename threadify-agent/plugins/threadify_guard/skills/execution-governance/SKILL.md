---
name: execution-governance
description: Use before attempting a contract-sensitive action, when asked what can run next, or when a Threadify-guarded tool reports missing prerequisites.
---

# Threadify execution governance

Use Threadify's recorded execution facts as the source of truth for ordering.

1. Obtain an exact Threadify thread ID and exact authored step name. Never use a
   Harnest session ID as a substitute unless the caller confirms they are the
   same identifier.
2. Call `propose_step` before planning a contract-sensitive action. Explain its
   `satisfiedSteps`, `missingSteps`, `previousStep`, and `reason` in plain terms.
3. Treat `allowed: true` as eligibility, not proof that the business action has
   already occurred.
4. A result with `THREADIFY_EXECUTION_BLOCKED` means the action tool body did
   not run. Use the returned missing steps to revise the plan; do not retry the
   same call without a new execution fact.
5. `verify_step_execution` is a no-side-effect boundary probe. Use it only when
   the user asks to verify enforcement or eligibility behavior.
6. Intervening successful steps do not invalidate partial-order prerequisites.
   If a contract has an explicit strict transition, the immediately preceding
   successful step can still matter.
