# Threadify agent sidebar

## Product direction

Provide a persistent assistant on supported signed-in screens: contracts, threads,
entity profiles, and the trace ingestion settings tab. Users can describe a
workflow, create and revise Gherkin contracts, investigate threads and violations,
and author trace extraction rules while watching changes in the application.
The conversation survives route changes; its current page context remains visible.

The agent drafts and explains. Threadify's deterministic compiler and Engine remain
responsible for contract validation and enforcement. Observed execution history is
evidence for suggested rules, not proof that an observed sequence or field must
always be required. Unsupported language features must be surfaced explicitly.

## Phase 1 — frontend preview (completed; mock replies since replaced)

- Persistent agent provider above the route tree, a navigation entry, top-bar AI
  icon (replacing the top wallet control), and Cmd/Ctrl+J shortcut. Billing remains
  in the navigation.
- Docked panel below the top navigation at widths of 1024px and above, so both
  remain interactive. Below 1024px, a dismissible panel keeps the top navigation
  accessible and covers only the content area.
- Route context, optional context attachment, suggested tasks, conversation,
  new conversation, editable example artifacts, copy and undo.
- Clearly identified local sample replies for Gherkin, trace extraction, and
  workflow investigation. No model, tool execution, API calls, publishing, or
  claims of completed validation from the preview.
- In-memory conversation and drafts survive navigation and closing/reopening the
  panel. Clear them when leaving the signed-in route area; no browser persistence.
- Keyboard access, focus management, reduced-motion support, and mobile layout.

## Phase 2 — connect the assistant (initial implementation complete)

The sidebar now uses Harnest 0.20 with Ollama (Qwen 3.5 cloud by default),
authenticated Engine proxying, streamed
responses, and client-tool continuations. Implemented actions are
`get_page_context`, `navigate_ui`, `open_contract_draft`, and
`preview_contract_draft`. Draft updates and previews require the current revision.
Server tools read contracts, contract graphs, threads, violations, profile types,
and entity profiles. Contract saving remains in the existing UI.

See [agent setup and operating limits](../../threadify-agent/README.md).
Remaining follow-ups include restored conversation history, durable cancellation,
and resource-specific draft editors beyond new contract creation.

Original direction:

Reuse the existing agent service where appropriate. Update legacy YAML generation
to the supported Gherkin vocabulary and return structured artifacts rather than
scraping source from Markdown. Stream replies and action results into the sidebar.

Introduce a typed action registry with input/output schemas. Separate page actions
(navigate, select, edit a local draft) from authenticated data actions (read, preview,
save). The UI and the agent invoke shared handlers. Bind calls to the current user,
workspace, resource, and draft revision; enforce permissions on the server. Avoid
overwriting concurrent edits. Trace content is data, never instructions to the agent.

Suggested initial actions:

- `get_page_context`, `open_contract_editor`, `update_contract_draft`
- `read_thread`, `preview_contract`, `create_contract_version`
- `inspect_trace_sample`, `update_extraction_draft`, `preview_extraction_rule`

Return source, assumptions, unresolved questions, and unsupported requirements as
separate fields. Run contract previews through the existing compiler and use a
bounded repair loop. Present the exact draft and graph before publication, honoring
the user's requested action and existing authorization. Record actual action results
and provide cancellation, errors, and retry without duplicating mutations.

## Phase 3 — trace extraction and investigation

Show original sample spans beside proposed extracted fields, missing values, and
unmatched spans. Establish the extraction schema and backend capability before
connecting this UI. Existing ingestion filters and their preview are not a field
extraction API. The phase 1 example mapping is illustrative, not executable config.

Add investigation that links actual events and validation failures to their rules.
Expand inference to multiple threads, showing which suggestions are observed facts
and which are proposed policy.

## Phase 4 — WebMCP adapter

Expose the same action registry through WebMCP for browser agents. Keep the built-in
sidebar independent of WebMCP/browser availability. Feature-detect the adapter and
track browser support: WebMCP is a proposed standard, with Chrome origin-trial
documentation at https://developer.chrome.com/docs/ai/webmcp/ (checked 2026-09-22).
WebMCP exposes page capabilities; it does not supply the model or agent runtime.

## Verification

Phase 1: check build/types, open/close and shortcut, focus and dismissal, route
context, navigation persistence, new conversation, local draft edit/undo/copy,
desktop/mobile overflow, and absence of assistant network requests.

Connected phases: measure compilation success, unsupported-rule reporting, semantic
correctness against representative workflows, permissions, stale drafts, cancellation,
and preview accuracy. Start with: thread → Gherkin draft → visible edits → compiler
preview → contract creation.
