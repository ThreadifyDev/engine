from typing import Any

from harnest.plugins.threadify_guard import guarded_step
from harnest.tool import tool


@tool
@guarded_step(
    step_name_argument="step_name",
    thread_id_argument="thread_id",
)
async def verify_step_execution(
    thread_id: str,
    step_name: str,
) -> dict[str, Any]:
    """Run a no-side-effect probe through the enforced Threadify tool boundary.

    The function body is reached only when Threadify authorizes the candidate
    step for the supplied thread. A denied result is produced by the plugin
    before this function executes.

    Args:
        thread_id: Exact Threadify thread ID whose gathered execution facts apply.
        step_name: Exact authored contract step to test.
    """

    return {
        "executed": True,
        "executionContext": {
            "source": "threadify",
            "decision": "allowed",
            "threadId": thread_id,
            "stepName": step_name,
        },
        "message": "The guarded tool body executed after Threadify authorization.",
    }
