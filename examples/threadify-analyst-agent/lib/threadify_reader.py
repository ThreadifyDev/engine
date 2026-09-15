"""Authored read queries; models never select endpoints or supply credentials."""
import json
import os

import httpx
from harnest import context


LIST_QUERY = """
query SupportRuns($limit: Int!) {
  threadsByRef(refKey: "harnest_agent", refValue: "customer_support_agent", limit: $limit) {
    totalCount
    threads { id label status refs startedAt completedAt error }
  }
}
"""
INSPECT_QUERY = """
query SupportRun($id: ID!) {
  thread(id: $id) {
    id label status refs startedAt completedAt error
    steps {
      stepName status actorService latestContext
      startedAt finishedAt firstSeenAt lastUpdatedAt
    }
  }
}
"""


async def read_credential():
    return await context.credentials.resolve("threadify-local", scopes=("thread:read",))


async def graphql(query, variables, key):
    endpoint = os.environ["THREADIFY_GRAPHQL_URL"]
    async with httpx.AsyncClient(timeout=15, follow_redirects=False) as client:
        response = await client.post(endpoint, headers={"X-API-Key": key.reveal()},
                                     json={"query": query, "variables": variables})
    response.raise_for_status()
    payload = response.json()
    if payload.get("errors") or not isinstance(payload.get("data"), dict):
        raise RuntimeError("Threadify rejected the read query")
    return payload["data"]


def decode_json(value):
    """Threadify JSON scalars and context may arrive already decoded or as text."""
    if not isinstance(value, str):
        return value
    try:
        return json.loads(value)
    except (ValueError, TypeError):
        return value


def normalize_thread(thread):
    result = dict(thread)
    refs = decode_json(result.get("refs"))
    result["refs"] = refs if isinstance(refs, dict) else {}
    if "steps" in result:
        result["steps"] = [
            {**step, "latestContext": decode_json(step.get("latestContext"))}
            for step in result["steps"]
        ]
    return result


async def read_support_runs(limit, key):
    data = await graphql(LIST_QUERY, {"limit": limit}, key)
    connection = data["threadsByRef"]
    return {**connection, "threads": [normalize_thread(thread) for thread in connection["threads"]]}


async def read_support_run(thread_id, key):
    data = await graphql(INSPECT_QUERY, {"id": thread_id}, key)
    thread = normalize_thread(data["thread"]) if data.get("thread") else None
    if not thread or (thread.get("refs") or {}).get("harnest_agent") != "customer_support_agent":
        return None
    return thread
