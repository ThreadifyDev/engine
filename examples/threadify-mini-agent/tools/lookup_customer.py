from harnest.agent import tool
from harnest.lib.support_db import execute


@tool
def lookup_customer(email: str) -> dict:
    """Look up a demo customer and their orders by email address."""
    customers = execute("SELECT", "customers", "SELECT * FROM customers WHERE email = ?", (email.strip().lower(),))
    if not customers:
        return {"error": "Customer not found"}
    customer = customers[0]
    customer["orders"] = execute("SELECT", "orders", "SELECT * FROM orders WHERE customer_id = ?", (customer["id"],))
    return customer
