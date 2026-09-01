"""One shared Harnest store for sessions and private checkpoints."""

import os

from harnest.store import MemoryStore, PostgresStore


_database_url = os.getenv("HARNEST_DATABASE_URL", "").strip()
store = PostgresStore(_database_url) if _database_url else MemoryStore()
