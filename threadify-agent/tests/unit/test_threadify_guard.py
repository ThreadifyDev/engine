import asyncio

from harnest.lifecycle import LifecycleListener
from harnest.extensions.threadify_guard import guarded_step
from harnest.extensions.threadify_guard.lifecycle.enforce import (
    attach_threadify_execution,
    enforce_threadify_execution,
)
from harnest.extensions.threadify_guard.lifecycle.telemetry import (
    ToolCompletion,
    _completion_span,
    _threadify_span,
    attach_threadify_trace_context,
)
from harnest.tool_lifecycle import (
    ToolCallRequest,
    ToolLifecycleContext,
    ToolLifecyclePipeline,
)
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import ReadableSpan
from opentelemetry.trace import SpanContext, TraceFlags


def _context():
    return ToolLifecycleContext(
        framework="adk",
        agent_name="threadify_agent",
        invocation_id="invocation-1",
        user_id="user-1",
        session_id="session-1",
        tool_name="test_guarded_charge",
    )


def _listener(phase, callback, order):
    return LifecycleListener(
        phase=phase,
        callback=callback,
        order=order,
        relative_path="extensions/threadify_guard/lifecycle/enforce.py",
        line=1,
        function_name=callback.__name__,
    )


def _pipeline():
    return ToolLifecyclePipeline(
        [
            _listener("before_tool", enforce_threadify_execution, -100),
            _listener("after_tool", attach_threadify_execution, 100),
        ]
    )


def test_guard_short_circuits_handler_with_real_remediation_shape(monkeypatch):
    calls = []

    @guarded_step("charge", thread_id_argument="thread_id")
    async def test_guarded_charge(thread_id: str):
        calls.append(thread_id)
        return {"executed": True}

    callback = enforce_threadify_execution

    async def execute(_query, variables):
        assert variables == {"threadId": "thread-1", "stepName": "charge"}
        return {
            "data": {
                "can": {
                    "threadId": "thread-1",
                    "stepName": "charge",
                    "allowed": False,
                    "requiredSteps": ["authenticated"],
                    "satisfiedSteps": [],
                    "missingSteps": ["authenticated"],
                    "previousStep": "unrelated",
                    "reason": "missing prerequisite steps: authenticated",
                }
            }
        }

    monkeypatch.setitem(callback.__globals__, "execute_threadify_query", execute)
    pipeline = _pipeline()
    result = asyncio.run(
        pipeline.run(
            ToolCallRequest(
                "test_guarded_charge",
                kwargs={"thread_id": "thread-1"},
            ),
            lambda request: test_guarded_charge(**dict(request.kwargs)),
            _context(),
        )
    )

    assert calls == []
    assert result["executed"] is False
    assert result["errors"][0]["code"] == "THREADIFY_EXECUTION_BLOCKED"
    assert result["executionContext"]["missingSteps"] == ["authenticated"]
    assert result["executionContext"]["previousStep"] == "unrelated"


def test_guard_allows_handler_after_prerequisite_even_with_intervening_step(
    monkeypatch,
):
    calls = []

    @guarded_step("charge", thread_id_argument="thread_id")
    async def test_guarded_charge(thread_id: str):
        calls.append(thread_id)
        return {"executed": True}

    callback = enforce_threadify_execution

    async def execute(_query, _variables):
        return {
            "data": {
                "can": {
                    "threadId": "thread-1",
                    "stepName": "charge",
                    "allowed": True,
                    "requiredSteps": ["authenticated"],
                    "satisfiedSteps": ["authenticated"],
                    "missingSteps": [],
                    "previousStep": "unrelated",
                    "reason": "all prerequisite steps are satisfied",
                }
            }
        }

    monkeypatch.setitem(callback.__globals__, "execute_threadify_query", execute)
    pipeline = _pipeline()
    result = asyncio.run(
        pipeline.run(
            ToolCallRequest(
                "test_guarded_charge",
                kwargs={"thread_id": "thread-1"},
            ),
            lambda request: test_guarded_charge(**dict(request.kwargs)),
            _context(),
        )
    )

    assert result["executed"] is True
    assert result["executionContext"] == {
        "source": "threadify",
        "decision": "allowed",
        "threadId": "thread-1",
        "stepName": "charge",
        "reason": "all prerequisite steps are satisfied",
        "requiredSteps": ["authenticated"],
        "satisfiedSteps": ["authenticated"],
        "missingSteps": [],
        "previousStep": "unrelated",
    }
    assert calls == ["thread-1"]


