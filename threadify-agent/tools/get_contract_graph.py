from typing import Any
from harnest.agent import tool
from harnest.lib.threadify_tooling import execute_threadify_query, required_text, tool_error, optional_variables

_QUERY = """query GetContractGraph($name: String!, $version: Int) {
  contractGraph(name: $name, version: $version) {
    graph { entryPoints terminalSteps nodes { id owner type mode required dependsOn next steps timeout maxDuration businessContext parentGroup } }
    transitions { from to timeout maxRetries }
    parties
    validation { maxDuration allowMultipleTerminals multipleTerminalsSeverity }
  }
}"""


@tool
async def get_contract_graph(name: str, version: int | None = None) -> dict[str, Any]:
    """Read a contract's compiled graph, owners, prerequisites, and transitions.

    Args:
        name: Exact contract name, not its UUID, returned by a thread or list_contracts.
        version: Positive version number; omit to select the latest.
    """
    value = required_text(name, "name")
    if value is None or (version is not None and (isinstance(version, bool) or not isinstance(version, int) or version < 1)):
        return tool_error("Provide a contract name and an optional positive version.")
    return await execute_threadify_query(_QUERY, optional_variables(name=value, version=version))
