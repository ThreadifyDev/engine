# Nanites with Harnest and Threadify

This example targets Harnest 0.12.1 and runs three independent agents:

| Agent | Responsibility | Threadify activity |
| --- | --- | --- |
| `dispatcher` | Accept a request and create the work thread | Records `work_requested` |
| `analyst` | Extract facts, risks, and missing information | Subscribes to `work_requested`; records `analysis_completed` |
| `reviewer` | Turn the analysis into a concise final plan | Subscribes to `analysis_completed`; records terminal `review_completed` |

Threadify notifications are the work-distribution path. There is no A2A layer
and no additional queue. Each worker acknowledges its notification only after
its Harnest invocation and next Threadify step succeed.

## Requirements

- Harnest 0.12.1 installed on the host
- Python 3 for the source materialization step; Harnest manages Python 3.12 for each agent
- A running Threadify Engine with three service-account keys
- PostgreSQL for durable Harnest sessions and checkpoints
- An OpenAI-compatible model endpoint and credential

## Prepare

Materialize the single Threadify Harnest Extension and shared worker adapter
into each independently compilable agent:

```bash
python3 prepare.py
python3 prepare.py --check

harnest env sync dispatcher
harnest env sync analyst
harnest env sync reviewer
```

The generated agents use the current `extensions/threadify/` and `lifecycle/`
layout. Each checked-in `harnest.lock` pins Google ADK 2.8.0, the framework
version resolved by Harnest 0.12.1.

Create `contract.feature` in the Threadify Engine. Give every Nanite its own
Threadify service-account key. The dispatcher owns the workflow event stream;
workers use their own key for joins and step mutations, plus the dispatcher's
key for subscriptions:

```bash
# Run from examples/nanites; use the workspace SDK containing thread(key, options).
export PYTHONPATH="$PWD/../../threadify-sdk-python${PYTHONPATH:+:$PYTHONPATH}"
export THREADIFY_WS_URL=ws://127.0.0.1:8081/threads
export THREADIFY_GRAPHQL_URL=http://127.0.0.1:8081/graphql
export HARNEST_NANITES_POSTGRES_URL='<postgres-dsn>'
export LITELLM_MODEL=openai/gpt-4.1-mini
export OPENAI_API_KEY=...

# Dispatcher process
export THREADIFY_API_KEY='<dispatcher-service-account-key>'

# Analyst process
export THREADIFY_API_KEY='<analyst-service-account-key>'
export THREADIFY_SUBSCRIPTION_API_KEY='<dispatcher-service-account-key>'

# Reviewer process
export THREADIFY_API_KEY='<reviewer-service-account-key>'
export THREADIFY_SUBSCRIPTION_API_KEY='<dispatcher-service-account-key>'
```

Keep the workspace SDK on `PYTHONPATH` for compilation, tests, and all runtime processes until the keyed API is published and the extension dependency pins are updated. The pinned SDK 0.2.10 does not expose `thread()`. The dispatcher uses the invocation ID as its durable thread key; later turns can resolve that key without repeating its contract.

`THREADIFY_API_KEY` is always the identity used for actions. Worker startup
requires `THREADIFY_SUBSCRIPTION_API_KEY` to subscribe as the event owner; it
is not an action-key fallback.

Set the PostgreSQL DSN before both `harnest compile` and runtime startup.
Compilation validates lifecycle storage factories but does not connect to the
database.

## Test and run

With the environment above set, run every offline agent contract:

```bash
harnest test dispatcher
harnest test analyst
harnest test reviewer
```

Start the workers in separate terminals:

```bash
NANITE_SELF_URL=http://127.0.0.1:8111 harnest serve analyst
NANITE_SELF_URL=http://127.0.0.1:8112 harnest serve reviewer
```

Submit work through the dispatcher:

```bash
harnest run dispatcher "Review the launch plan and identify the highest operational risk."
```

The agents are ordinary Harnest deployments and may scale independently. They
share PostgreSQL-backed session and checkpoint infrastructure, while session
ownership remains scoped to each Nanite's Harnest identity. A worker uses the
Threadify notification ID as its next-step idempotency key, so a redelivery
cannot create a second logical step.

## SDK escape hatch

The extension re-exports the official Python SDK from
`harnest.extensions.threadify`. Managed helpers emit Harnest mutation audit events;
use `threadify.connection` or `delivery.connection` when a provider-specific SDK
operation is required.
