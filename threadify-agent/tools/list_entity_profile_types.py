from typing import Any
from harnest.agent import tool
from harnest.lib.threadify_tooling import execute_threadify_query


@tool
async def list_entity_profile_types() -> dict[str, Any]:
    """List accessible entity profile types before searching for an entity."""
    return await execute_threadify_query("query { entityProfileTypes { id name type description } }")
