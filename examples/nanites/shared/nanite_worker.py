"""Shared Threadify event adapter used by each worker Nanite."""

from __future__ import annotations

import json
import os
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any

import httpx

_MAX_PROMPT_CHARS = 32_000
_MAX_OUTPUT_CHARS = 16_000


@dataclass(frozen=True, slots=True)
class NaniteSpec:
    """Declare the single Threadify transition owned by one Nanite."""

    trigger_step: str
    output_step: str
    role: str

    def __post_init__(self) -> None:
        """Reject incomplete routing before the agent process starts."""

        _required_text(self.trigger_step, "trigger step")
        _required_text(self.output_step, "output step")
        _required_text(self.role, "Nanite role")


def register_nanite(spec: NaniteSpec, *, extension: Any | None = None) -> Any:
    """Bind one worker to Threadify without introducing another queue."""

    selected_extension = extension or _threadify_extension()

    @selected_extension.on("step.success", spec.trigger_step)
    async def consume(delivery: Any) -> None:
        """Invoke this Harnest agent, publish its result, then acknowledge."""

        notification = delivery.notification
        prompt = _event_prompt(spec, notification)
        output = await _invoke_self(
            prompt,
            thread_id=notification.thread_id,
            role=spec.role,
        )
        await delivery.record_step(
            spec.output_step,
            context={"output": _bounded(output, _MAX_OUTPUT_CHARS), "nanite": spec.role},
            message=f"{spec.role} Nanite completed",
            role=spec.role,
        )

    return consume


async def _invoke_self(prompt: str, *, thread_id: str, role: str) -> str:
    """Invoke the local Harnest HTTP contract with a retry-stable session."""

    endpoint = _required_text(os.getenv("NANITE_SELF_URL"), "NANITE_SELF_URL")
    headers = {"x-user": f"threadify:{role}"}
    session_id = (
        f"threadify-{_required_text(role, 'Nanite role')}-"
        f"{_required_text(thread_id, 'Threadify thread ID')}"
    )
    async with httpx.AsyncClient(
        base_url=endpoint,
        headers=headers,
        timeout=300,
    ) as client:
        created = await client.post("/sessions", json={"id": session_id})
        if created.status_code not in {201, 409}:
            created.raise_for_status()
        response = await client.post(
            "/responses",
            json={
                "input": prompt,
                "sessionId": session_id,
                "metadata": {"source": "threadify", "role": role},
            },
        )
        response.raise_for_status()
        return _response_text(response.json())


def _event_prompt(spec: NaniteSpec, notification: Any) -> str:
    """Provide only bounded Threadify business context to the model."""

    details = notification.to_dict()
    payload = {
        "threadId": notification.thread_id,
        "triggerStep": spec.trigger_step,
        "role": spec.role,
        "details": details.get("details", {}),
    }
    encoded = json.dumps(payload, sort_keys=True, default=str)
    return _bounded(encoded, _MAX_PROMPT_CHARS)


def _response_text(payload: Any) -> str:
    """Require the stable Harnest response field before advancing the workflow."""

    if not isinstance(payload, Mapping):
        raise TypeError("Harnest Nanite response must be an object")
    value = payload.get("outputText")
    return _required_text(value, "Harnest Nanite output")


def _threadify_extension() -> Any:
    """Import lazily so the shared module remains unit-testable outside activation."""

    from harnest.extensions.threadify import threadify

    return threadify


def _bounded(value: str, limit: int) -> str:
    """Bound model/provider context while making truncation explicit."""

    if len(value) <= limit:
        return value
    return value[: limit - 15] + "...[truncated]"


def _required_text(value: Any, label: str) -> str:
    """Normalize routing and response values before network use."""

    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} must be a non-empty string")
    return value.strip()


__all__ = ["NaniteSpec", "register_nanite"]
