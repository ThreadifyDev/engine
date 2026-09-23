from collections.abc import Mapping
from typing import Any

from harnest.lib.threadify_tooling import (
    execute_threadify_query,
    optional_variables,
)
from harnest.agent import tool


_QUERY = """
query GetOldestThread(
  $contractName: String
  $contractVersion: Int
  $status: String
  $actor: String
  $tags: [String!]
  $startedAfter: String
  $startedBefore: String
  $completedAfter: String
  $completedBefore: String
  $limit: Int
  $offset: Int
) {
  threads(
    contractName: $contractName
    contractVersion: $contractVersion
    status: $status
    actor: $actor
    tags: $tags
    startedAfter: $startedAfter
    startedBefore: $startedBefore
    completedAfter: $completedAfter
    completedBefore: $completedBefore
    limit: $limit
    offset: $offset
  ) {
    threads {
      id
      status
      contractName
      contractVersion
      startedAt
      completedAt
      error
    }
    totalCount
  }
}
"""


@tool
async def get_oldest_thread(
    contractName: str | None = None,
    contractVersion: int | None = None,
    status: str | None = None,
    actor: str | None = None,
    tags: list[str] | None = None,
    startedAfter: str | None = None,
    startedBefore: str | None = None,
    completedAfter: str | None = None,
    completedBefore: str | None = None,
) -> dict[str, Any]:
    """Return the single oldest Threadify thread matching typed filters.

    Args:
        contractName: Exact contract name, not a thread identifier.
        contractVersion: Exact integer contract version.
        status: Thread status such as ACTIVE, COMPLETED, or FAILED.
        actor: Exact actor identifier.
        tags: Tags that matching threads must contain.
        startedAfter: Include starts after this ISO-8601 timestamp.
        startedBefore: Include starts before this ISO-8601 timestamp.
        completedAfter: Include completions after this ISO-8601 timestamp.
        completedBefore: Include completions before this ISO-8601 timestamp.
    """

    variables = optional_variables(
        contractName=contractName,
        contractVersion=contractVersion,
        status=status,
        actor=actor,
        tags=tags,
        startedAfter=startedAfter,
        startedBefore=startedBefore,
        completedAfter=completedAfter,
        completedBefore=completedBefore,
        limit=1,
        offset=0,
    )
    first_page = await execute_threadify_query(_QUERY, variables)
    total = _total_count(first_page)
    if total is None or total <= 1:
        return first_page
    # Threadify owns newest-first ordering. A second bounded page selects the
    # oldest row without loading and filtering the full result set in the agent.
    variables["offset"] = total - 1
    return await execute_threadify_query(_QUERY, variables)


def _total_count(result: Mapping[str, Any]) -> int | None:
    """Read the bounded pagination count from a successful GraphQL envelope."""

    data = result.get("data")
    connection = data.get("threads") if isinstance(data, Mapping) else None
    total = connection.get("totalCount") if isinstance(connection, Mapping) else None
    if not isinstance(total, int) or isinstance(total, bool) or total < 0:
        return None
    return total
