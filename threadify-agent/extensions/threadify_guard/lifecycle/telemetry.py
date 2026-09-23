"""Export the agent's executed-tool exhaust to Threadify over OTLP/HTTP."""

from __future__ import annotations

import asyncio
from collections.abc import Sequence
from contextvars import ContextVar
from dataclasses import dataclass
import hashlib
import os
import secrets
import threading
import time
from typing import Any
from urllib.parse import urlsplit, urlunsplit

from harnest.lib.threadify_tooling import execute_threadify_query
from harnest import lifecycle
from harnest.extensions.threadify_guard import extension
from harnest.telemetry import TelemetryExporter
from opentelemetry import trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import ReadableSpan
from opentelemetry.sdk.trace.export import SpanExporter, SpanExportResult
from opentelemetry.trace import SpanContext, Status, StatusCode, TraceFlags


_OPERATION_NAME = "gen_ai.operation.name"
_TOOL_NAME = "gen_ai.tool.name"
_AGENT_NAME = "gen_ai.agent.name"
_CONVERSATION_ID = "gen_ai.conversation.id"
_ACK_ATTEMPTS = 100
_ACK_POLL_SECONDS = 0.1
_STEP_ACK_QUERY = """
query ToolStepAck($threadId: ID!) {
  thread(id: $threadId) {
    steps {
      stepName
      status
    }
  }
}
"""


@dataclass(frozen=True, slots=True)
class ToolCompletion:
    """Request-local facts needed to acknowledge one process action."""

    tool_name: str
    step_name: str
    thread_id: str
    agent_name: str
    session_id: str
    invocation_id: str
    started_at: int


_ACTIVE_EXPORTER: "ThreadifyToolSpanExporter | None" = None
_OBSERVED_CALLS: ContextVar[tuple[ToolCompletion, ...]] = ContextVar(
    "threadify_observed_tool_calls",
    default=(),
)


class ThreadifyToolSpanExporter(SpanExporter):
    """Forward only completed tool executions, normalized as Threadify steps."""

    def __init__(self, delegate: SpanExporter) -> None:
        self._delegate = delegate
        self._lock = threading.Lock()

    def export(self, spans: Sequence[ReadableSpan]) -> SpanExportResult:
        """Send one coherent tool batch while dropping model and transport spans."""

        selected = tuple(
            prepared
            for span in spans
            if (prepared := _threadify_span(span)) is not None
        )
        if not selected:
            return SpanExportResult.SUCCESS
        return self._send(selected)

    def export_completion(self, completion: ToolCompletion, *, failed: bool) -> None:
        """Synchronously send one agent-completed action before its result returns."""

        result = self._send((_completion_span(completion, failed=failed),))
        if result is not SpanExportResult.SUCCESS:
            raise RuntimeError(
                f"Threadify did not acknowledge step {completion.step_name!r}"
            )

    def _send(self, spans: Sequence[ReadableSpan]) -> SpanExportResult:
        """Serialize access to the delegate shared with Harnest's batch worker."""

        with self._lock:
            return self._delegate.export(spans)

    def shutdown(self) -> None:
        """Release the underlying OTLP HTTP client."""

        with self._lock:
            self._delegate.shutdown()

    def force_flush(self, timeout_millis: int = 30000) -> bool:
        """Flush the underlying exporter when it supports an explicit flush."""

        return self._delegate.force_flush(timeout_millis)


@lifecycle.tool.before(order=-1000)
def attach_threadify_trace_context(context, request):
    """Stamp request-local identity onto the active ADK tool execution span."""

    span = trace.get_current_span()
    observation = extension.observation_for(request.name)
    if observation is not None:
        try:
            thread_id, step_name = observation.resolve(request.args, request.kwargs)
        except ValueError:
            pass
        else:
            _OBSERVED_CALLS.set(
                (
                    *_OBSERVED_CALLS.get(),
                    ToolCompletion(
                        tool_name=request.name,
                        step_name=step_name,
                        thread_id=thread_id,
                        agent_name=_text(context.agent_name) or "harnest-agent",
                        session_id=_text(context.session_id) or "",
                        invocation_id=_text(context.invocation_id) or "",
                        started_at=time.time_ns(),
                    ),
                )
            )
    if span.is_recording():
        agent_name = _text(context.agent_name) or "harnest-agent"
        session_id = _text(context.session_id)
        span.set_attribute("threadify.step_name", request.name)
        span.set_attribute("threadify.tags", ("harnest", "agent-exhaust"))
        span.set_attribute("threadify.ref.harnest_agent", agent_name)
        span.set_attribute(
            "threadify.label",
            f"{agent_name}:{session_id}" if session_id else agent_name,
        )
        if session_id:
            span.set_attribute("threadify.ref.harnest_session_id", session_id)
    return context.next()


