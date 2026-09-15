"""Persistent, synthetic business data; no Threadify client belongs here."""
import os
import sqlite3
from contextlib import closing
from pathlib import Path
from harnest.tracing import span


def database_path():
    value = os.environ.get("SUPPORT_DATABASE_PATH")
    if not value:
        raise RuntimeError("Set SUPPORT_DATABASE_PATH to the demo SQLite database")
    return Path(value)


def initialize():
    path = database_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    with closing(sqlite3.connect(path)) as db, db:
        db.executescript("""
        CREATE TABLE IF NOT EXISTS customers (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL);
        CREATE TABLE IF NOT EXISTS orders (id TEXT PRIMARY KEY, customer_id TEXT NOT NULL REFERENCES customers(id), item TEXT NOT NULL, status TEXT NOT NULL, total_cents INTEGER NOT NULL);
        CREATE TABLE IF NOT EXISTS tickets (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_id TEXT NOT NULL REFERENCES customers(id), issue TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open', created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
        INSERT OR IGNORE INTO customers VALUES ('CUST-001','Alex Morgan','alex@example.test'),('CUST-002','Sam Taylor','sam@example.test');
        INSERT OR IGNORE INTO orders VALUES ('ORD-1001','CUST-001','Wireless headphones','delayed',7900),('ORD-1002','CUST-002','Desk lamp','delivered',4500);
        """)


def execute(operation, table, sql, parameters):
    with span(f"db.{operation.lower()}.{table}", attributes={"db.system.name": "sqlite", "db.namespace": "northstar_support", "db.operation.name": operation, "db.collection.name": table}) as current:
        with closing(sqlite3.connect(database_path(), timeout=10)) as db, db:
            db.row_factory = sqlite3.Row
            db.execute("PRAGMA foreign_keys=ON")
            cursor = db.execute(sql, parameters)
            rows = [dict(row) for row in cursor.fetchall()]
            current.set_attribute("db.response.returned_rows", len(rows))
            # Curated business facts make telemetry useful without recording full prompts or SQL.
            for field in ("id", "customer_id", "status", "item"):
                values = tuple(str(row[field]) for row in rows if field in row)
                if values:
                    current.set_attribute(f"support.result.{field}", values)
            return rows
