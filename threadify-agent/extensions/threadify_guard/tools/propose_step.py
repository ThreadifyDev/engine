from typing import Any

from harnest.lib.threadify_tooling import (
    execute_threadify_query,
    required_text,
    tool_error,
)
from harnest.agent import tool


_QUERY = """
query ProposeStep($threadId: ID!, $stepName: String!) {
  proposeStep(threadId: $threadId, stepName: $stepName) {
    threadId
    stepName
    allowed
    requiredSteps
    satisfiedSteps
    missingSteps
    previousStep
    reason
  }
}
"""


@tool
async def propose_step(thread_id: str, step_name: str) -> dict[str, Any]:
    """Ask Threadify whether a contract step is eligible in the real thread.

    Args:
        thread_id: Exact Threadify thread ID whose gathered execution facts apply.
        step_name: Exact authored contract step to evaluate.
    """

    normalized_thread = required_text(thread_id, "thread_id")
    normalized_step = required_text(step_name, "step_name")
    if normalized_thread is None or normalized_step is None:
        return tool_error("thread_id and step_name are required non-empty strings.")
    return await execute_threadify_query(
        _QUERY,
        {"threadId": normalized_thread, "stepName": normalized_step},
    )
