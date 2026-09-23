import asyncio
import importlib.util
from pathlib import Path
from types import SimpleNamespace

import httpx
import harnest.lib.identity as identity


def test_workspace_tools_are_discovered_and_client_stubs_are_not_server_actions(tools):
    for name in ("list_contracts", "get_contract", "get_contract_graph", "list_entity_profiles", "list_entity_profile_types"):
        assert name in tools
    for name in ("get_page_context", "navigate_ui", "open_contract_draft", "preview_contract_draft"):
        assert getattr(tools[name], "__harnest_client_tool__", False)


def test_engine_identity_is_verified_and_scoped_by_tenant(monkeypatch):
    seen = []
    original = httpx.AsyncClient

    def handler(request):
        seen.append(request)
        return httpx.Response(200, json={"user_id": "user-1", "company_id": "company-1"})

    monkeypatch.setenv("THREADIFY_AUTH_MODE", "engine")
    monkeypatch.setenv("THREADIFY_ENGINE_URL", "http://engine.test")
    monkeypatch.setattr(identity.httpx, "AsyncClient", lambda **kwargs: original(transport=httpx.MockTransport(handler), **kwargs))
    result = asyncio.run(identity.verify_threadify_bearer("Bearer synthetic-opaque-session"))
    assert result.user_id == "company-1:user-1"
    assert result.claims == {"threadify_user_id": "user-1", "company_id": "company-1"}
    assert "synthetic" not in repr(result)
    assert str(seen[0].url) == "http://engine.test/v1/agent/identity"
    assert seen[0].headers["Authorization"] == "Bearer synthetic-opaque-session"
    assert "cookie" not in seen[0].headers


def test_revoked_engine_session_does_not_fall_back_to_jwks(monkeypatch):
    original = httpx.AsyncClient
    monkeypatch.setenv("THREADIFY_AUTH_MODE", "engine")
    monkeypatch.setattr(identity.httpx, "AsyncClient", lambda **kwargs: original(transport=httpx.MockTransport(lambda request: httpx.Response(401)), **kwargs))
    monkeypatch.setattr(identity, "_decode", lambda token: (_ for _ in ()).throw(AssertionError("must not fall back")))
    assert asyncio.run(identity.verify_threadify_bearer("Bearer revoked-synthetic-token")) is None
    assert asyncio.run(identity.verify_threadify_bearer("not-bearer")) is None


def test_management_credential_scope_cannot_request_writes():
    root = Path(__file__).resolve().parents[2]
    spec = importlib.util.spec_from_file_location("tested_credentials", root / "lifecycle/credentials.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    provider = module.ThreadifyCredentialProvider()
    token = object()
    request = SimpleNamespace(audience="threadify-management", scopes=("contracts:read",), principal=SimpleNamespace(credentials={"threadify_bearer": token}))
    assert asyncio.run(provider.resolve(request)) is token
    request.scopes = ("contracts:write",)
    assert asyncio.run(provider.resolve(request)) is None
    request.audience = "unrelated-api"
    assert asyncio.run(provider.resolve(request)) is None


def test_contract_reads_reject_path_injection_and_invalid_versions(tools):
    for value in ("../users", "https://external.test", "not-a-uuid"):
        assert asyncio.run(tools["get_contract"](value))["errors"]
    assert asyncio.run(tools["get_contract"]("11111111-1111-4111-8111-111111111111", version=-1))["errors"]


def test_json_schema_results_survive_harnest_media_inspection():
    import json
    from harnest.lib.threadify_tooling import runtime_safe_result
    from harnest.transient_media import transient_media_placeholders

    payload = {"data": {"contracts": [{"schema": {"type": ["object", "null"]}}]}}
    result = runtime_safe_result(payload)
    assert json.loads(result["data_json"]) == payload
    assert not transient_media_placeholders(result)
    ordinary = {"data": {"type": "workflow", "items": []}}
    assert runtime_safe_result(ordinary) is ordinary
