import pytest


@pytest.fixture(autouse=True)
def isolated_sdk_connection(monkeypatch):
    """Auth smoke tests do not subscribe to a live Engine websocket."""
    from harnest.extensions.threadify import ThreadifyExtension

    async def noop(*args, **kwargs):
        pass

    monkeypatch.setattr(ThreadifyExtension, "start", noop)
    monkeypatch.setattr(ThreadifyExtension, "stop", noop)


def test_health_is_public(client):
    response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_responses_require_a_bearer_token(client):
    response = client.post("/responses", json={"input": "Hello"})
    assert response.status_code == 401
    assert response.json() == {
        "detail": "A valid Threadify bearer token is required"
    }


def test_engine_session_scopes_harnest_history_and_honors_revocation(client, monkeypatch):
    import httpx
    import harnest.lib.identity as identity

    original = httpx.AsyncClient
    revoked = False
    headers = []

    def verify(request):
        headers.append(request.headers.get("authorization"))
        if revoked:
            return httpx.Response(401)
        company = "company-b" if request.headers["authorization"].endswith("other") else "company-a"
        return httpx.Response(200, json={"user_id": "same-user", "company_id": company})

    monkeypatch.setenv("THREADIFY_AUTH_MODE", "engine")
    monkeypatch.setattr(identity.httpx, "AsyncClient", lambda **kwargs: original(transport=httpx.MockTransport(verify), **kwargs))
    auth = {"Authorization": "Bearer synthetic-session"}
    response = client.post("/sessions", headers=auth, json={"state": {"title": "Authentication smoke"}})
    assert response.status_code in (200, 201), response.text
    session_id = response.json()["id"]
    assert "synthetic-session" not in response.text
    assert client.get(f"/sessions/{session_id}", headers=auth).status_code == 200
    assert client.get(f"/sessions/{session_id}", headers={"Authorization": "Bearer synthetic-other"}).status_code == 404
    revoked = True
    assert client.get(f"/sessions/{session_id}", headers=auth).status_code == 401
    assert headers[0] == "Bearer synthetic-session"
