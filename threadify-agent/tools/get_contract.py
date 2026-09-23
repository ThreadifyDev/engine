from typing import Any
from uuid import UUID
from harnest.agent import tool
from harnest.lib.threadify_management import read_contracts
from harnest.lib.threadify_tooling import tool_error


@tool
async def get_contract(id: str, version: int | None = None) -> dict[str, Any]:
    """Read a contract's source and metadata, including an optional exact version.

    Args:
        id: Exact contract UUID from list_contracts or a thread's contractId.
        version: Positive version number; omit to read the latest version.
    """
    try:
        contract_id = str(UUID(id))
    except (ValueError, TypeError, AttributeError):
        return tool_error("id must be a contract UUID.")
    if version is not None and (isinstance(version, bool) or not isinstance(version, int) or version < 1):
        return tool_error("version must be a positive integer.")
    path = f"/v1/contracts/{contract_id}"
    if version is not None:
        path += f"/versions/{version}"
    return await read_contracts(path, {})
