from harnest.agent import tool
from harnest.lib.support_db import execute


@tool
def list_support_tickets(customer_id: str) -> dict:
    """List the latest 20 support tickets for a demo customer."""
    return {"tickets": execute("SELECT", "tickets", "SELECT * FROM tickets WHERE customer_id = ? ORDER BY id DESC LIMIT 20", (customer_id.strip().upper(),))}
