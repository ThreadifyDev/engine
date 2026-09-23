from typing import Any
from harnest.agent import tool
from harnest.lib.threadify_management import read_contracts
from harnest.lib.threadify_tooling import bounded_limit, bounded_offset


@tool
async def list_contracts(search: str = "", limit: int = 20, offset: int = 0) -> dict[str, Any]:
    """Find accessible contracts and their exact IDs before reading or opening one.

    Args:
        search: Contract name search text; empty lists accessible contracts.
        limit: Page size, bounded to 1..100.
        offset: Zero-based result offset.
    """
    return await read_contracts("/v1/contracts", {"search": search[:500], "limit": bounded_limit(limit), "offset": bounded_offset(offset)})
