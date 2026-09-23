"""Shared runtime helpers for deterministic Threadify tools."""

from __future__ import annotations

from typing import Any, Mapping
import json

from harnest import context
from harnest.lib.threadify_graphql import ThreadifyGraphQLClient


async def execute_threadify_query(
    query: str,
    variables: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    """Run one authored query with the authenticated caller's bearer credential."""

    credential = await context.credentials.resolve(
        "threadify-graphql",
        scopes=("graphql:query",),
    )
    client = context.resource("threadify_graphql", ThreadifyGraphQLClient)
    try:
        result = await client.execute(
            query,
            variables,
            authorization=credential.reveal(),
        )
        return runtime_safe_result(result)
    except PermissionError:
        return tool_error("Threadify rejected the caller's credentials.")
    except (RuntimeError, TypeError, ValueError) as error:
        return tool_error(str(error))


def optional_variables(**values: Any) -> dict[str, Any]:
    """Remove absent optional values while preserving meaningful falsey values."""

    return {
        key: value
        for key, value in values.items()
        if value is not None and value != "" and value != []
    }


def bounded_limit(value: int, *, default: int = 50) -> int:
    """Apply the same 1..100 result bound as Threadify MCP tools."""

    if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
        return default
    return min(value, 100)


def bounded_offset(value: int) -> int:
    """Normalize a zero-based page offset without accepting booleans."""

    if not isinstance(value, int) or isinstance(value, bool) or value < 0:
        return 0
    return value


def required_text(value: str, field: str) -> str | None:
    """Normalize a required short tool argument without raising into the runtime."""

    if not isinstance(value, str) or not value.strip():
        return None
    normalized = value.strip()
    if len(normalized) > 500:
        return None
    return normalized


def tool_error(message: str) -> dict[str, Any]:
    """Return an ordinary tool result so the model can correct its arguments."""

    bounded = " ".join(str(message).split())[:500]
    return {"data": None, "errors": [{"message": bounded}]}


def runtime_safe_result(value: dict[str, Any]) -> dict[str, Any]:
    """Keep JSON schemas intact across Harnest 0.20's media discriminator.

    Its asset walker assumes every dictionary's `type` is hashable. JSON Schema
    allows arrays there. Return lossless JSON text only for affected results;
    never change the underlying schema or patch the managed runtime.
    """
    def has_schema_type(item: Any) -> bool:
        if isinstance(item, dict):
            return isinstance(item.get("type"), (list, dict)) or any(has_schema_type(child) for child in item.values())
        return isinstance(item, list) and any(has_schema_type(child) for child in item)

    if has_schema_type(value):
        return {"data_json": json.dumps(value, ensure_ascii=False)}
    return value
