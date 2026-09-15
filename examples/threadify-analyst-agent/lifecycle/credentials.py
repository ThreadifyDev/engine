"""Resolve the operator's key only for the fixed read-only audience."""
import os

from harnest import lifecycle
from harnest.credentials import Credential, CredentialProvider


class ThreadifyCredentials(CredentialProvider):
    async def resolve(self, request):
        if request.audience != "threadify-local":
            return None
        if not set(request.scopes).issubset({"thread:read"}):
            return None
        value = os.getenv("THREADIFY_API_KEY")
        return Credential(value) if value else None


@lifecycle.credential_provider
def credential_provider():
    return ThreadifyCredentials()
