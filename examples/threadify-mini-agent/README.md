# Customer support agent with Threadify telemetry

This Harnest 0.18 agent serves the fictional Northstar Shop. Four business tools read customers/orders and create/list tickets in a persistent SQLite database. Threadify is an OTLP telemetry destination, not a model tool. A separate [analyst agent](../threadify-analyst-agent/README.md) reads the recorded activity through GraphQL.

```text
Browser -> support agent -> customer/order/ticket tools -> SQLite
                 |
           OTLP trace spans
                 v
        Threadify :8083 -> PostgreSQL
                 ^
              GraphQL
                 |
Browser -> analyst agent
```

From the repository root, start both playgrounds:

```sh
python3 examples/start_support_demo.py
```

These examples require Harnest 0.18's `OllamaModel` API. If your global CLI has been upgraded, set `HARNEST_PYTHON` to a Python executable in an existing Harnest 0.18 environment before running the launcher or tests.

- Support chat: http://127.0.0.1:8120/
- Analyst chat: http://127.0.0.1:8121/

The launchers load separate keys from `.threadify-local/support-credentials.json` and `.threadify-local/analyst-credentials.json`, falling back to `credentials.json` when absent. `THREADIFY_CREDENTIALS_FILE` overrides either path. Each file contains `engine_url` and `api_key`. On a Registry-licensed engine, create both service accounts under the engine's bound company: `standard_service` for support and `reader` for the analyst. The Registry license stays in the engine configuration; agents use engine API keys. Keys are never printed.

Model configuration comes from the existing `OLLAMA_BASE_URL`, `OLLAMA_API_KEY`, and `OLLAMA_MODEL` or `OLLAMA_ANSWER_MODEL` environment. With the current Ollama cloud configuration, sending a chat invokes that cloud model. Starting the servers sends no model prompt.

Try support: **“I'm Alex, alex@example.test. Check order ORD-1001 and create a support ticket if it is delayed.”**

Then ask the analyst: **“Inspect the latest support run. Which database operations happened, what was the order status, and was a ticket created?”**

Wait a few seconds for batched OTLP export and Threadify persistence. Each trace is a Threadify run; multiple chat turns can create multiple runs. Agent, model, tool, and database spans are exported. Database spans include curated record IDs, item/status fields, operation, and row count. Full chat-message capture is disabled. Playground health/asset requests are excluded. Telemetry does not enforce a workflow contract or block support operations.

Business data persists in `.threadify-local/support.sqlite3`; Harnest chat history uses an in-memory store and resets when its server restarts. These are local demonstration agents, with no real customer data, email sending, refunds, or production authentication. Logs and process-group leader PIDs are `.threadify-local/{support,analyst}-server.{log,pid}`. Keep these local servers on loopback.

To run only support in the foreground:

```sh
python3 examples/threadify-mini-agent/run_local.py
```

Tests:

```sh
python3 examples/threadify-mini-agent/run_local.py test
SUPPORT_VERIFICATION_FILE="$PWD/.threadify-local/support-verification.json" \
  python3 examples/threadify-mini-agent/run_local.py test --smoke
SUPPORT_VERIFICATION_FILE="$PWD/.threadify-local/support-verification.json" \
  python3 examples/threadify-analyst-agent/run_local.py test --smoke
```

The support smoke starts the real Harnest application, invokes actual business functions under a trace, creates a synthetic ticket, and checks OTLP conversion/readback in Threadify. The analyst smoke reads that trace through its tools. Neither smoke invokes an LLM; model tool selection and conversational quality are exercised by chatting in the playground. A separate direct PostgreSQL check verified persistence of the support smoke's seven spans.

OTEL runs close automatically when their finished root span arrives. This exporter also marks the finished customer-support invocation with `threadify.run.complete=true`, so completion works when the trace's HTTP parent is omitted. Engine-created, contract-free threads accept late spans from the same trace after closure without reopening. Contract-linked and explicitly targeted workflow threads keep their original lifecycle. Completion means execution ended, including failures; it does not mean a ticket was resolved or every distributed span has arrived.
