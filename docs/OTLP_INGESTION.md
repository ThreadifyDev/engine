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
| Thread key (or fallback trace ID) | Thread |
| Span | Step |
| Span event | Sub-step |
| Resource/span attributes | Context or refs |
| `UNSET` or `OK` status | Successful step |
| `ERROR` status | Failed step |

Identity is scoped by company and selected in this order: explicit
`threadify.thread_id`, `threadify.thread_key`, `workflow.run_id`, then trace ID.
The application key uses the same resolver as JavaScript `connection.thread(key)`,
Python `await connection.thread(key)`, and Go `connection.Thread(ctx, key)`.
Thread keys are trimmed, nonblank strings of at most 1024 UTF-8 bytes. Set the
same key on every span or on a resource dedicated to that run; several traces
can contribute to one thread. Disable only the workflow fallback with
`/v1/traces?use_workflow_run_id=false`.

Span retries are deduplicated using the company, trace ID, and span ID. Invalid spans
are reported using OTLP partial-success responses; temporary backend failures
return a retryable `503` response.

Shared free-form threads remain open when a root span ends. Complete the run
explicitly with the Boolean attribute `threadify.run.complete = true` or through
the SDK after its work has arrived. Trace-only threads complete on root export;
contracted threads complete according to their contract. Closed threads reject
further writes, including delayed spans, and their keys cannot create replacements.

Resuming loads the stored contract and pinned version; subsequent spans can omit
`threadify.contract`. A conflicting contract or version is rejected. Initialize
contracted runs before telemetry begins: a new key without a contract creates a
free-form thread and cannot acquire a contract later.

## Threadify attributes

Threadify directives can be set as resource or span attributes:

| Attribute | Effect |
| --- | --- |
| `threadify.tags` | Thread tags; string or string array |
| `threadify.contract` | Contract used when creating the Thread |
| `threadify.label` | Thread label |
| `threadify.thread_id` | Attach the trace to an existing writable Thread |
| `threadify.thread_key` | Create or resume using the application session/process key |
| `workflow.run_id` | Shared-key fallback when no explicit thread key is supplied |
| `threadify.run.complete` | Explicitly complete a shared free-form run |
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
not apply to direct OTLP ingestion. Use `threadify.ref.*` attributes for direct
ref mapping.

## Contract input config

Contract version **Input config** maps OTel span names and auto-captured browser
actions to contract steps. Exact mappings win over prefix mappings; the longest
prefix wins, followed by the first matching `regex:` mapping in authored order.
For example, `regex:(?i)^POST /checkout=order_placed` maps case-insensitively to
`order_placed`. Use one regex per line; the final `=` separates its target step.
Regex uses the same Go/RE2 syntax as trace filters and is validated on save.
Mapped spans use normal contract validation and retain their trace
timestamps, context, status, and idempotency key. Explicit `threadify.step_name`
and unmapped spans already named for a contract step still use the normal step path.
Unmapped names use Jev, when configured, to suggest a step; candidates and unmatched
inputs are stored as activity evidence without completing steps. Direct Threadify
SDK events bypass input mappings and continue through normal contract validation.

## Trace ingestion keep list

The Engine's Trace ingestion setting controls OTLP span names for general threads
without contracts. Contract threads bypass this keep list and use the input
mappings for their pinned contract version.
It defaults to `*`, which keeps every span. Set exact names or prefix patterns
such as `checkout.*` to keep only matching spans. An empty list keeps no spans.
Plain matching is case-sensitive and uses the original span name. Excluded spans are
acknowledged so exporters do not retry them. Direct SDK events are unaffected.

The **Span filters** editor uses two sections. Both accept exact span names,
one trailing `*`, or an explicit `regex:` expression, one pattern per line:

```text
[keep spans]
*

[drop spans]
regex:(?i)^POST /graphql
regex:(?i)health|heartbeat
internal.*
```

A span must match **keep spans** and must not match **drop spans**. Drop patterns
win regardless of section order. Empty **keep spans** keeps nothing; empty
**drop spans** excludes nothing further. Unheaded lines at the start are treated
as keep-span patterns; unknown headers are rejected.

Regex uses Go's RE2 syntax. `(?i)` enables case-insensitive matching. Regex
matches anywhere in the name unless anchored with `^` or `$`; plain patterns
remain case-sensitive. Lookarounds and backreferences are unsupported. Invalid
expressions are rejected on save and preview. Compiled matchers are reused
throughout each ingestion batch.

Filtering uses the original OTel span name. A span name that looks like a path
or URL is still matched as a name: `POST /graphql*` matches the method and path
together. URL attributes and the optional `threadify.step_name` display override
do not affect these rules.

**Test your rules** accepts one span name per line and identifies matching drop
patterns. Rules apply to future exports; existing records are unchanged.

The settings API accepts `filters`, optional `exclude`, and `revision`:

```json
{
  "filters": ["*"],
  "exclude": ["regex:(?i)^POST /graphql", "internal.*"],
  "revision": "<current revision>"
}
```

Omitting `exclude` preserves the saved value; an empty array clears it. The
preview API accepts `filters`, `exclude`, and `span_names`.

Previously saved exclusion policies retain their behavior. The editor carries
their span patterns into **drop spans** with `*` in **keep spans**, so saving
preserves the same filtering.

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
