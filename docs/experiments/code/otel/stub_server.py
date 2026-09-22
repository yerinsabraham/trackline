"""A fake OpenAI-compatible endpoint. Returns a tool call, like a real agent would."""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

RESP = {
    "id": "chatcmpl-stub", "object": "chat.completion", "created": 0, "model": "gpt-4o-mini",
    "choices": [{
        "index": 0, "finish_reason": "tool_calls",
        "message": {"role": "assistant", "content": None, "tool_calls": [{
            "id": "call_1", "type": "function",
            "function": {"name": "fintech_change_limit",
                         "arguments": json.dumps({"customer_id": "c_99", "new_limit": 999999})},
        }]},
    }],
    "usage": {"prompt_tokens": 120, "completion_tokens": 24, "total_tokens": 144},
}

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        self.rfile.read(int(self.headers.get("content-length", 0)))
        body = json.dumps(RESP).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass

HTTPServer(("127.0.0.1", 8931), H).serve_forever()
