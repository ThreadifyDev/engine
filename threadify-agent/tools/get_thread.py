from typing import Any

from harnest.lib.threadify_tooling import (
    execute_threadify_query,
    required_text,
    tool_error,
)
from harnest.tool import tool


_QUERY = """
query GetThread($id: ID!) {
  thread(id: $id) {
    id
    contractId
    contractName
    contractVersion
    status
    startedAt
    completedAt
    error
    refs
    steps {
      stepName
      idempotencyKey
      status
      retryCount
      firstSeenAt
      lastUpdatedAt
      startedAt
      finishedAt
      actor
      actorService
      latestContext
      history(limit: 20) {
        attempt
        status
        timestamp
        context
        error
        duration
        actor
        actorService
      }
    }
    notifications {
      notificationId
      source
      notificationType
      stepStatus
      validationStatus
      violationType
      severity
      message
      timestamp
    }
  }
}
"""


@tool
async def get_thread(id: str) -> dict[str, Any]:
    """Deep-dive one Threadify thread, including steps, history, and violations.

    Args:
        id: Exact Threadify thread UUID returned by a Threadify search or event.
    """

    thread_id = required_text(id, "id")
    if thread_id is None:
        return tool_error("id must be a non-empty Threadify thread identifier.")
    return await execute_threadify_query(_QUERY, {"id": thread_id})
