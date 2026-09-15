"""Real DB -> runtime OTLP exporter -> Threadify readback, with no model call."""
import json
import os
import time
import uuid

import httpx


def test_support_database_otel_reaches_threadify(client, tools):
    assert client.get("/healthz").status_code == 200
    implementation = tools["get_order"]
    while hasattr(implementation, "__wrapped__"):
        implementation = implementation.__wrapped__
    execute = implementation.__globals__["execute"]
    business_span = execute.__globals__["span"]
    marker = str(uuid.uuid4())
    # Match real Harnest: the invocation has an HTTP parent omitted by the exporter.
    with business_span("unexported.http.parent"):
        with business_span("support.telemetry.connection_check", attributes={"gen_ai.operation.name": "invoke_agent", "gen_ai.agent.name": "customer_support_agent", "threadify.ref.connection_check": marker}) as root:
            trace_id = format(root.get_span_context().trace_id, "032x")
            assert int(trace_id, 16) != 0
            customer = tools["lookup_customer"](email="alex@example.test")
            assert tools["get_order"](order_id="ORD-1001")["status"] == "delayed"
            ticket = tools["create_support_ticket"](customer_id=customer["id"], issue="Synthetic OTEL connection check " + marker)
            assert any(t["id"] == ticket["id"] for t in tools["list_support_tickets"](customer_id=customer["id"])["tickets"])
    url = os.environ["THREADIFY_OTLP_ENDPOINT"].removesuffix("/v1/traces") + "/graphql"
    query = 'query($value:String!){threadsByRef(refKey:"connection_check",refValue:$value,limit:1){threads{id status completedAt refs steps{stepName latestContext}}}}'
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        response = httpx.post(url, headers={"X-API-Key": os.environ["THREADIFY_OTLP_API_KEY"]}, json={"query": query, "variables": {"value": marker}}, timeout=5)
        response.raise_for_status()
        payload = response.json()
        assert not payload.get("errors"), payload.get("errors")
        threads = payload["data"]["threadsByRef"]["threads"]
        if threads and len(threads[0]["steps"]) == 7 and threads[0]["status"] == "completed":
            thread = threads[0]
            break
        time.sleep(0.5)
    else:
        raise AssertionError("Business trace did not reach Threadify")
    assert thread["completedAt"]
    refs = json.loads(thread["refs"]) if isinstance(thread["refs"], str) else thread["refs"]
    assert refs["harnest_agent"] == "customer_support_agent"
    assert any(step["stepName"] == "db.insert.tickets" for step in thread["steps"])
    assert any(str(ticket["id"]) in str(step["latestContext"]) for step in thread["steps"] if step["stepName"] == "db.insert.tickets")
    integrity_query = 'query($id:String!){verifyThreadIntegrity(threadId:$id){verified totalEvents error}}'
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        response = httpx.post(url, headers={"X-API-Key": os.environ["THREADIFY_OTLP_API_KEY"]}, json={"query": integrity_query, "variables": {"id": thread["id"]}}, timeout=5)
        response.raise_for_status()
        payload = response.json()
        assert not payload.get("errors"), payload.get("errors")
        integrity = payload["data"]["verifyThreadIntegrity"]
        if integrity["verified"] and integrity["totalEvents"] == 7:
            break
        time.sleep(0.5)
    else:
        raise AssertionError("Completed run's seven-event hash chain did not verify")
    report = os.environ.get("SUPPORT_VERIFICATION_FILE")
    if report:
        from pathlib import Path
        Path(report).write_text(json.dumps({"thread_id": thread["id"], "trace_id": trace_id, "ticket_id": ticket["id"], "step_count": len(thread["steps"]), "model_invoked": False, "status": thread["status"], "completed_at": thread["completedAt"], "hash_chain_verified": integrity["verified"]}, indent=2) + "\n")
