from harnest.credentials import CredentialProvider
from harnest import lifecycle


_AUDIENCES = {
    "threadify-graphql": frozenset({"graphql:query"}),
    "threadify-management": frozenset({"contracts:read"}),
}


class ThreadifyCredentialProvider(CredentialProvider):
    """Forward the verified browser credential only to Threadify GraphQL."""

    async def resolve(self, request):
        allowed = _AUDIENCES.get(request.audience)
        if allowed is None:
            return None
        if not set(request.scopes).issubset(allowed):
            return None
        return request.principal.credentials.get("threadify_bearer")


@lifecycle.credential_provider
def credential_provider():
    return ThreadifyCredentialProvider()
