"""A small production support agent, instrumented the way a real one is.

Model calls go through the official OpenAI instrumentation. Each conversation
is an invoke_agent span and each tool run an execute_tool span, per the GenAI
semantic conventions, which is what agent frameworks emit. Spans leave through
the standard OTLP/HTTP exporter in its default encoding, protobuf.

    python agent.py scenarios.json http://127.0.0.1:4319/v1/traces
"""
import json, os, sys
os.environ.setdefault("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "SPAN_AND_EVENT")
os.environ.setdefault("OTEL_SEMCONV_STABILITY_OPT_IN", "gen_ai_latest_experimental")

from opentelemetry import trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.openai_v2 import OpenAIInstrumentor
from opentelemetry.trace import Status, StatusCode
from openai import OpenAI

spec = json.load(open(sys.argv[1]))
tp = TracerProvider(resource=Resource.create({"service.name": "northwind-support"}))
tp.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=sys.argv[2])))
trace.set_tracer_provider(tp)
OpenAIInstrumentor().instrument(tracer_provider=tp)
tracer = trace.get_tracer("northwind.agent")
client = OpenAI(api_key="stub", base_url="http://127.0.0.1:8931/v1")

TOOLS = [{"type": "function", "function": {"name": n, "description": d, "parameters": {"type": "object"}}}
         for n, d in [("fintech_transaction_status", "Look up a transaction"), ("get_balance", "Read a balance"),
                      ("update_address", "Change a customer's address"), ("request_human_approval", "Ask a human to approve an action"),
                      ("fintech_change_limit", "Change a credit limit"), ("send_marketing_email", "Send a marketing email")]]

for c in spec["conversations"]:
    with tracer.start_as_current_span("invoke_agent northwind-support") as root:
        root.set_attribute("gen_ai.operation.name", "invoke_agent")
        root.set_attribute("gen_ai.agent.name", "northwind-support")
        root.set_attribute("gen_ai.conversation.id", c["id"])
        messages = [{"role": "system", "content": spec["system"]}, {"role": "user", "content": c["user"]}]
        for step in range(len(c["steps"]) + 1):
            r = client.chat.completions.create(model="gpt-4o-mini", messages=messages, tools=TOOLS,
                                               extra_headers={"x-scenario": c["id"], "x-step": str(step)})
            m = r.choices[0].message
            if not m.tool_calls:
                break
            messages.append({"role": "assistant", "content": None, "tool_calls": [tc.model_dump() for tc in m.tool_calls]})
            for tc in m.tool_calls:
                s = c["steps"][step]
                with tracer.start_as_current_span(f"execute_tool {tc.function.name}") as span:
                    span.set_attribute("gen_ai.operation.name", "execute_tool")
                    span.set_attribute("gen_ai.tool.name", tc.function.name)
                    span.set_attribute("gen_ai.tool.call.id", tc.id)
                    span.set_attribute("gen_ai.tool.call.arguments", tc.function.arguments)
                    if s["result"] == "error":
                        span.set_status(Status(StatusCode.ERROR, "upstream timeout"))
                        span.set_attribute("error.type", "TimeoutError")
                        result = {"error": "upstream timeout"}
                    else:
                        result = {"ok": True}
                messages.append({"role": "tool", "tool_call_id": tc.id, "content": json.dumps(result)})
tp.shutdown()
