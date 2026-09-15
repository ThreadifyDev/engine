from harnest import Credential
from harnest.lib.identity import verify_threadify_bearer
from harnest.lifecycle import lifecycle
from harnest.runtime_auth import AuthPrincipal, AuthenticationError


@lifecycle.authenticate
async def authenticate(connection, principal):
    """Authenticate a Threadify bearer token and retain it only as a credential."""

    if principal is not None:
        return None

    authorization = connection.headers.get("authorization", "")
    identity = await verify_threadify_bearer(authorization)
    if identity is None:
        raise AuthenticationError("A valid Threadify bearer token is required")

    return AuthPrincipal(
        user_id=identity.user_id,
        claims=identity.claims,
        credentials={"threadify_bearer": Credential(authorization)},
    )
