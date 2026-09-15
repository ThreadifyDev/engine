from harnest.agent import tool
from harnest.lib.support_db import execute


@tool
def get_order(order_id: str) -> dict:
    """Read the item, delivery status and price for a demo order."""
    rows = execute("SELECT", "orders", "SELECT * FROM orders WHERE id = ?", (order_id.strip().upper(),))
    return rows[0] if rows else {"error": "Order not found"}
