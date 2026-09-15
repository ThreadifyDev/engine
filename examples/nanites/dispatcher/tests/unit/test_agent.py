"""Offline contract for the Nanite dispatcher."""


def test_agent_identity_and_tool_surface(agent, tools):
    """Expose only the bounded workflow-submission tool."""
    assert agent.name == "nanite_dispatcher"
    assert set(tools) == {"submit_work"}
