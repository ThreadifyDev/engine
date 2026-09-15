"""Bounded, read-only client for the Threadify GraphQL endpoint."""

from __future__ import annotations

import json
from typing import Any, Mapping
from urllib.parse import urlsplit

from graphql import OperationType, parse
from graphql.language.ast import OperationDefinitionNode
import httpx


class ThreadifyGraphQLClient:
    """Execute authenticated queries without logging credentials or payloads."""

    def __init__(
        self,
        endpoint: str,
        *,
        timeout_seconds: float = 30.0,
        max_response_bytes: int = 1024 * 1024,
    ) -> None:
        self._endpoint = _endpoint(endpoint)
        if timeout_seconds <= 0:
            raise ValueError("GraphQL timeout must be positive")
        if max_response_bytes < 1024:
            raise ValueError("GraphQL response limit must be at least 1024 bytes")
        self._max_response_bytes = max_response_bytes
        self._client = httpx.AsyncClient(
            timeout=httpx.Timeout(timeout_seconds),
            follow_redirects=False,
            headers={"User-Agent": "threadify-harnest-agent/0.1.0"},
        )

    async def close(self) -> None:
        await self._client.aclose()

    async def execute(
        self,
        query: str,
        variables: Mapping[str, Any] | None,
        *,
        authorization: str,
    ) -> dict[str, Any]:
        validated_query = _read_only_query(query)
        validated_variables = _variables(variables)
        payload = {"query": validated_query, "variables": validated_variables}

        async with self._client.stream(
            "POST",
            self._endpoint,
            json=payload,
            headers={
                "Accept": "application/json",
                "Authorization": authorization,
            },
        ) as response:
            body = bytearray()
            async for chunk in response.aiter_bytes():
                body.extend(chunk)
                if len(body) > self._max_response_bytes:
                    raise RuntimeError("Threadify GraphQL response exceeded the configured limit")

        if response.status_code in {401, 403}:
            raise PermissionError("Threadify rejected the caller's credentials")
        if response.status_code == 402:
            raise RuntimeError("Threadify reported insufficient credits")
        if response.status_code == 422:
            return _graphql_rejection(body)
        if response.status_code < 200 or response.status_code >= 300:
            raise RuntimeError(
                f"Threadify GraphQL request failed with HTTP {response.status_code}"
            )

        return _response_envelope(body)


def _response_envelope(body: bytes | bytearray) -> dict[str, Any]:
    try:
        result = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise RuntimeError("Threadify GraphQL returned invalid JSON") from error
    if not isinstance(result, dict):
        raise RuntimeError("Threadify GraphQL returned an invalid response envelope")
    return result


def _graphql_rejection(body: bytes | bytearray) -> dict[str, Any]:
    """Return bounded validation feedback so the agent can correct its query."""

    try:
        result = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        result = None

    errors: list[dict[str, str]] = []
    if isinstance(result, dict) and isinstance(result.get("errors"), list):
        for item in result["errors"][:5]:
            if not isinstance(item, Mapping):
                continue
            message = item.get("message")
            if not isinstance(message, str) or not message.strip():
                continue
            bounded = " ".join(message.split())[:500]
            errors.append({"message": bounded})

    if not errors:
        errors.append(
            {
                "message": (
                    "Threadify rejected the GraphQL document. Check the loaded "
                    "schema reference and retry with supported fields and arguments."
                )
            }
        )
    return {"data": None, "errors": errors}


def _read_only_query(value: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError("GraphQL query must be a non-empty string")
    query = value.strip()
    if len(query) > 20_000:
        raise ValueError("GraphQL query exceeds the 20000 character limit")
    try:
        document = parse(query)
    except Exception as error:
        raise ValueError("GraphQL query is not syntactically valid") from error

    operations = [
        definition
        for definition in document.definitions
        if isinstance(definition, OperationDefinitionNode)
    ]
    if not operations:
        raise ValueError("GraphQL document must contain a query operation")
    if any(operation.operation is not OperationType.QUERY for operation in operations):
        raise ValueError("Only read-only GraphQL query operations are permitted")
    return query


def _variables(value: Mapping[str, Any] | None) -> dict[str, Any]:
    if value is None:
        return {}
    if not isinstance(value, Mapping):
        raise TypeError("GraphQL variables must be an object")
    normalized = dict(value)
    try:
        encoded = json.dumps(normalized, allow_nan=False, separators=(",", ":"))
    except (TypeError, ValueError) as error:
        raise ValueError("GraphQL variables must be JSON serializable") from error
    if len(encoded.encode("utf-8")) > 100_000:
        raise ValueError("GraphQL variables exceed the 100000 byte limit")
    return normalized


def _endpoint(value: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError("THREADIFY_GRAPHQL_URL must be configured")
    endpoint = value.strip()
    parsed = urlsplit(endpoint)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("THREADIFY_GRAPHQL_URL must be an absolute HTTP(S) URL")
    if parsed.username is not None or parsed.password is not None or parsed.fragment:
        raise ValueError("THREADIFY_GRAPHQL_URL cannot contain credentials or a fragment")
    return endpoint
