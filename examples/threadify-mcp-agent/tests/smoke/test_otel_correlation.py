"""Opt-in real agent/model/MCP run, exporting two traces for one logical workflow."""
import json
import os
from pathlib import Path

import pytest


@pytest.mark.skipif(
    os.environ.get("THREADIFY_AGENT_CORRELATION_TEST") != "1",
    reason="Requires the disposable Engine correlation fixture and configured model",
)
def test_agent_exports_two_correlated_requests(smoke):
    from harnest.tracing import span
    from opentelemetry import trace

    expected = os.environ["THREADIFY_SMOKE_THREAD_ID"]
    evidence = []
    for number in (1, 2):
        with span(
            f"sample.agent.inspect.{number}",
            attributes={"threadify.step_name": f"agent.inspect.{number}"},
        ) as root:
            trace_id = format(root.get_span_context().trace_id, "032x")
            assert trace_id != "0" * 32, "Harnest tracing was not enabled"
            response = smoke.respond(
                f"Use the Threadify get_thread MCP tool to inspect the synthetic test thread {expected}. "
                "Report its ID, recorded status, and one exact step name. Do not modify data.",
            )
            assert response["status"] == "completed", response
            results = [item for item in response["output"] if item["type"] == "tool_result"]
            result = next(item for item in results if item["name"].endswith("get_thread"))
            payload = result["output"]
            if isinstance(payload, str):
                payload = json.loads(payload)
            assert not payload.get("isError"), payload
            data = json.loads(next(item["text"] for item in payload["content"] if item.get("type") == "text"))
            assert not data.get("errors"), data
            thread = data["data"]["thread"]
            assert thread["id"] == expected
            answer = response["outputText"]
            assert expected in answer
            assert thread["status"].lower() in answer.lower()
            assert any(step["stepName"] in answer for step in thread["steps"]), answer
            evidence.append({"trace_id": trace_id, "tools": [item["name"] for item in results], "answer": answer})
    assert evidence[0]["trace_id"] != evidence[1]["trace_id"]
    assert trace.get_tracer_provider().force_flush(timeout_millis=15000)
    Path(os.environ["THREADIFY_AGENT_EVIDENCE_FILE"]).write_text(json.dumps(evidence, indent=2))
