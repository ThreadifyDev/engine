"""Fail-closed Threadify policy at Harnest's managed tool boundary."""

from __future__ import annotations

from collections.abc import Mapping
from contextvars import ContextVar
from typing import Any

from harnest.lib.threadify_tooling import execute_threadify_query
from harnest import lifecycle
from harnest.extensions.threadify_guard import extension


_CAN_QUERY = """
query Can($threadId: ID!, $stepName: String!) {
  can(threadId: $threadId, action: $stepName) {
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
_ALLOWED_CONTEXTS: ContextVar[tuple[tuple[str, dict[str, Any]], ...]] = ContextVar(
    "threadify_guard_allowed_contexts",
    default=(),
)


@lifecycle.tool.before(order=-100)
async def enforce_threadify_execution(context, request):
    """Short-circuit a guarded call unless Threadify authorizes its exact step."""

    policy = extension.policy_for(request.name)
    if policy is None:
        return context.next()
    try:
        thread_id, step_name = policy.resolve(request.args, request.kwargs)
    except ValueError as error:
        return context.finish(_blocked(None, None, str(error)))

    result = await execute_threadify_query(
        _CAN_QUERY,
        {"threadId": thread_id, "stepName": step_name},
    )
    proposal, failure = _proposal(result)
    if failure is not None:
        return context.finish(_blocked(thread_id, step_name, failure))
    if proposal is not None and proposal["allowed"] is True:
        _push_allowed(
            request.name,
            _execution_context(thread_id, step_name, "allowed", proposal),
        )
        return context.next()
    reason = _text(proposal.get("reason")) if proposal is not None else None
    return context.finish(
        _blocked(
            thread_id,
            step_name,
            reason or "Threadify did not authorize this step.",
            proposal,
        )
    )


@lifecycle.tool.after(order=100)
def attach_threadify_execution(context, result):
    """Attach the proposal that authorized a guarded mapping-shaped tool result."""

    execution_context = _pop_allowed(context.tool_name)
    if execution_context is None or not isinstance(result, Mapping):
        return context.next()
    enriched = dict(result)
    enriched["executionContext"] = execution_context
    return context.next(enriched)


@lifecycle.tool.on_error(order=100)
def discard_threadify_execution(context, _error):
    """Discard an authorization decision when the guarded handler raises."""

    _pop_allowed(context.tool_name)


def _proposal(
    result: Any,
) -> tuple[dict[str, Any] | None, str | None]:
    """Validate only the decision fields trusted by the execution guard."""

    if not isinstance(result, Mapping):
        return None, "Threadify returned an invalid proposal response."
    errors = result.get("errors")
    if isinstance(errors, list) and errors:
        first = errors[0]
        message = first.get("message") if isinstance(first, Mapping) else None
        return None, _text(message) or "Threadify could not validate this step."
    data = result.get("data")
    value = data.get("can") if isinstance(data, Mapping) else None
    if not isinstance(value, Mapping) or not isinstance(value.get("allowed"), bool):
        return None, "Threadify returned an invalid proposal decision."
    return dict(value), None


def _blocked(
    thread_id: str | None,
    step_name: str | None,
    reason: str,
    proposal: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    """Return visible remediation context while proving the handler never ran."""

    execution_context = _execution_context(
        thread_id,
        step_name,
        "blocked",
        proposal,
        reason=reason,
    )
    return {
        "executed": False,
        "executionContext": execution_context,
        "errors": [
            {
                "code": "THREADIFY_EXECUTION_BLOCKED",
                "message": execution_context["reason"],
            }
        ],
    }


def _execution_context(
    thread_id: str | None,
    step_name: str | None,
    decision: str,
    proposal: Mapping[str, Any] | None,
    *,
    reason: str | None = None,
) -> dict[str, Any]:
    """Normalize the same evidence shape for denied and authorized calls."""

    proposal_reason = _text(proposal.get("reason")) if proposal is not None else None
    return {
        "source": "threadify",
        "decision": decision,
        "threadId": thread_id,
        "stepName": step_name,
        "reason": (
            _text(reason)
            or proposal_reason
            or (
                "Threadify authorized this tool call."
                if decision == "allowed"
                else "Threadify blocked this tool call."
            )
        ),
        "requiredSteps": _string_list(proposal, "requiredSteps"),
        "satisfiedSteps": _string_list(proposal, "satisfiedSteps"),
        "missingSteps": _string_list(proposal, "missingSteps"),
        "previousStep": (
            _text(proposal.get("previousStep")) if proposal is not None else None
        ),
    }


def _push_allowed(tool_name: str, value: dict[str, Any]) -> None:
    """Retain one task-local decision until the matching after-tool boundary."""

    _ALLOWED_CONTEXTS.set((*_ALLOWED_CONTEXTS.get(), (tool_name, value)))


def _pop_allowed(tool_name: str) -> dict[str, Any] | None:
    """Pop only the matching outer call so nested unguarded tools cannot steal it."""

    values = _ALLOWED_CONTEXTS.get()
    if not values or values[-1][0] != tool_name:
        return None
    _ALLOWED_CONTEXTS.set(values[:-1])
    return values[-1][1]


def _string_list(value: Mapping[str, Any] | None, key: str) -> list[str]:
    """Copy a bounded list of proposal identifiers into the visible decision."""

    items = value.get(key) if value is not None else None
    if not isinstance(items, list):
        return []
    return [item[:500] for item in items[:100] if isinstance(item, str)]


def _text(value: Any) -> str | None:
    """Bound external diagnostic text before returning it to the agent."""

    if not isinstance(value, str) or not value.strip():
        return None
    return " ".join(value.split())[:500]
