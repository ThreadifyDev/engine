from harnest import CredentialProvider
from harnest.lifecycle import lifecycle


_AUDIENCE = "threadify-graphql"
_ALLOWED_SCOPES = frozenset({"graphql:query"})


class ThreadifyCredentialProvider(CredentialProvider):
    """Forward the verified browser credential only to Threadify GraphQL."""

    async def resolve(self, request):
        if request.audience != _AUDIENCE:
            return None
        if not set(request.scopes).issubset(_ALLOWED_SCOPES):
            return None
        return request.principal.credentials.get("threadify_bearer")


@lifecycle.credential_provider
def credential_provider():
    return ThreadifyCredentialProvider()
