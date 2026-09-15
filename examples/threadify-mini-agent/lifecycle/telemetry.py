"""Observe the support runtime through OTLP; never expose Threadify as a tool."""
import os
from harnest import lifecycle
from harnest.telemetry import TelemetryExporter
from opentelemetry.sdk.trace import ReadableSpan
from opentelemetry.sdk.trace.export import SpanExporter, SpanExportResult


class SupportSpanExporter(SpanExporter):
    def __init__(self, delegate):
        self.delegate = delegate

    def export(self, spans):
        tagged = []
        for span in spans:
            attributes = dict(span.attributes or {})
            # Keep agent/model/tool and business database spans, not playground HTTP traffic.
            if not (any(key.startswith("gen_ai.") for key in attributes) or "db.system.name" in attributes or "threadify.step_name" in attributes):
                continue
            if attributes.get("gen_ai.operation.name") == "invoke_agent" and attributes.get("gen_ai.agent.name") == "customer_support_agent":
                attributes["threadify.run.complete"] = True
            attributes["threadify.ref.harnest_agent"] = "customer_support_agent"
            attributes["threadify.label"] = "Northstar customer support"
            attributes["threadify.tags"] = ("harnest", "customer-support", "otel")
            attributes["threadify.step_name"] = attributes.get("gen_ai.tool.name") or span.name
            session = attributes.get("gen_ai.conversation.id") or attributes.get("threadify.ref.harnest_session_id")
            if session:
                attributes["threadify.ref.harnest_session_id"] = str(session)
            tagged.append(ReadableSpan(
                name=span.name, context=span.context, parent=span.parent,
                resource=span.resource, attributes=attributes, events=span.events,
                links=span.links, kind=span.kind, status=span.status,
                start_time=span.start_time, end_time=span.end_time,
                instrumentation_scope=span.instrumentation_scope,
            ))
        return self.delegate.export(tagged) if tagged else SpanExportResult.SUCCESS

    def shutdown(self):
        self.delegate.shutdown()

    def force_flush(self, timeout_millis=30000):
        return self.delegate.force_flush(timeout_millis)


@lifecycle.telemetry_exporter
def support_telemetry():
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    return TelemetryExporter(name="threadify-support-otel", traces=SupportSpanExporter(
        OTLPSpanExporter(endpoint=os.environ["THREADIFY_OTLP_ENDPOINT"],
                         headers={"X-API-Key": os.environ["THREADIFY_OTLP_API_KEY"]}, timeout=10)
    ))


@lifecycle.tool.before
def correlate_tool(context, request):
    from opentelemetry import trace
    current = trace.get_current_span()
    current.set_attribute("threadify.step_name", request.name)
    if context.session_id:
        current.set_attribute("threadify.ref.harnest_session_id", str(context.session_id))
    return context.next()
