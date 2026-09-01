from typing import Any

from harnest.lib.threadify_tooling import (
    bounded_limit,
    bounded_offset,
    execute_threadify_query,
    optional_variables,
)
from harnest.tool import tool


_QUERY = """
query SearchThreads(
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
async def search_threads(
    contractName: str | None = None,
    contractVersion: int | None = None,
    status: str | None = None,
    actor: str | None = None,
    tags: list[str] | None = None,
    startedAfter: str | None = None,
    startedBefore: str | None = None,
    completedAfter: str | None = None,
    completedBefore: str | None = None,
    limit: int = 50,
    offset: int = 0,
) -> dict[str, Any]:
    """Search newest-first Threadify threads with typed filters and pagination.

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
        limit: Page size from 1 through 100.
        offset: Zero-based offset in the newest-first result set.
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
        limit=bounded_limit(limit),
        offset=bounded_offset(offset),
    )
    return await execute_threadify_query(_QUERY, variables)
