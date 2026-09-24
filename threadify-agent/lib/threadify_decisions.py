"""Threadify decision queries for use inside ordinary agent action tools."""

from __future__ import annotations

from typing import Any

from harnest.lib.threadify_tooling import execute_threadify_query, required_text, tool_error


_CAN_QUERY = """
query Can($threadId: ID!, $action: String, $goal: String, $context: JSON) {
  can(threadId: $threadId, action: $action, goal: $goal, context: $context) {
    threadId stepName allowed status matchedBy requiredSteps
    satisfiedSteps missingSteps previousStep reason
  }
}
"""

_NEXT_QUERY = """
query Next($threadId: ID!) {
  next(threadId: $threadId) {
    threadId paths { actions status reason }
  }
}
"""

_SHOULD_QUERY = """
query Should($threadId: ID!, $action: String, $goal: String) {
  should(threadId: $threadId, action: $action, goal: $goal) {
    threadId stepName eligible recommendation reason
  }
}
"""


async def can(
    thread_id: str,
    action: str = "",
    goal: str = "",
    context_data: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Read eligibility for one contract action or goal inside a normal tool."""
    thread = required_text(thread_id, "thread_id")
    if thread is None or not isinstance(action, str) or not isinstance(goal, str):
        return tool_error("thread_id, action, and goal must be strings.")
    if bool(action.strip()) == bool(goal.strip()):
        return tool_error("provide exactly one of action or goal.")
    if action and required_text(action, "action") is None:
        return tool_error("action must be a short non-empty string.")
    if len(goal) > 4096:
        return tool_error("goal must be at most 4096 characters.")
    if context_data is not None and not isinstance(context_data, dict):
        return tool_error("context_data must be a JSON object.")
    return await execute_threadify_query(
        _CAN_QUERY,
        {"threadId": thread, "action": action or None, "goal": goal or None, "context": context_data},
    )


async def next(thread_id: str) -> dict[str, Any]:
    """Read possible paths for a thread inside a normal tool."""
    thread = required_text(thread_id, "thread_id")
    if thread is None:
        return tool_error("thread_id is required.")
    return await execute_threadify_query(_NEXT_QUERY, {"threadId": thread})


async def should(thread_id: str, action: str = "", goal: str = "") -> dict[str, Any]:
    """Read optional advice without using it as execution authority."""
    thread = required_text(thread_id, "thread_id")
    if thread is None or not isinstance(action, str) or not isinstance(goal, str):
        return tool_error("thread_id, action, and goal must be strings.")
    if bool(action.strip()) == bool(goal.strip()):
        return tool_error("provide exactly one of action or goal.")
    if len(goal) > 4096:
        return tool_error("goal must be at most 4096 characters.")
    return await execute_threadify_query(
        _SHOULD_QUERY,
        {"threadId": thread, "action": action or None, "goal": goal or None},
    )
