"""Offline contract for the analyst Nanite."""


def test_agent_identity_and_tool_surface(agent, tools):
    """Keep analysis event-driven and free from direct agent tools."""
    assert agent.name == "analyst_nanite"
    assert tools == {}