@lifecycle.tool.after(order=1000)
async def acknowledge_threadify_tool_completion(context, result):
    """Persist and verify an agent-executed process action before continuing."""

    completion = _pop_observed(context.tool_name)
    if completion is None or _blocked_result(result):
        return context.next()
    exporter = _ACTIVE_EXPORTER
    if exporter is None:
        raise RuntimeError("Threadify execution exhaust is not configured")
    await asyncio.to_thread(exporter.export_completion, completion, failed=False)
    await _confirm_step(completion, expected_status="success")
    if not isinstance(result, dict):
        return context.next()
    enriched = dict(result)
    enriched["executionExhaust"] = {
        "source": "threadify",
        "acknowledged": True,
        "threadId": completion.thread_id,
        "stepName": completion.step_name,
    }
    return context.next(enriched)


@lifecycle.tool.on_error(order=1000)
async def acknowledge_failed_threadify_tool(context, _error):
    """Record a failed process action without replacing its original exception."""

    completion = _pop_observed(context.tool_name)
    exporter = _ACTIVE_EXPORTER
    if completion is None or exporter is None:
        return
    await asyncio.to_thread(exporter.export_completion, completion, failed=True)


@lifecycle.telemetry_exporter
def threadify_execution_exhaust() -> TelemetryExporter:
    """Attach Threadify as a runtime-owned destination for agent tool exhaust."""

    api_key = _required_environment("THREADIFY_OTLP_API_KEY", "THREADIFY_API_KEY")
    endpoint = os.getenv("THREADIFY_OTLP_ENDPOINT", "").strip() or _default_endpoint()
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import (
        OTLPSpanExporter,
    )

    delegate = OTLPSpanExporter(
        endpoint=endpoint,
        headers={"X-API-Key": api_key},
    )
    exporter = ThreadifyToolSpanExporter(delegate)
    global _ACTIVE_EXPORTER
    _ACTIVE_EXPORTER = exporter
    return TelemetryExporter(
        name="threadify-agent-exhaust",
        traces=exporter,
    )


def _threadify_span(span: ReadableSpan) -> ReadableSpan | None:
    """Select real tool spans and add stable Threadify correlation directives."""

    attributes = dict(span.attributes or {})
    if attributes.get(_OPERATION_NAME) != "execute_tool":
        return None
    tool_name = _text(attributes.get(_TOOL_NAME))
    if tool_name is None:
        return None
    if extension.observation_for(tool_name) is not None:
        return None
    conversation_id = _text(attributes.get(_CONVERSATION_ID)) or _text(
        attributes.get("threadify.ref.harnest_session_id")
    )
    agent_name = (
        _text(attributes.get(_AGENT_NAME))
        or _text(attributes.get("threadify.ref.harnest_agent"))
        or "harnest-agent"
    )
    attributes.setdefault("threadify.step_name", tool_name)
    attributes.setdefault("threadify.tags", ("harnest", "agent-exhaust"))
    attributes.setdefault(
        "threadify.label",
        f"{agent_name}:{conversation_id}" if conversation_id else agent_name,
    )
    attributes.setdefault("threadify.ref.harnest_agent", agent_name)
    if conversation_id:
        attributes.setdefault("threadify.ref.harnest_session_id", conversation_id)

    return ReadableSpan(
        name=span.name,
        context=span.context,
        parent=span.parent,
        resource=span.resource,
        attributes=attributes,
        events=span.events,
        links=span.links,
        kind=span.kind,
        status=span.status,
        start_time=span.start_time,
        end_time=span.end_time,
        instrumentation_scope=span.instrumentation_scope,
    )


