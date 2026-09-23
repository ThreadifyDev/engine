from typing import Any
from harnest.agent import tool
from harnest.lib.threadify_tooling import execute_threadify_query, bounded_limit, bounded_offset, required_text, tool_error

_QUERY = """query ListEntityProfiles($type: String!, $search: String, $limit: Int, $offset: Int) {
  entityProfilesByType(type: $type, search: $search, limit: $limit, offset: $offset) {
    items { id refKey name }
    totalCount
  }
}"""


@tool
async def list_entity_profiles(type: str, search: str = "", limit: int = 20, offset: int = 0) -> dict[str, Any]:
    """Find entity identifiers within a known profile type.

    Args:
        type: Exact profile type name returned by list_entity_profile_types.
        search: Optional text matched against entity names and reference values.
        limit: Page size, bounded to 1..100.
        offset: Zero-based result offset.
    """
    value = required_text(type, "type")
    if value is None:
        return tool_error("type must be a non-empty profile type name.")
    return await execute_threadify_query(_QUERY, {"type": value, "search": search[:500], "limit": bounded_limit(limit), "offset": bounded_offset(offset)})
