from pathlib import Path
import runpy


def connection():
    return runpy.run_path(str(Path(__file__).resolve().parents[2] / "mcp/threadify.py"))["client"]()


def test_mcp_configuration_does_not_capture_credentials(monkeypatch):
    monkeypatch.delenv("THREADIFY_MCP_URL", raising=False)
    monkeypatch.setenv("THREADIFY_API_KEY", "unit-test-secret-do-not-embed")
    config = connection()
    assert config.transport == "streamable-http"
    assert config.url == "http://127.0.0.1:8086/mcp"
    assert config.headers == {"X-API-Key": "${THREADIFY_API_KEY}"}
    assert "unit-test-secret-do-not-embed" not in repr(config)
    assert config.tool_name_prefix == "threadify"
    assert set(config.tool_filter) == {"search_threads", "get_thread", "get_entity_profile", "get_contract_violations"}


def test_mcp_endpoint_override(monkeypatch):
    monkeypatch.setenv("THREADIFY_MCP_URL", "http://127.0.0.1:8083/mcp")
    assert connection().url == "http://127.0.0.1:8083/mcp"


def test_agent_uses_mcp_without_authored_database_tools(agent, tools):
    assert agent.name == "threadify_mcp_agent"
    assert not tools
