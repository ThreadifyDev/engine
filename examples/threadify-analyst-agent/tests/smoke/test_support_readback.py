"""Read a real support DB/OTEL smoke run through analyst tools; no model call."""
import asyncio
import json
import os
from pathlib import Path

import pytest


def test_analyst_reads_support_telemetry(tools, monkeypatch):
    report_path = os.environ.get("SUPPORT_VERIFICATION_FILE")
    if not report_path:
        pytest.skip("Set SUPPORT_VERIFICATION_FILE to the support OTEL smoke report")
    report = json.loads(Path(report_path).read_text())
    private_key = os.environ["THREADIFY_API_KEY"]
    class Credential:
        def reveal(self):
            return private_key
    async def credential():
        return Credential()
    for name in ("list_support_runs", "inspect_run"):
        implementation = tools[name]
        while hasattr(implementation, "__wrapped__"):
            implementation = implementation.__wrapped__
        monkeypatch.setitem(implementation.__globals__, "read_credential", credential)
    listing = asyncio.run(tools["list_support_runs"](limit=10))
    assert "error" not in listing, listing
    assert any(thread["id"] == report["thread_id"] for thread in listing["threads"])
    assert all(thread["refs"]["harnest_agent"] == "customer_support_agent" for thread in listing["threads"])
    result = asyncio.run(tools["inspect_run"](thread_id=report["thread_id"]))
    assert "error" not in result, result
    thread = result["thread"]
    assert thread["id"] == report["thread_id"]
    assert len(thread["steps"]) >= report["step_count"]
    insert_steps = [step for step in thread["steps"] if step["stepName"] == "db.insert.tickets"]
    assert insert_steps
    assert any(str(report["ticket_id"]) in json.dumps(step["latestContext"]) for step in insert_steps)
    assert all(step["actorService"] for step in insert_steps)
    assert all(step["startedAt"] and step["finishedAt"] for step in insert_steps)
    assert private_key not in json.dumps({"list": listing, "inspect": result})
