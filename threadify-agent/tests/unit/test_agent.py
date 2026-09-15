import asyncio
from inspect import signature
from pathlib import Path


def test_compiled_agent_has_threadify_tool(agent, tools):
    assert agent.name == "threadify_agent"
    direct = {
        "search_threads",
        "get_oldest_thread",
        "get_thread",
        "get_entity_profile",
        "get_contract_violations",
    }
    compiled = {item.__name__: item for item in agent.tools}
    assert direct.issubset(tools)
    assert {
        "authenticated",
        "unrelated",
        "execute_guarded_charge",
        "propose_step",
        "verify_step_execution",
    }.issubset(compiled)
    assert "execute_graphql" not in tools
    assert all(callable(tools[name]) for name in direct)


def test_threadify_tools_match_mcp_argument_names(agent, tools):
    expected = {
        "search_threads": {
            "contractName",
            "contractVersion",
            "status",
            "actor",
            "tags",
            "startedAfter",
            "startedBefore",
            "completedAfter",
            "completedBefore",
            "limit",
            "offset",
        },
        "get_oldest_thread": {
            "contractName",
            "contractVersion",
            "status",
            "actor",
            "tags",
            "startedAfter",
            "startedBefore",
            "completedAfter",
            "completedBefore",
        },
        "get_thread": {"id"},
        "get_entity_profile": {"refKey", "type", "range"},
        "get_contract_violations": {
            "contractName",
            "refKey",
            "refValue",
            "severity",
            "startedAfter",
            "startedBefore",
            "limit",
        },
        "propose_step": {"thread_id", "step_name"},
        "verify_step_execution": {"thread_id", "step_name"},
        "authenticated": {"thread_id"},
        "unrelated": {"thread_id"},
        "execute_guarded_charge": {"thread_id"},
    }
    compiled = {item.__name__: item for item in agent.tools}
    implementations = {**tools, **compiled}
    assert {
        name: set(signature(implementations[name]).parameters)
        for name in expected
    } == expected


def test_ambiguous_threadify_parameters_are_described(tools):
    profile_docs = tools["get_entity_profile"].__doc__
    violation_docs = tools["get_contract_violations"].__doc__
    search_docs = tools["search_threads"].__doc__

    assert "identifier value" in profile_docs
    assert "Reference field name" in violation_docs
    assert "Value stored under refKey" in violation_docs
    assert "Zero-based offset" in search_docs


def test_agent_instructions_do_not_conflate_skills_with_permissions():
    instructions = (
        Path(__file__).resolve().parents[2] / "instructions.md"
    ).read_text(encoding="utf-8")

    assert "Skills are agent instruction modules" in instructions
    assert "Never use `list_skills`" in instructions
    assert "HTTP 401/403" in instructions


def test_oldest_thread_uses_bounded_count_then_exact_offset(tools, monkeypatch):
    calls = []

    async def execute(_query, variables):
        calls.append(dict(variables))
        if len(calls) == 1:
            return {
                "data": {
                    "threads": {
                        "threads": [{"id": "newest"}],
                        "totalCount": 3,
                    }
                }
            }
        return {
            "data": {
                "threads": {
                    "threads": [{"id": "oldest"}],
                    "totalCount": 3,
                }
            }
        }

    implementation = tools["get_oldest_thread"]
    while hasattr(implementation, "__wrapped__"):
        implementation = implementation.__wrapped__
    monkeypatch.setitem(
        implementation.__globals__, "execute_threadify_query", execute
    )

    result = asyncio.run(tools["get_oldest_thread"](status="COMPLETED"))

    assert [call["offset"] for call in calls] == [0, 2]
    assert all(call["limit"] == 1 for call in calls)
    assert result["data"]["threads"]["threads"][0]["id"] == "oldest"
