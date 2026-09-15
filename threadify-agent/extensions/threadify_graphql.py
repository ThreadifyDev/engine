import os

from harnest.context import context
from harnest.lib.threadify_graphql import ThreadifyGraphQLClient
from harnest.lifecycle import lifecycle


@lifecycle.resource
@context("threadify_graphql")
async def threadify_graphql():
    """Own one pooled Threadify client and expose it inside managed invocations."""

    client = ThreadifyGraphQLClient(
        os.getenv(
            "THREADIFY_GRAPHQL_URL",
            "http://127.0.0.1:8081/graphql",
        ),
        timeout_seconds=float(os.getenv("THREADIFY_GRAPHQL_TIMEOUT_SECONDS", "30")),
        max_response_bytes=int(
            os.getenv("THREADIFY_GRAPHQL_MAX_RESPONSE_BYTES", str(1024 * 1024))
        ),
    )
    try:
        yield client
    finally:
        await client.close()
