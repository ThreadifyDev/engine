"""Agent-executed independent work for partial-order enforcement proofs."""

from __future__ import annotations

from typing import Any

from harnest.plugins.threadify_guard import observed_step
from harnest.tool import tool


@tool
@observed_step("unrelated", thread_id_argument="thread_id")
async def unrelated(thread_id: str) -> dict[str, Any]:
    """Run independent work between a prerequisite and guarded final action.

    Args:
        thread_id: Exact contract-bound Threadify thread receiving this fact.
    """

    return {
        "executed": True,
        "threadId": thread_id,
        "stepName": "unrelated",
        "message": "The agent completed unrelated intervening work.",
    }
