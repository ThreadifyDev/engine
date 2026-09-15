from uuid import UUID

from harnest.agent import tool
from harnest.lib.threadify_reader import read_credential, read_support_run


@tool
async def inspect_run(thread_id: str) -> dict:
    """Inspect a customer support run's recorded steps, status, context, and timestamps.

    Args:
        thread_id: A Threadify UUID from list_support_runs or supplied by the user.
    """
    try:
        normalized_id = str(UUID(thread_id))
    except (ValueError, TypeError, AttributeError):
        return {"error": "thread_id must be a UUID."}
    try:
        key = await read_credential()
        thread = await read_support_run(normalized_id, key)
        return {"thread": thread} if thread else {"error": "Customer support run not found."}
    except Exception as error:
        return {"error": f"Threadify read failed ({type(error).__name__})."}
