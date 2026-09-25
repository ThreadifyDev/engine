"""Read a bounded, public Threadify developer guide from a fixed source list."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any
from urllib.error import URLError
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener

from harnest.agent import tool


_DOCS = "https://docs.threadify.dev/"
_SDK_README = "https://raw.githubusercontent.com/ThreadifyDev/"
_SOURCES = {
    "javascript": {
        "install": _DOCS + "core-concepts/installation/javascript.md",
        "quickstart": _DOCS + "quickstart.md",
        "connect": _DOCS + "core-concepts/connecting.md",
        "workflows": _DOCS + "core-concepts/tracking-workflows.md",
        "contracts": _DOCS + "core-concepts/working-with-contracts.md",
        "waits": _DOCS + "core-concepts/execution-waits.md",
        "otel": _DOCS + "opentelemetry.md",
        "api": _SDK_README + "node-sdk/main/README.md",
    },
    "python": {
        "install": _DOCS + "core-concepts/installation/python.md",
        "quickstart": _DOCS + "quickstart.md",
        "connect": _DOCS + "core-concepts/connecting.md",
        "workflows": _DOCS + "core-concepts/tracking-workflows.md",
        "contracts": _DOCS + "core-concepts/working-with-contracts.md",
        "waits": _DOCS + "core-concepts/installation/python.md",
        "otel": _DOCS + "opentelemetry.md",
        "api": _SDK_README + "python-sdk/main/README.md",
    },
    "go": {
        "install": _DOCS + "core-concepts/installation/go.md",
        "quickstart": _DOCS + "quickstart.md",
        "connect": _DOCS + "core-concepts/connecting.md",
        "workflows": _DOCS + "core-concepts/tracking-workflows.md",
        "contracts": _DOCS + "core-concepts/working-with-contracts.md",
        "waits": _DOCS + "core-concepts/installation/go.md",
        "otel": _DOCS + "opentelemetry.md",
        "api": _SDK_README + "go-sdk/main/README.md",
    },
    "cli": {
        "commands": _DOCS + "cli.md",
    },
}
_MAX_BYTES = 64 * 1024


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, msg, headers, newurl):
        return None


def _read_public_guide(url: str) -> str:
    # No ambient proxy, cookies, bearer credentials, or redirect to another host.
    opener = build_opener(ProxyHandler({}), _NoRedirect())
    request = Request(url, headers={"Accept": "text/plain", "User-Agent": "Threadify-Agent/1"})
    with opener.open(request, timeout=6) as response:
        body = response.read(_MAX_BYTES + 1)
    if len(body) > _MAX_BYTES:
        raise ValueError("reference exceeds size limit")
    return body.decode("utf-8")


@tool
async def get_developer_reference(kind: str, topic: str) -> dict[str, Any]:
    """Get current public SDK or CLI documentation to ground code examples.

    Args:
        kind: One of javascript, python, go, cli.
        topic: For SDKs: install, quickstart, connect, workflows, contracts,
            waits, otel, api. For CLI: commands.
    """
    url = _SOURCES.get(kind, {}).get(topic)
    if url is None:
        return {"status": "invalid_reference", "valid": {k: list(v) for k, v in _SOURCES.items()}}
    try:
        # This tool reads only fixed public files and never sends caller credentials.
        from asyncio import to_thread

        content = await to_thread(_read_public_guide, url)
    except (OSError, URLError, UnicodeError, ValueError) as exc:
        return {"status": "unavailable", "source": url, "reason": type(exc).__name__}
    return {
        "status": "ok",
        "source": url,
        "fetched_at": datetime.now(timezone.utc).isoformat(),
        "content": content,
    }