def _completion_span(completion: ToolCompletion, *, failed: bool) -> ReadableSpan:
    """Build a dedicated process span isolated from generic agent-trace correlation."""

    trace_bytes = hashlib.sha256(
        f"threadify-action:{completion.thread_id}:{completion.session_id}".encode()
    ).digest()[:16]
    trace_id = int.from_bytes(trace_bytes, "big") or 1
    span_id = secrets.randbits(64) or 1
    attributes = {
        _OPERATION_NAME: "execute_tool",
        _TOOL_NAME: completion.tool_name,
        _AGENT_NAME: completion.agent_name,
        "threadify.thread_id": completion.thread_id,
        "threadify.step_name": completion.step_name,
        "threadify.tags": ("harnest", "agent-exhaust", "acknowledged-action"),
        "threadify.label": f"{completion.agent_name}:{completion.session_id}",
        "threadify.ref.harnest_agent": completion.agent_name,
        "threadify.ref.harnest_session_id": completion.session_id,
        "threadify.context.harnest_invocation_id": completion.invocation_id,
        "threadify.context.acknowledged_completion": True,
    }
    return ReadableSpan(
        name=f"execute_tool {completion.tool_name}",
        context=SpanContext(
            trace_id=trace_id,
            span_id=span_id,
            is_remote=False,
            trace_flags=TraceFlags.SAMPLED,
        ),
        resource=Resource.create({"service.name": completion.agent_name}),
        attributes=attributes,
        status=Status(
            StatusCode.ERROR if failed else StatusCode.OK,
            "tool execution failed" if failed else None,
        ),
        start_time=completion.started_at,
        end_time=time.time_ns(),
    )


async def _confirm_step(completion: ToolCompletion, *, expected_status: str) -> None:
    """Confirm Threadify accepted the exact step instead of OTLP partial success."""

    for attempt in range(_ACK_ATTEMPTS):
        response = await execute_threadify_query(
            _STEP_ACK_QUERY,
            {"threadId": completion.thread_id},
        )
        data = response.get("data") if isinstance(response, dict) else None
        thread = data.get("thread") if isinstance(data, dict) else None
        steps = thread.get("steps") if isinstance(thread, dict) else None
        if isinstance(steps, list):
            for step in steps:
                if not isinstance(step, dict):
                    continue
                if (
                    step.get("stepName") == completion.step_name
                    and _text(step.get("status")) == expected_status
                ):
                    return
        if attempt < _ACK_ATTEMPTS - 1:
            await asyncio.sleep(_ACK_POLL_SECONDS)
    raise RuntimeError(
        "Threadify did not persist acknowledged step "
        f"{completion.step_name!r} on thread {completion.thread_id!r}"
    )


def _blocked_result(result: Any) -> bool:
    """Recognize the guard's fail-closed result so denial never becomes a step."""

    if not isinstance(result, dict):
        return False
    execution = result.get("executionContext")
    return isinstance(execution, dict) and execution.get("decision") == "blocked"


def _pop_observed(tool_name: str) -> ToolCompletion | None:
    """Pop the matching action while leaving nested unrelated calls untouched."""

    values = _OBSERVED_CALLS.get()
    if not values or values[-1].tool_name != tool_name:
        return None
    _OBSERVED_CALLS.set(values[:-1])
    return values[-1]


def _default_endpoint() -> str:
    """Derive the sibling OTLP endpoint from the configured GraphQL URL."""

    graphql_url = os.getenv(
        "THREADIFY_GRAPHQL_URL", "http://127.0.0.1:8081/graphql"
    ).strip()
    parsed = urlsplit(graphql_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("THREADIFY_GRAPHQL_URL must be an absolute HTTP(S) URL")
    return urlunsplit((parsed.scheme, parsed.netloc, "/v1/traces", "", ""))


def _required_environment(*names: str) -> str:
    """Resolve the service credential used only by the telemetry sink."""

    for name in names:
        value = os.getenv(name, "").strip()
        if value:
            return value
    raise RuntimeError(
        "Threadify exhaust requires THREADIFY_OTLP_API_KEY or THREADIFY_API_KEY"
    )


def _text(value: Any) -> str | None:
    """Normalize an OpenTelemetry string attribute."""

    if not isinstance(value, str) or not value.strip():
        return None
    return value.strip()


__all__ = [
    "ThreadifyToolSpanExporter",
    "acknowledge_threadify_tool_completion",
    "attach_threadify_trace_context",
    "threadify_execution_exhaust",
]
