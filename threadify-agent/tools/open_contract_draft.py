from typing import Any
from harnest.agent import client_tool


@client_tool
def open_contract_draft(source: str, expected_revision: int) -> dict[str, Any]:
    """Open and populate the frontend contract editor with a local Gherkin draft. Does not save or publish.

    Args:
        source: Complete Threadify Gherkin source beginning with Feature; use only supported clauses.
        expected_revision: Exact local draft revision from get_page_context; conflicts preserve newer user edits.
    """
    ...
