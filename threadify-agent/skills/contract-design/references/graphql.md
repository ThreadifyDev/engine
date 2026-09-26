# Contract evidence workflow

Use the typed Threadify tools; never construct custom GraphQL.

1. If the user supplied a Threadify thread ID, call `get_thread` with that ID.
2. Otherwise call `search_threads` with `limit=1`, then pass the returned ID to
   `get_thread` exactly once.
3. Use the returned step timestamps, actor services, statuses, and bounded
   history as evidence for the contract.

The `get_thread` result includes up to 20 history entries per step. A history
`context` value may be a JSON string. Derive business-context keys only when that
string parses successfully, and only from successful observed history.

One execution cannot prove every possible branch. Preserve observed concurrency
and identify any inferred transitions or terminal steps as uncertain where the
evidence is incomplete.
For a proposed parallel `Group`, compare member start/end times and dependencies;
an order in the history list alone does not prove concurrent execution. Ask the
user when the workflow is intended to allow parallel work but the evidence is
inconclusive. Treat context keys seen in history as candidates: one occurrence
does not make a field required.
