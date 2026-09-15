"""Opt-in read-only calls through Harnest's actual MCP adapter; no model call."""
import asyncio
import json
import os
from pathlib import Path
import runpy

import pytest


def test_threadify_mcp_reads():
    if not os.environ.get("THREADIFY_API_KEY"):
        pytest.skip("Set THREADIFY_API_KEY for the live MCP smoke test")

    async def exercise():
        factory = runpy.run_path(str(Path(__file__).resolve().parents[2] / "mcp/threadify.py"))["client"]
        toolset = factory().to_adk_toolset()
        try:
            tools = {tool.name: tool for tool in await toolset.get_tools()}
            search = next(tool for name, tool in tools.items() if name.endswith("search_threads"))
            result = await search.run_async(args={"limit": 2}, tool_context=None)
            # MCP returns GraphQL JSON inside a text content block.
            if hasattr(result, "model_dump"):
                result = result.model_dump()
            assert not result.get("isError"), result
            content = result["content"]
            payload = json.loads(next(item["text"] for item in content if item.get("type") == "text"))
            assert not payload.get("errors"), payload
            assert "threads" in payload["data"]
            expected = os.environ.get("THREADIFY_SMOKE_THREAD_ID")
            if expected:
                get_thread = next(tool for name, tool in tools.items() if name.endswith("get_thread"))
                detail = await get_thread.run_async(args={"id": expected}, tool_context=None)
                if hasattr(detail, "model_dump"):
                    detail = detail.model_dump()
                assert not detail.get("isError"), detail
                payload = json.loads(next(item["text"] for item in detail["content"] if item.get("type") == "text"))
                assert not payload.get("errors"), payload
                assert payload["data"]["thread"]["id"] == expected
        finally:
            await toolset.close()
    asyncio.run(exercise())
