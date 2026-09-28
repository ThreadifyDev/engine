"""Connect to the threadify MCP server."""

import os

from harnest.mcp import MCPClient


def client() -> MCPClient:
    """Create the remote MCP client from non-secret authored configuration."""

    return MCPClient.streamable_http(
        os.environ.get("THREADIFY_MCP_URL", "http://127.0.0.1:8086/mcp"),
        headers={"X-API-Key": "${THREADIFY_API_KEY}"},
        tools=("search_threads", "get_thread", "get_entity_profile", "get_contract_violations"),
        prefix="threadify",
    )
