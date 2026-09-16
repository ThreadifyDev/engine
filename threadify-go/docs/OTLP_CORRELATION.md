# Correlating traces into threads

A thread remains the unit that owns a contract. Multiple OTel traces can contribute steps to that thread without changing its internal UUID.

## Identity selection

The Engine's OTLP/HTTP endpoint and the updated Threadify SDK exporters select identity in this order:

1. `threadify.thread_id`: an existing internal Threadify thread ID (existing explicit-target behavior).
2. `threadify.external_ref`: your reference for one logical run, such as `order-123/payment-attempt-2`.
3. `workflow.run_id`: used by default when no explicit external reference is present.
4. OTel trace ID: one trace maps to one thread.

References are case-sensitive strings, trimmed at the edges, with a maximum of 1024 UTF-8 bytes. Blank references fall through to the next choice. Explicit external references and workflow run IDs use the same identity namespace: the same value selects the same thread. Trace IDs use a separate namespace. All identities are scoped to the licensed company.

Set the reference on every relevant span, or as an OTel resource attribute when the resource belongs to one workflow run. Do not use a category such as `payments` as the reference for unrelated runs. A request-dependent run ID should not be put on a process-wide resource.

The first accepted trace binding is retained. If later telemetry supplies a conflicting reference or contract, ingestion rejects it instead of moving existing steps. Missing attributes cannot retroactively identify an earlier uncorrelated trace. Existing stored threads are not merged by this change.

## Opt out of the workflow fallback

For a standard OTLP/HTTP exporter, configure this traces URL:

```text
http://127.0.0.1:8086/v1/traces?use_workflow_run_id=false
```

The endpoint still requires `X-API-Key` and OTLP Protobuf. The query value must be exactly `true` or `false`; repeated or invalid values are rejected. `false` ignores only `workflow.run_id`, so an explicit `threadify.external_ref` still works.

For the JavaScript SDK exporter:

```js
const exporter = new ThreadifySpanExporter(connection, {
  useWorkflowRunId: false,
});
```

For Python:

```python
exporter = ThreadifySpanExporter(connection, options={"useWorkflowRunId": False})
```

For Go:

```go
useWorkflow := false
exporter := otel.NewSpanExporter(connection, otel.SpanExporterOptions{
    UseWorkflowRunID: &useWorkflow,
})
```

Omit the option to enable workflow correlation. These SDK changes require the matching updated Engine; release them together. The SDK exporters continue to use WebSockets, while standard OTLP exporters use `/v1/traces` over HTTP/Protobuf. Both paths resolve identity in the Engine.

## Concurrency, retries and lifetime

Replicas use a shared Valkey creation lock and correlation mapping. An external reference deterministically selects the internal UUID, allowing lookup of the existing thread after mapping expiry. Storage failures are propagated instead of treating an unavailable database as an absent thread. Existing authorization and contract checks still apply; changing a bound contract or its explicit version is rejected.

Each span retains its original trace ID and span ID in step context. Its idempotency key contains both IDs, so two distinct spans with identical names/content remain distinct, while retries do not append duplicate steps. The shared reference is stored as `threadify.external_ref`; unrelated `threadify.ref.*` attributes remain ordinary searchable references.

A root span ends one trace, which may be only part of a shared run. Shared, contract-free threads therefore remain open until a span explicitly sets the Boolean attribute `threadify.run.complete = true`, or the application completes the thread. Trace-only threads retain automatic root completion. Contract-bound threads retain their contract lifecycle.

## Validation

Coverage includes reference precedence, workflow opt-out, invalid references, conflicting contracts and versions, company scoping, retries, missing attributes on later spans, lookup failure, and concurrent creation. The opt-in compiled-binary test `TestTwoEnginesShareManagedValkey` exercises two replicas, real HTTP/Protobuf exports, the Node SDK over WebSocket, PostgreSQL persistence, correlation mapping expiry, and restart recovery using disposable services.
