from harnest.agent import tool
from harnest.lib.support_db import execute


@tool
def create_support_ticket(customer_id: str, issue: str) -> dict:
    """Create a persistent support ticket for a known demo customer."""
    if not issue.strip() or len(issue) > 2000:
        return {"error": "Issue must contain 1 to 2000 characters"}
    customer_id = customer_id.strip().upper()
    if not execute("SELECT", "customers", "SELECT id FROM customers WHERE id = ?", (customer_id,)):
        return {"error": "Customer not found"}
    return execute("INSERT", "tickets", "INSERT INTO tickets (customer_id, issue) VALUES (?, ?) RETURNING *", (customer_id, issue.strip()))[0]
