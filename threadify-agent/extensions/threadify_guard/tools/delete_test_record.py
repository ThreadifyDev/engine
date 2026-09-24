"""Disposable deletion probe for the local execution-governance demo."""

from typing import Any

from harnest.agent import tool
from harnest.agent.approval import request_human_approval
from harnest.lib.threadify_decisions import can, next


_TEST_RECORDS = {"record-1"}


@tool
async def delete_test_record(thread_id: str, record_id: str) -> dict[str, Any]:
    """Check Threadify paths and eligibility, then request approval before deleting a test record.

    Args:
        thread_id: Exact Threadify thread used for the eligibility check.
        record_id: Disposable test record ID; only record-1 is supported.
    """
    if record_id != "record-1":
        return {"deleted": False, "reason": "Only the disposable record-1 exists."}

    paths_result = await next(thread_id)
    paths_data = paths_result.get("data")
    paths = paths_data.get("next") if isinstance(paths_data, dict) else None
    if not isinstance(paths, dict):
        return {"deleted": False, "reason": "Threadify could not list the available paths.",
                "decision": paths_result}

    can_result = await can(thread_id, action="delete_test_record")
    can_data = can_result.get("data")
    decision = can_data.get("can") if isinstance(can_data, dict) else None
    if not isinstance(decision, dict) or decision.get("stepName") != "delete_test_record":
        return {"deleted": False, "reason": "Threadify could not validate the delete action.",
                "paths": paths, "decision": can_result}
    if decision.get("allowed") is not True:
        return {"deleted": False, "reason": decision.get("reason") or "Threadify denied deletion.",
                "paths": paths, "decision": decision}

    async with request_human_approval(
        action="delete_test_record",
        message="Approve deletion of the disposable test record record-1?",
        arguments={"thread_id": thread_id, "record_id": record_id},
        timeout_seconds=900,
    ):
        if record_id not in _TEST_RECORDS:
            return {"deleted": False, "reason": "The test record was already deleted.",
                    "paths": paths, "decision": decision}
        _TEST_RECORDS.remove(record_id)

    return {"deleted": True, "recordId": record_id, "threadId": thread_id,
            "paths": paths, "decision": decision}
