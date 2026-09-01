# OTLP Trace Ingestion

Threadify accepts OpenTelemetry traces directly over OTLP/HTTP. Direct ingestion
uses the same thread, step, validation, billing, hash-chain, archival, and
notification paths as the Threadify SDKs, but it does not require a WebSocket
connection or a Threadify-specific span exporter.

## Endpoint

```http
POST /v1/traces
Content-Type: application/x-protobuf
X-API-Key: YOUR_API_KEY
```

The endpoint accepts binary `ExportTraceServiceRequest` protobuf messages,
including requests with `Content-Encoding: gzip`. OTLP JSON, OTLP/gRPC, logs,
metrics, and profiles are not currently supported.

Standard OpenTelemetry environment configuration:

```bash
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=https://your-threadify-host/v1/traces
export OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_TRACES_HEADERS=X-API-Key=YOUR_API_KEY
export OTEL_SERVICE_NAME=checkout-service
```

When `OTEL_EXPORTER_OTLP_ENDPOINT` is used instead, configure the Threadify host
as the base URL and allow the SDK to append `/v1/traces`.

## Mapping

| OpenTelemetry | Threadify |
| --- | --- |
| Trace | Thread |
| Span | Step |
| Span event | Sub-step |
| Resource/span attributes | Context or refs |
| `UNSET` or `OK` status | Successful step |
| `ERROR` status | Failed step |

Trace IDs are correlated to Thread IDs in Valkey and scoped by company. Span
retries are deduplicated using the company, trace ID, and span ID. Invalid spans
are reported using OTLP partial-success responses; temporary backend failures
return a retryable `503` response.

Directly ingested Threads are not closed merely because a root span arrives.
OTLP batches from different services can arrive later and OTLP has no explicit
end-of-trace signal. Thread completion will require an explicit or durable
quiescence policy rather than prematurely rejecting late spans.

## Threadify attributes

Threadify directives can be set as resource or span attributes:

| Attribute | Effect |
| --- | --- |
| `threadify.tags` | Thread tags; string or string array |
| `threadify.contract` | Contract used when creating the Thread |
| `threadify.label` | Thread label |
| `threadify.thread_id` | Attach the trace to an existing writable Thread |
| `threadify.role` | Contract role used for Thread creation |
| `threadify.service` | Override `service.name` for the step actor service |
| `threadify.step_name` | Override the current span's step name |
| `threadify.ref.<key>` | Store the value as Threadify ref `<key>` |
| `threadify.context.<key>` | Store the value as context `<key>` |

Span attributes override resource attributes with the same name. Put immutable
Thread creation attributes—especially tags, contract, role, and label—on the
resource so they are present in every batch. Conflicting immutable directives
within one OTLP request are rejected rather than applied nondeterministically.
After a trace is correlated, later creation directives do not mutate the Thread.

Contract validation follows the normal Threadify rules. Spans in one request
are applied in start-time order, but collectors do not guarantee ordering across
separate OTLP batches; contract workflows should therefore export dependent
spans together or otherwise preserve their delivery order.

`threadify.step_name` is span-scoped. All other non-directive resource and span
attributes are added to step context. Explicit `threadify.context.*` values take
precedence over ordinary attributes that resolve to the same context key.

SDK exporter options such as `filters` and the exporter-level `refs` mapping do
not apply to direct OTLP ingestion. Filter spans in the OpenTelemetry Collector,
and use `threadify.ref.*` attributes for direct ref mapping.

## Collector example

```yaml
receivers:
  otlp:
    protocols:
      grpc: {}
      http: {}

exporters:
  otlphttp/threadify:
    endpoint: https://your-threadify-host
    headers:
      X-API-Key: ${env:THREADIFY_API_KEY}

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlphttp/threadify]
```
