"""Fixed-audience, bounded contract reads using the caller's Engine credential."""
import os
from typing import Any
from urllib.parse import urlsplit

import httpx
from harnest import context
from harnest.lib.threadify_tooling import tool_error, runtime_safe_result


def engine_url() -> str:
    value = os.getenv("THREADIFY_ENGINE_URL", "http://127.0.0.1:8081").rstrip("/")
    parsed = urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc or parsed.path or parsed.query or parsed.fragment or parsed.username is not None or parsed.password is not None:
        raise ValueError("THREADIFY_ENGINE_URL must be an HTTP(S) origin without credentials")
    return value


async def read_contracts(path: str, params: dict[str, Any]) -> dict[str, Any]:
    credential = await context.credentials.resolve("threadify-management", scopes=("contracts:read",))
    try:
        async with httpx.AsyncClient(timeout=30, follow_redirects=False) as client:
            async with client.stream("GET", engine_url() + path, params=params,
                headers={"Authorization": credential.reveal(), "Accept": "application/json"}) as response:
                body = bytearray()
                async for chunk in response.aiter_bytes():
                    body.extend(chunk)
                    if len(body) > 1024 * 1024:
                        return tool_error("Contract response exceeds the 1 MiB limit; narrow the request.")
                if response.status_code != 200:
                    return tool_error(f"Threadify contract read failed with HTTP {response.status_code}.")
                import json
                return runtime_safe_result({"data": json.loads(body)})
    except (httpx.HTTPError, ValueError):
        return tool_error("Threadify contract data is unavailable.")
