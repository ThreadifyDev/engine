from typing import Any

from harnest.lib.threadify_tooling import (
    execute_threadify_query,
    optional_variables,
    required_text,
    tool_error,
)
from harnest.tool import tool


_ALLOWED_RANGES = frozenset({"7d", "30d", "90d"})
_QUERY = """
query GetEntityProfile($refKey: String, $type: String, $range: String) {
  entityProfile(refKey: $refKey, type: $type) {
    id
    refKey
    name
    createdAt
    lastActiveAt
    metrics {
      totalDeliveries
      completedSuccessfully
      validationViolations
      deliveryHealthScore
      prevDeliveryHealthScore
      healthTrendSlope
      averageDeliveryTimeMs
    }
    computedMetrics(range: $range)
  }
}
"""


@tool
async def get_entity_profile(
    refKey: str,
    type: str,
    range: str = "7d",
) -> dict[str, Any]:
    """Get an entity profile with health, delivery metrics, and trend data.

    Args:
        refKey: The entity identifier value, for example CUST-4981 or user_123;
            this is not a reference field name.
        type: The profile type paired with refKey, such as Customer profile.
        range: Computed-metric window: 7d, 30d, or 90d.
    """

    normalized_ref = required_text(refKey, "refKey")
    normalized_type = required_text(type, "type")
    if normalized_ref is None or normalized_type is None:
        return tool_error("refKey and type are required non-empty strings.")
    normalized_range = range.strip() if isinstance(range, str) else "7d"
    if normalized_range not in _ALLOWED_RANGES:
        return tool_error("range must be one of: 7d, 30d, 90d.")

    variables = optional_variables(
        refKey=normalized_ref,
        type=normalized_type,
        range=normalized_range,
    )
    return await execute_threadify_query(_QUERY, variables)
