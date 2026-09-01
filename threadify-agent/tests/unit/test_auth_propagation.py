import asyncio
from types import SimpleNamespace

import harnest.lib.threadify_tooling as tooling


class _Credential:
    def reveal(self):
        return "Bearer same-request-token"


class _Credentials:
    async def resolve(self, audience, scopes):
        assert audience == "threadify-graphql"
        assert scopes == ("graphql:query",)
        return _Credential()


class _Client:
    def __init__(self):
        self.authorization = None

    async def execute(self, query, variables, *, authorization):
        self.authorization = authorization
        return {"data": {"ok": True}}


def test_query_helper_forwards_resolved_request_credential(monkeypatch):
    client = _Client()
    fake_context = SimpleNamespace(
        credentials=_Credentials(),
        resource=lambda name, resource_type: client,
    )
    monkeypatch.setattr(tooling, "context", fake_context)

    result = asyncio.run(
        tooling.execute_threadify_query("query { ok }", {"limit": 1})
    )

    assert result == {"data": {"ok": True}}
    assert client.authorization == "Bearer same-request-token"
