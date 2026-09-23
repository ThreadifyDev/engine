"""Verification of Threadify browser JWTs without retaining request secrets."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from functools import lru_cache
import os
from typing import Any, Mapping

import jwt
import httpx
from jwt import PyJWKClient


_MAX_AUTHORIZATION_LENGTH = 8192
_ALGORITHMS = ("RS256", "ES256")


@dataclass(frozen=True, slots=True)
class VerifiedIdentity:
    """Non-secret identity facts extracted from a verified Threadify JWT."""

    user_id: str
    claims: Mapping[str, Any]


@dataclass(frozen=True, slots=True)
class _Verifier:
    jwks: PyJWKClient
    audience: str | None
    issuer: str | None


async def verify_threadify_bearer(authorization: str) -> VerifiedIdentity | None:
    """Verify a bearer header and return only stable, non-secret identity facts."""

    token = _bearer_token(authorization)
    if token is None:
        return None

    mode = os.getenv("THREADIFY_AUTH_MODE", "engine").strip()
    if mode == "engine":
        return await _verify_engine_session(authorization)
    if mode != "jwks":
        raise RuntimeError("THREADIFY_AUTH_MODE must be engine or jwks")

    try:
        claims = await asyncio.to_thread(_decode, token)
    except jwt.PyJWTError:
        return None

    subject = _text(claims.get("sub"))
    if subject is None:
        return None

    metadata = _first_metadata(claims)
    threadify_user_id = _text(metadata.get("threadify_user_id"))
    company_id = _text(metadata.get("threadify_company_id"))
    roles = _roles(claims.get("role"))

    public_claims: dict[str, Any] = {"auth_user_id": subject}
    if threadify_user_id is not None:
        public_claims["threadify_user_id"] = threadify_user_id
    if company_id is not None:
        public_claims["company_id"] = company_id
    if roles:
        public_claims["roles"] = roles

    return VerifiedIdentity(
        user_id=threadify_user_id or subject,
        claims=public_claims,
    )


async def _verify_engine_session(authorization: str) -> VerifiedIdentity | None:
    # The fixed Engine audience verifies opaque sessions, current membership,
    # and revocation on every request. No cookies or browser secrets enter state.
    from harnest.lib.threadify_management import engine_url

    try:
        async with httpx.AsyncClient(timeout=10, follow_redirects=False) as client:
            response = await client.get(
                engine_url() + "/v1/agent/identity",
                headers={"Authorization": authorization, "Accept": "application/json"},
            )
        if response.status_code != 200 or len(response.content) > 16_384:
            return None
        identity = response.json()
        if not isinstance(identity, dict):
            return None
        user = _text(identity.get("user_id"))
        company = _text(identity.get("company_id"))
        if not user or not company:
            return None
        return VerifiedIdentity(
            user_id=f"{company}:{user}",
            claims={"threadify_user_id": user, "company_id": company},
        )
    except (httpx.HTTPError, ValueError):
        return None


def _decode(token: str) -> dict[str, Any]:
    verifier = _verifier()
    signing_key = verifier.jwks.get_signing_key_from_jwt(token)
    options: dict[str, Any] = {
        "require": ["exp", "sub"],
        "verify_aud": verifier.audience is not None,
        "verify_iss": verifier.issuer is not None,
    }
    return jwt.decode(
        token,
        signing_key.key,
        algorithms=list(_ALGORITHMS),
        audience=verifier.audience,
        issuer=verifier.issuer,
        options=options,
    )


@lru_cache(maxsize=1)
def _verifier() -> _Verifier:
    jwks_url = (
        os.getenv("THREADIFY_JWKS_URL", "").strip()
        or os.getenv("JWKS_URL", "").strip()
    )
    if not jwks_url:
        raise RuntimeError("THREADIFY_JWKS_URL or JWKS_URL must be configured")

    audience = os.getenv("THREADIFY_JWKS_AUDIENCE", "authenticated").strip()
    issuer = (
        os.getenv("THREADIFY_JWKS_ISSUER", "").strip()
        or os.getenv("JWKS_ISSUER", "").strip()
    )
    return _Verifier(
        jwks=PyJWKClient(jwks_url),
        audience=audience or None,
        issuer=issuer or None,
    )


def _bearer_token(authorization: str) -> str | None:
    if not isinstance(authorization, str):
        return None
    if not authorization or len(authorization) > _MAX_AUTHORIZATION_LENGTH:
        return None
    parts = authorization.split()
    if len(parts) != 2 or parts[0].casefold() != "bearer":
        return None
    token = parts[1]
    if not token or any(character.isspace() or ord(character) < 32 for character in token):
        return None
    return token


def _first_metadata(claims: Mapping[str, Any]) -> Mapping[str, Any]:
    for key in ("user_metadata", "app_metadata", "metadata"):
        value = claims.get(key)
        if isinstance(value, Mapping):
            return value
    return {}


def _roles(value: Any) -> list[str]:
    if isinstance(value, str) and value.strip():
        return [value.strip()]
    if isinstance(value, (list, tuple)):
        return [item.strip() for item in value if isinstance(item, str) and item.strip()]
    return []


def _text(value: Any) -> str | None:
    if not isinstance(value, str) or not value.strip():
        return None
    return value.strip()
