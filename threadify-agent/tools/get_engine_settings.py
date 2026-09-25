from typing import Any

from harnest.agent import client_tool


@client_tool
def get_engine_settings() -> dict[str, Any]:
    """Read the authenticated workspace's saved Engine URL and its source from Settings. This is a fresh backend read, not a view of unsaved form edits or other Settings fields. It works independently of page context and never returns API keys."""
    ...
