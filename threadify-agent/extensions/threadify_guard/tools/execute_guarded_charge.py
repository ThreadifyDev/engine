"""Guarded no-side-effect final action for live enforcement proofs."""

from __future__ import annotations

from typing import Any

from harnest.extensions.threadify_guard import guarded_step
from harnest.tool import tool


@tool
@guarded_step("charge", thread_id_argument="thread_id")
async def execute_guarded_charge(thread_id: str) -> dict[str, Any]:
    """Execute a charge simulation only after Threadify allows it.

    Args:
        thread_id: Exact contract-bound Threadify thread used for authorization.
    """

    return {
        "executed": True,
        "threadId": thread_id,
        "stepName": "charge",
        "message": "The guarded charge action body executed.",
    }
