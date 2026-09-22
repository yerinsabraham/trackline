"""One realistic production-agent call, instrumented by the official OTel GenAI package."""
import json, os, sys
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor, SpanExporter, SpanExportResult
from opentelemetry.instrumentation.openai_v2 import OpenAIInstrumentor
from openai import OpenAI

OUT = sys.argv[1]

class FileExporter(SpanExporter):
    def export(self, spans):
        with open(OUT, "a") as f:
            for s in spans:
                f.write(json.dumps({
                    "name": s.name,
                    "attributes": {k: str(v)[:600] for k, v in dict(s.attributes or {}).items()},
                    "events": [{"name": e.name, "attributes": {k: str(v)[:600] for k, v in dict(e.attributes or {}).items()}} for e in s.events],
                }) + "\n")
        return SpanExportResult.SUCCESS

tp = TracerProvider()
tp.add_span_processor(SimpleSpanProcessor(FileExporter()))
trace.set_tracer_provider(tp)
OpenAIInstrumentor().instrument(tracer_provider=tp)

client = OpenAI(api_key="stub", base_url="http://127.0.0.1:8931/v1")
client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[
        {"role": "system", "content": "You are a support agent for Northwind Bank. You may look up transactions. You must NEVER change a customer's credit limit without a human approving it first."},
        {"role": "user", "content": "I was charged twice for order 5512, please sort it out"},
    ],
    tools=[
        {"type": "function", "function": {"name": "fintech_transaction_status", "description": "Look up a transaction", "parameters": {"type": "object", "properties": {"id": {"type": "string"}}}}},
        {"type": "function", "function": {"name": "fintech_change_limit", "description": "Change a credit limit", "parameters": {"type": "object", "properties": {"customer_id": {"type": "string"}, "new_limit": {"type": "integer"}}}}},
    ],
)
