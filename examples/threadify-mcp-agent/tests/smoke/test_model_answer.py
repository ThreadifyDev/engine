"""Explicitly enabled end-to-end model + MCP test using a known test thread."""
import json
import os
from pathlib import Path

import pytest


@pytest.mark.skipif(os.environ.get("THREADIFY_LIVE_MODEL_TEST") != "1", reason="Live model invocation requires THREADIFY_LIVE_MODEL_TEST=1")
def test_model_reads_thread_and_cites_evidence(smoke):
    thread_id = os.environ["THREADIFY_SMOKE_THREAD_ID"]
    response = smoke.respond(
        f"Use the Threadify get_thread MCP tool to inspect thread {thread_id}. "
        "Report its ID, recorded status, and at least one exact step name. "
        "Do not modify data or make claims unsupported by the tool result."
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
    assert thread["id"] == thread_id
    answer = response["outputText"]
    assert thread_id in answer
    assert thread["status"].lower() in answer.lower()
    assert any(step["stepName"] in answer for step in thread["steps"])
    if path := os.environ.get("THREADIFY_MODEL_EVIDENCE_FILE"):
        Path(path).write_text(json.dumps({"status": response["status"], "thread_id": thread_id, "tools": [item["name"] for item in results], "answer": answer}, indent=2))
