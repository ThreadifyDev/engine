from typing import Any
from harnest.agent import client_tool


@client_tool
def preview_contract_draft(expected_revision: int) -> dict[str, Any]:
    """Validate the connected frontend's current contract draft with the Engine compiler and show diagnostics. Does not publish.

    Args:
        expected_revision: Exact draft revision returned by get_page_context or open_contract_draft.
    """
    ...
