from harnest.agent import tool
from harnest.lib.threadify_reader import read_credential, read_support_runs


@tool
async def list_support_runs(limit: int = 5) -> dict:
    """List the most recent customer support runs recorded in Threadify.

    Args:
        limit: Number of recent runs to read, from 1 to 10.
    """
    if type(limit) is not int or not 1 <= limit <= 10:
        return {"error": "limit must be an integer from 1 to 10."}
    try:
        key = await read_credential()
        return await read_support_runs(limit, key)
    except Exception as error:
        return {"error": f"Threadify read failed ({type(error).__name__})."}
