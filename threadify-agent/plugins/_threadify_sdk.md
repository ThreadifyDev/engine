# Threadify runtime plugin

`threadify/` is a Harnest `RuntimePlugin` which owns one Threadify SDK
connection per agent process. It exposes the official SDK types from
`harnest.plugins.threadify`, adds audited helpers for common mutations, and
turns Threadify subscriptions into tracked async callbacks.

```python
from harnest.plugins.threadify import ThreadifyDelivery, threadify


@threadify.on("step.success", "work_requested")
async def pick_up(delivery: ThreadifyDelivery) -> None:
    await delivery.record_step(
        "work_completed",
        context={"worker": "research-nanite"},
        role="researcher",
    )
```

Within an agent tool, use `threadify.start_thread(...)`,
`threadify.record_step(...)`, or the typed invocation view returned by
`context.plugins("threadify")`. Use `.connection` only when the managed helpers
do not cover an SDK operation.

`THREADIFY_API_KEY` is required. `THREADIFY_SERVICE_NAME` defaults to the root
agent name. Optional connection settings are `THREADIFY_WS_URL`,
`THREADIFY_GRAPHQL_URL`, `THREADIFY_MAX_IN_FLIGHT`,
`THREADIFY_CONNECT_TIMEOUT`, and `THREADIFY_DEBUG`.
