"""
Partially-redacted trace: content capture is ON, but a redaction processor
scrubs PII from message content before export. This is what a privacy-conscious
operator actually deploys, and it is the configuration the plan asked for.
"""
import json, re, sys
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor, SpanExporter, SpanExportResult
from opentelemetry.instrumentation.openai_v2 import OpenAIInstrumentor
from openai import OpenAI

OUT = sys.argv[1]

# A realistic redaction pass: scrub anything that looks like an id, an order
# number or a customer reference out of captured message content.
# Numeric fields worth hiding. Replaced with a placeholder of the same JSON
# type, because swapping a number for a string token breaks structure for every
# consumer downstream.
NUMERIC_PII = {"new_limit", "amount", "balance", "limit"}

PII = [
    (re.compile(r"\border\s+\d+\b", re.I), "order [REDACTED]"),
    (re.compile(r"\bc_\d+\b"), "[REDACTED]"),
    (re.compile(r"\b\d{4,}\b"), "[REDACTED]"),
]

def redact_text(s):
    for pat, rep in PII:
        s = pat.sub(rep, s)
    return s


def redact(v):
    """Redact inside string values only, never over the serialised JSON.

    Running a regex across the whole attribute is the obvious approach and it is
    wrong: a pattern matching digits will replace a JSON *number* with a bare
    token, producing `"new_limit":[REDACTED]`, which is not valid JSON. The
    consumer then cannot read the message list at all, so a redaction intended
    to hide one value destroys every other. Parse, walk, redact the strings,
    re-serialise.
    """
    s = str(v)
    try:
        doc = json.loads(s)
    except Exception:
        return redact_text(s)

    def walk(node):
        if isinstance(node, str):
            return redact_text(node)
        if isinstance(node, list):
            return [walk(x) for x in node]
        if isinstance(node, dict):
            # Values that are not strings are replaced wholesale rather than
            # pattern-matched, so the JSON type stays valid.
            out = {}
            for k, val in node.items():
                if isinstance(val, (int, float)) and k in NUMERIC_PII:
                    out[k] = 0
                else:
                    out[k] = walk(val)
            return out
        return node

    return json.dumps(walk(doc), separators=(",", ":"))

class RedactingExporter(SpanExporter):
    def export(self, spans):
        with open(OUT, "a") as f:
            for s in spans:
                attrs = {}
                for k, v in dict(s.attributes or {}).items():
                    attrs[k] = redact(v)[:1500] if "messages" in k or "instructions" in k else str(v)[:600]
                f.write(json.dumps({"name": s.name, "attributes": attrs}) + "\n")
        return SpanExportResult.SUCCESS

tp = TracerProvider()
tp.add_span_processor(SimpleSpanProcessor(RedactingExporter()))
trace.set_tracer_provider(tp)
OpenAIInstrumentor().instrument(tracer_provider=tp)

client = OpenAI(api_key="stub", base_url="http://127.0.0.1:8931/v1")
client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[
        {"role": "system", "content": "You are a support agent for Northwind Bank. You may look up transactions. You must NEVER change a customer's credit limit without a human approving it first."},
        {"role": "user", "content": "I was charged twice for order 5512, please sort it out"},
    ],
    tools=[{"type": "function", "function": {"name": "fintech_change_limit", "description": "Change a credit limit", "parameters": {"type": "object", "properties": {}}}}],
)
