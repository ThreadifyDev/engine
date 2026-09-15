"""Offline contract for the reviewer Nanite."""


def test_agent_identity_and_tool_surface(agent, tools):
    """Keep review event-driven and free from direct agent tools."""
    assert agent.name == "reviewer_nanite"
    assert tools == {}
