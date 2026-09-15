def test_support_surface(agent, tools):
    assert agent.name == "customer_support_agent"
    assert set(tools) == {"lookup_customer", "get_order", "create_support_ticket", "list_support_tickets"}


def test_real_database_tools(tools, monkeypatch, tmp_path):
    # Resolve the authored library through the compiler's public tool fixture.
    implementation = tools["get_order"]
    while hasattr(implementation, "__wrapped__"):
        implementation = implementation.__wrapped__
    execute = implementation.__globals__["execute"]
    monkeypatch.setenv("SUPPORT_DATABASE_PATH", str(tmp_path / "support.sqlite3"))
    execute.__globals__["initialize"]()
    assert tools["lookup_customer"](email="ALEX@example.test")["id"] == "CUST-001"
    assert tools["get_order"](order_id="ORD-1001")["status"] == "delayed"
    assert "error" in tools["get_order"](order_id="' OR 1=1 --")
    assert "error" in tools["create_support_ticket"](customer_id="CUST-001", issue=" ")
    assert "error" in tools["create_support_ticket"](customer_id="unknown", issue="Late")
    ticket = tools["create_support_ticket"](customer_id="CUST-001", issue="Order is late")
    assert ticket["status"] == "open"
    tickets = tools["list_support_tickets"](customer_id="CUST-001")["tickets"]
    assert tickets[0]["id"] == ticket["id"]
    assert tools["list_support_tickets"](customer_id="CUST-002")["tickets"] == []
