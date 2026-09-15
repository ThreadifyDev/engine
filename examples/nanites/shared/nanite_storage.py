"""Replica-safe session and checkpoint storage shared by all Nanites."""

from __future__ import annotations

import os
from functools import lru_cache

from harnest.lifecycle import lifecycle
from harnest.store import PostgresStore


def _dsn() -> str:
    """Require the private connection string without placing it in diagnostics."""

    value = os.getenv("HARNEST_NANITES_POSTGRES_URL")
    if not value:
        raise RuntimeError("HARNEST_NANITES_POSTGRES_URL is required")
    return value


@lru_cache(maxsize=1)
def _store() -> PostgresStore:
    """Create one application pool when runtime storage is first resolved."""

    # Deferring construction keeps ordinary imports side-effect free, while
    # caching preserves one pool for both storage responsibilities.
    return PostgresStore(_dsn())


@lifecycle.storage.sessions
def sessions() -> PostgresStore:
    """Provide durable sessions which any replica of this Nanite can resume."""

    return _store()


@lifecycle.storage.checkpoints
def checkpoints() -> PostgresStore:
    """Reuse the session pool for framework-native durable checkpoints."""

    return _store()
