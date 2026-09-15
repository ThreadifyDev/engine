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
