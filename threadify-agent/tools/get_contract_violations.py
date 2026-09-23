from typing import Any

from harnest.lib.threadify_tooling import (
    bounded_limit,
    execute_threadify_query,
    optional_variables,
)
from harnest.agent import tool


_QUERY = """
query GetContractViolations(
  $contractName: String
  $refKey: String
  $refValue: String
  $severity: [String!]
  $startedAfter: String
  $startedBefore: String
  $limit: Int
) {
  contractViolations(
    contractName: $contractName
    refKey: $refKey
    refValue: $refValue
    severity: $severity
    startedAfter: $startedAfter
    startedBefore: $startedBefore
    limit: $limit
  ) {
    threadId
    stepName
    source
    notificationType
    severity
    message
    details
    timestamp
  }
}
"""


@tool
async def get_contract_violations(
    contractName: str | None = None,
    refKey: str | None = None,
    refValue: str | None = None,
    severity: list[str] | None = None,
    startedAfter: str | None = None,
    startedBefore: str | None = None,
    limit: int = 50,
) -> dict[str, Any]:
    """Find contract violations by contract, reference, severity, or time.

    Args:
        contractName: Exact contract name.
        refKey: Reference field name, for example customerId or orderId.
        refValue: Value stored under refKey, for example CUST-456; never put the
            field name here.
        severity: Accepted violation severities.
        startedAfter: Include violations after this ISO-8601 timestamp.
        startedBefore: Include violations before this ISO-8601 timestamp.
        limit: Maximum results from 1 through 100.
    """

    variables = optional_variables(
        contractName=contractName,
        refKey=refKey,
        refValue=refValue,
        severity=severity,
        startedAfter=startedAfter,
        startedBefore=startedBefore,
        limit=bounded_limit(limit),
    )
    return await execute_threadify_query(_QUERY, variables)