def test_guard_fails_closed_when_threadify_cannot_validate(monkeypatch):
    @guarded_step("charge", thread_id_argument="thread_id")
    async def test_guarded_charge(thread_id: str):
        return {"executed": True, "threadId": thread_id}

    callback = enforce_threadify_execution

    async def execute(_query, _variables):
        return {"data": None, "errors": [{"message": "thread not found"}]}

    monkeypatch.setitem(callback.__globals__, "execute_threadify_query", execute)
    pipeline = _pipeline()
    result = asyncio.run(
        pipeline.run(
            ToolCallRequest(
                "test_guarded_charge",
                kwargs={"thread_id": "thread-404"},
            ),
            lambda request: test_guarded_charge(**dict(request.kwargs)),
            _context(),
        )
    )

    assert result["executed"] is False
    assert result["executionContext"]["reason"] == "thread not found"


def test_exhaust_selects_and_normalizes_real_tool_execution_spans():
    span = ReadableSpan(
        name="execute_tool search_threads",
        context=SpanContext(
            trace_id=1,
            span_id=2,
            is_remote=False,
            trace_flags=TraceFlags.SAMPLED,
        ),
        resource=Resource.create({"service.name": "threadify-agent"}),
        attributes={
            "gen_ai.operation.name": "execute_tool",
            "gen_ai.tool.name": "search_threads",
            "gen_ai.agent.name": "threadify_agent",
            "gen_ai.conversation.id": "session-1",
        },
        start_time=1,
        end_time=2,
    )

    prepared = _threadify_span(span)

    assert prepared is not None
    assert prepared.attributes["threadify.step_name"] == "search_threads"
    assert prepared.attributes["threadify.label"] == (
        "threadify_agent:session-1"
    )
    assert prepared.attributes["threadify.ref.harnest_session_id"] == "session-1"
    assert prepared.attributes["threadify.tags"] == ("harnest", "agent-exhaust")


def test_exhaust_drops_non_tool_spans():
    span = ReadableSpan(
        name="call_llm",
        attributes={"gen_ai.operation.name": "chat"},
        start_time=1,
        end_time=2,
    )

    assert _threadify_span(span) is None


def test_acknowledged_action_span_targets_exact_thread_and_step():
    completion = ToolCompletion(
        tool_name="execute_guarded_charge",
        step_name="charge",
        thread_id="thread-1",
        agent_name="threadify_agent",
        session_id="session-1",
        invocation_id="invocation-1",
        started_at=1,
    )

    span = _completion_span(completion, failed=False)

    assert span.attributes["threadify.thread_id"] == "thread-1"
    assert span.attributes["threadify.step_name"] == "charge"
    assert span.attributes["threadify.context.acknowledged_completion"] is True
    assert span.status.status_code.name == "OK"


def test_tool_boundary_stamps_harnest_session_on_active_span(monkeypatch):
    attributes = {}

    class RecordingSpan:
        def is_recording(self):
            return True

        def set_attribute(self, name, value):
            attributes[name] = value

    callback = attach_threadify_trace_context
    monkeypatch.setattr(
        callback.__globals__["trace"],
        "get_current_span",
        lambda: RecordingSpan(),
    )
    context = _context()

    callback(context, ToolCallRequest("search_threads"))

    assert attributes["threadify.step_name"] == "search_threads"
    assert attributes["threadify.ref.harnest_session_id"] == "session-1"
    assert attributes["threadify.ref.harnest_agent"] == "threadify_agent"
