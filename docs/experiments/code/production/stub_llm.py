"""An OpenAI-compatible stub that plays each scripted conversation.

The agent tells it which conversation and which step through headers; the
stub answers with that step's tool call, or a final reply once the script is
done. No model is called and nothing is paid for, but every request goes
through the real OpenAI client and the real instrumentation.
"""
import json, sys, time
from http.server import BaseHTTPRequestHandler, HTTPServer

SCEN = {c["id"]: c for c in json.load(open(sys.argv[1]))["conversations"]}

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        self.rfile.read(int(self.headers.get("content-length", 0)))
        c = SCEN[self.headers["x-scenario"]]
        step = int(self.headers["x-step"])
        time.sleep(c["latency_ms"] / 1000)
        if step < len(c["steps"]):
            s = c["steps"][step]
            msg = {"role": "assistant", "content": None, "tool_calls": [{
                "id": f"call_{c['id']}_{step}", "type": "function",
                "function": {"name": s["tool"], "arguments": json.dumps(s["args"])}}]}
            finish = "tool_calls"
        else:
            msg = {"role": "assistant", "content": "Done. Is there anything else I can help with?"}
            finish = "stop"
        body = json.dumps({"id": f"chatcmpl-{c['id']}-{step}", "object": "chat.completion", "created": 0,
            "model": "gpt-4o-mini", "choices": [{"index": 0, "finish_reason": finish, "message": msg}],
            "usage": {"prompt_tokens": 180 + 40 * step, "completion_tokens": 24, "total_tokens": 204 + 40 * step}}).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass

HTTPServer(("127.0.0.1", 8931), H).serve_forever()
