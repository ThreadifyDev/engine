"""Tool which creates one observable Nanites workflow."""

from __future__ import annotations

import os

from harnest.context import context
from harnest.plugins.threadify import threadify
from harnest.tool import tool


@tool
async def submit_work(request: str) -> dict[str, str]:
    """Submit one request to the Nanites workflow and return its thread ID."""

    normalized = _required_request(request)
    invocation_id = context.current().invocation_id
    contract_name = os.getenv("THREADIFY_CONTRACT_NAME", "nanite_work_v1")
    existing = await threadify.connection.get_thread_by_ref(
        "harnestInvocation", invocation_id
    )
    if existing is not None:
        return {"thread_id": existing.thread_id, "status": "already_submitted"}
    thread = await threadify.start_thread(
        label=f"nanite-job-{invocation_id[-12:]}",
        contract_name=contract_name,
        refs={"harnestInvocation": invocation_id},
        role="dispatcher",
    )
    await threadify.record_step(
        thread.thread_id,
        "work_requested",
        context={"request": normalized},
        role="dispatcher",
        idempotency_key=invocation_id,
    )
    return {"thread_id": thread.thread_id, "status": "submitted"}


def _required_request(value: str) -> str:
    """Reject empty or unbounded work before creating remote state."""

    if not isinstance(value, str) or not value.strip():
        raise ValueError("request must be a non-empty string")
    normalized = value.strip()
    if len(normalized) > 16_000:
        raise ValueError("request exceeds 16000 characters")
    return normalized


__all__ = ["submit_work"]
