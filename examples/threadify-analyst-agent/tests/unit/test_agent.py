import asyncio
from inspect import signature


def implementation(tool):
    while hasattr(tool, "__wrapped__"):
        tool = tool.__wrapped__
    return tool


def globals_for(tools, name):
    return implementation(tools[name]).__globals__


def test_read_only_tool_surface(agent, tools):
    assert agent.name == "threadify_analyst_agent"
    assert set(tools) == {"list_support_runs", "inspect_run"}
    assert set(signature(tools["list_support_runs"]).parameters) == {"limit"}
    assert set(signature(tools["inspect_run"]).parameters) == {"thread_id"}


def test_invalid_inputs_do_not_resolve_credentials(tools, monkeypatch):
    async def forbidden():
        raise AssertionError("Unexpected credential lookup")
    for name in tools:
        monkeypatch.setitem(globals_for(tools, name), "read_credential", forbidden)
    for limit in (0, 11, -1, "5", 1.5, True):
        assert asyncio.run(tools["list_support_runs"](limit=limit)) == {
            "error": "limit must be an integer from 1 to 10."
        }
    for value in ("bad", "", None):
        assert asyncio.run(tools["inspect_run"](thread_id=value)) == {
            "error": "thread_id must be a UUID."
        }


def test_list_passes_bounded_limit_with_private_credential(tools, monkeypatch):
    calls = []
    private_key = object()
    async def credential():
        return private_key
    async def read(limit, key):
        calls.append((limit, key))
        return {"totalCount": 12, "threads": [{"id": "example", "status": "completed"}]}
    namespace = globals_for(tools, "list_support_runs")
    monkeypatch.setitem(namespace, "read_credential", credential)
    monkeypatch.setitem(namespace, "read_support_runs", read)
    result = asyncio.run(tools["list_support_runs"](limit=3))
    assert calls == [(3, private_key)]
    assert result == {"totalCount": 12, "threads": [{"id": "example", "status": "completed"}]}


def test_inspect_returns_evidence_and_normalizes_uuid(tools, monkeypatch):
    calls = []
    async def credential():
        return "private"
    async def read(thread_id, key):
        calls.append((thread_id, key))
        return {"id": thread_id, "steps": [{"stepName": "db.query", "status": "success"}]}
    namespace = globals_for(tools, "inspect_run")
    monkeypatch.setitem(namespace, "read_credential", credential)
    monkeypatch.setitem(namespace, "read_support_run", read)
    thread_id = "01234567-89AB-CDEF-0123-456789ABCDEF"
    result = asyncio.run(tools["inspect_run"](thread_id=thread_id))
    assert calls == [(thread_id.lower(), "private")]
    assert result["thread"]["steps"][0]["stepName"] == "db.query"


def test_transport_errors_are_sanitized(tools, monkeypatch):
    async def credential():
        raise RuntimeError("secret-key-and-private-endpoint")
    for name in tools:
        monkeypatch.setitem(globals_for(tools, name), "read_credential", credential)
    assert asyncio.run(tools["list_support_runs"]()) == {
        "error": "Threadify read failed (RuntimeError)."
    }
    assert asyncio.run(tools["inspect_run"](thread_id="01234567-89ab-cdef-0123-456789abcdef")) == {
        "error": "Threadify read failed (RuntimeError)."
    }


def test_reader_only_returns_support_threads(tools, monkeypatch):
    read = globals_for(tools, "inspect_run")["read_support_run"]
    for candidate in (None, {"id": "foreign", "refs": {}}, {"refs": {"harnest_agent": "other"}}):
        async def graphql(query, variables, key):
            return {"thread": candidate}
        monkeypatch.setitem(read.__globals__, "graphql", graphql)
        assert asyncio.run(read("thread", "key")) is None
    support = {"id": "support", "refs": {"harnest_agent": "customer_support_agent"}}
    async def graphql(query, variables, key):
        return {"thread": support}
    monkeypatch.setitem(read.__globals__, "graphql", graphql)
    assert asyncio.run(read("thread", "key")) == support


def test_fixed_graphql_queries_and_key_header(tools, monkeypatch):
    import json
    import httpx

    read = globals_for(tools, "list_support_runs")["read_support_runs"]
    namespace = read.__globals__
    requests = []
    def handle(request):
        requests.append(request)
        return httpx.Response(200, json={"data": {"threadsByRef": {"totalCount": 0, "threads": []}}})
    original_client = httpx.AsyncClient
    def client(**options):
        assert options["follow_redirects"] is False
        return original_client(**options, transport=httpx.MockTransport(handle))
    class Key:
        def reveal(self):
            return "test-only-credential"
    monkeypatch.setenv("THREADIFY_GRAPHQL_URL", "http://127.0.0.1:8083/graphql")
    monkeypatch.setattr(namespace["httpx"], "AsyncClient", client)
    assert asyncio.run(read(2, Key())) == {"totalCount": 0, "threads": []}
    assert str(requests[0].url) == "http://127.0.0.1:8083/graphql"
    assert requests[0].headers["X-API-Key"] == "test-only-credential"
    body = json.loads(requests[0].content)
    assert body["variables"] == {"limit": 2}
    assert 'refKey: "harnest_agent"' in body["query"]
    assert 'refValue: "customer_support_agent"' in body["query"]
    assert "mutation" not in body["query"]
    assert "test-only-credential" not in body["query"]
    for field in ("status", "actorService", "latestContext", "startedAt", "finishedAt"):
        assert field in namespace["INSPECT_QUERY"]


def test_serialized_refs_and_context_are_normalized(tools, monkeypatch):
    import json

    listing = globals_for(tools, "list_support_runs")["read_support_runs"]
    inspect = globals_for(tools, "inspect_run")["read_support_run"]
    source = {
        "id": "support", "refs": json.dumps({"harnest_agent": "customer_support_agent"}),
        "steps": [
            {"stepName": "db.query", "latestContext": '{"rows":1,"table":"orders"}'},
            {"stepName": "context_already_parsed", "latestContext": {"count": 2}},
            {"stepName": "plain_text", "latestContext": "not JSON"},
        ],
    }
    async def graphql(query, variables, key):
        return {"thread": source, "threadsByRef": {"totalCount": 1, "threads": [source]}}
    monkeypatch.setitem(inspect.__globals__, "graphql", graphql)
    run = asyncio.run(inspect("support", "key"))
    assert run["refs"] == {"harnest_agent": "customer_support_agent"}
    assert run["steps"][0]["latestContext"] == {"rows": 1, "table": "orders"}
    assert run["steps"][1]["latestContext"] == {"count": 2}
    assert run["steps"][2]["latestContext"] == "not JSON"
    assert isinstance(source["refs"], str)  # Normalize a copy, not shared source records.
    result = asyncio.run(listing(1, "key"))
    assert result["threads"][0] == run
    assert result["totalCount"] == 1


def test_invalid_serialized_refs_are_not_support_runs(tools, monkeypatch):
    inspect = globals_for(tools, "inspect_run")["read_support_run"]
    for refs in ('"text"', '[1,2]', 'null', 'not JSON'):
        async def graphql(query, variables, key):
            return {"thread": {"id": "foreign", "refs": refs}}
        monkeypatch.setitem(inspect.__globals__, "graphql", graphql)
        assert asyncio.run(inspect("foreign", "key")) is None
