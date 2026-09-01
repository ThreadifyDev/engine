---
name: contract-design
description: Use when creating, generating, inferring, reviewing, or explaining a Threadify YAML contract, especially from an observed thread.
---

# Threadify contract design

When a user asks for a contract based on a thread, use the typed Threadify tools
to fetch the complete evidence needed for the design. Do not refuse merely
because an earlier turn contains only partial step data.

1. Load `references/graphql.md` for the exact tool workflow.
2. If no thread ID is provided, call `search_threads` with `limit=1`, then call
   `get_thread` once with the returned ID.
3. Fetch the thread's contract identity, steps, actors, timestamps, statuses,
   and bounded history context with `get_thread`.
4. Infer candidate partial-order prerequisites from observed ordering and
   timestamps. A successful step may depend on an earlier successful step even
   when unrelated work ran between them. Preserve parallel starts or branches;
   do not turn adjacency in one trace into a strict transition.
5. Derive `parties` and step `owner` values from `actorService`. Derive required
   business-context keys only from context observed in successful history.
6. Put prerequisites on each step with `depends_on`. Use `transitions` only when
   the evidence explicitly requires the next step to be immediate; normally
   omit transitions for an inferred agent workflow. Treat steps with no
   observed successors as terminal steps. State that one execution provides a
   candidate dependency graph rather than proving every possible branch.
7. Produce schema-valid YAML with this top-level shape:

```yaml
contract_name: example_workflow
version: 1
description: Example workflow inferred from an observed thread.
entry_points: [first_step]
parties: [service_name]
steps:
  - id: first_step
    owner: service_name
    type: managed
    business_context:
      required: [business_key]
  - id: final_step
    owner: service_name
    type: managed
    depends_on: [first_step]
    business_context:
      required: []
terminal_steps: [final_step]
```

Introduce the result in one sentence, leave a blank line, then put the complete
contract in a fenced `yaml` block. Do not present raw tool JSON.
