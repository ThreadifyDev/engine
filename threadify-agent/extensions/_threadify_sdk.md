# Threadify Harnest Extension

`threadify/` is a Harnest `Extension` which owns one Threadify SDK
connection per agent process. It exposes the official SDK types from
`harnest.extensions.threadify`, adds audited helpers for common mutations, and
turns Threadify subscriptions into tracked async callbacks.

```python
from harnest.extensions.threadify import ThreadifyDelivery, threadify


@threadify.on("step.success", "work_requested")
async def pick_up(delivery: ThreadifyDelivery) -> None:
    await delivery.record_step(
        "work_completed",
        context={"worker": "research-nanite"},
        role="researcher",
    )
```

Within an agent tool, use `threadify.thread(thread_key, options=None)`,
`threadify.record_step(...)`, or the typed invocation view returned by
`context.extensions("threadify")`. Use `.connection` only when the managed helpers
do not cover an SDK operation.

`THREADIFY_API_KEY` is required. `THREADIFY_SERVICE_NAME` defaults to the root
agent name. Optional connection settings are `THREADIFY_WS_URL`,
`THREADIFY_GRAPHQL_URL`, `THREADIFY_MAX_IN_FLIGHT`,
`THREADIFY_CONNECT_TIMEOUT`, and `THREADIFY_DEBUG`.

Use an application session or workflow ID as `thread_key`. Pass creation defaults such as `{"label": "Support session", "contract": "support:1", "refs": {"customer": "C-123"}}` on the first call. Later invocations call `await threadify.thread(thread_key)` to load the stored contract. Closed threads reject writes; use a new key for a new process.
