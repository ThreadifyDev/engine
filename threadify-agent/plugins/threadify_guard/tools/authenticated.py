"""Agent-executed authentication prerequisite for governed process actions."""

from __future__ import annotations

from typing import Any

from harnest.plugins.threadify_guard import observed_step
from harnest.tool import tool


@tool
@observed_step("authenticated", thread_id_argument="thread_id")
async def authenticated(thread_id: str) -> dict[str, Any]:
    """Complete the authentication prerequisite for a Threadify process.

    Args:
        thread_id: Exact contract-bound Threadify thread receiving this fact.
    """

    return {
        "executed": True,
        "threadId": thread_id,
        "stepName": "authenticated",
        "message": "The agent completed the authentication action.",
    }
