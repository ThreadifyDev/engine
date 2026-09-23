from typing import Any
from harnest.agent import client_tool


@client_tool
def navigate_ui(page: str, id: str = "", profile_type: str = "", ref_key: str = "") -> dict[str, Any]:
    """Navigate the connected Threadify UI to an allowlisted workspace page.

    Args:
        page: One of dashboard, threads, thread, contracts, contract, profiles, entity_profile, profile_designer, trace_settings.
        id: Exact thread or contract UUID, required for thread and contract pages.
        profile_type: Exact entity profile type name, required for entity_profile and profile_designer.
        ref_key: Entity identifier value (not field name), required for entity_profile.
    """
    ...
