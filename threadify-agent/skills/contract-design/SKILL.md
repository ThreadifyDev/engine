---
name: contract-design
description: Create, revise, review, or explain a Threadify Gherkin contract using observed workflow evidence and the connected frontend editor.
---

# Threadify contract design

1. Read `references/gherkin.md` for the exact supported language. Do not emit
   arbitrary Gherkin, YAML, scenarios, expressions, or content-based branching.
   Load `references/wait-for.md` only when the user asks about execution waits
   or permission claims.
2. For an existing thread/contract, recover its identity with get_page_context
   or a typed search, then read it with get_thread/get_contract. Use
   get_contract_graph to explain executable dependencies and transitions.
   Do not silently choose an unrelated recent thread as the user's workflow.
3. Propose owners and prerequisites from the user's intent and observed evidence.
   An observed sequence does not prove a strict transition. A field seen once
   does not establish that it is mandatory. Ask focused questions about unclear
   owners, required steps, timing, or failure behavior before encoding those
   decisions in a draft. Surface important assumptions.
4. Prefer successful prerequisites when unrelated work can intervene. If any
   strict next-step clause is used, every permitted immediate edge must be listed.
   For human approval, ask who may approve and whether one validated approval
   can cover later invocations. Model the reviewer-owned approval as a Contract
   step, then choose `must have succeeded` or `must succeed before each
   invocation` accordingly. A Harnest prompt alone is not a Contract step;
   the host must record an approved outcome under the authorized reviewer.
5. Call get_page_context to read the current contract draft and revision, then
   open_contract_draft with the complete source and expected revision. Preserve
   user edits; on conflict, read again before proposing a new draft.
6. Call preview_contract_draft with the returned revision. Repair compiler errors
   at most twice, without silently dropping requirements. Unsupported requirements
   remain visible to the user. Only report validation success from the tool result.
7. Summarize the rules and assumptions. The draft is not published; the user saves
   it from the editor. If no frontend is connected, provide a fenced `gherkin`
   draft without claiming to have opened or validated it.
