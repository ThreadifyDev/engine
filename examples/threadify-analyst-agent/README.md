# Threadify analyst agent

A separate read-only Harnest 0.18 managed ADK agent for investigating the customer
support agent's telemetry. It has two authored tools: `list_support_runs` (the
latest 1–10 runs) and `inspect_run` (a UUID). Both issue fixed GraphQL reads against
Threadify. Only threads tagged with `harnest_agent=customer_support_agent` are
returned. There is no Threadify writer tool or telemetry exporter in this agent.

Set `THREADIFY_GRAPHQL_URL=http://127.0.0.1:8083/graphql` and
`THREADIFY_API_KEY` in the server's environment. The key stays in the credential
provider, with the `threadify-local` audience and `thread:read` scope; it is never
an argument or tool result. Use a server-side read-only service-account key to
enforce read access beyond the tool surface. Configure `OLLAMA_MODEL` and
`OLLAMA_BASE_URL` for an available Ollama model. Cloud model choices send the
chat and returned telemetry to that provider.

```sh
harnest test examples/threadify-analyst-agent
harnest compile examples/threadify-analyst-agent
harnest serve examples/threadify-analyst-agent --host 127.0.0.1 --port 8121
```

Open the server's playground and ask, “What did the support agent do in its latest
run?” or “Which recorded operations failed, and how long did they take?” The
agent cites thread IDs and step names. Results depend on telemetry arriving in
Threadify; the recent-run sample is not an exhaustive search, and successful
spans do not independently prove that a business action was correct. Local
session history uses Harnest's process-local MemoryStore and resets on restart.

The unit suite exercises the discovered agent/tools, input validation, fixed
queries and headers, support-only filtering, and sanitized failures without
calling a model or a running Threadify instance.

After the support agent's OTEL smoke test writes its verification report, run the
analyst's live read-only test against that same local instance:

```sh
SUPPORT_VERIFICATION_FILE="$PWD/.threadify-local/support-verification.json" \
  python3 examples/threadify-analyst-agent/run_local.py test --smoke
```

This calls the analyst's tools directly, confirms the support thread and ticket
operation can be retrieved, and checks timestamps and service identity. It does
not invoke a model. GraphQL JSON scalars and step context are normalized whether
Threadify returns objects or serialized JSON strings.
