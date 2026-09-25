# Python SDK fallback

Bundled from the Threadify Python SDK and documentation. Confirm exact
signatures with `get_developer_reference` when it is available.

```sh
pip install threadify-sdk
```

```python
import os
from threadify import Threadify

connection = await Threadify.connect(
    os.environ["THREADIFY_API_KEY"],
    service_name="orders-service",
    engine_url="https://threadify.example.com",
)
try:
    thread = await connection.thread("order:ORD-123", {
        "label": "Order ORD-123",
        "contract": "order_processing",
        "refs": {"order_id": "ORD-123"},
    })
    await thread.step("order_received").add_context({
        "order_id": "ORD-123",
    }).success("Order accepted")
finally:
    await connection.close()
```

Use the application's stable key to resume the same active thread. Python SDK
0.3 and later adds `wait_for` and `wait_for_validation`; wait timeouts use seconds.
