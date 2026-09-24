"""Saves each OTLP/HTTP export request exactly as the exporter sent it."""
import os, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

OUT = sys.argv[1]
os.makedirs(OUT, exist_ok=True)

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("content-length", 0)))
        n = len(os.listdir(OUT))
        ext = "json" if "json" in self.headers.get("content-type", "") else "pb"
        with open(os.path.join(OUT, f"{n:03d}.{ext}"), "wb") as f:
            f.write(body)
        self.send_response(200)
        self.send_header("content-type", self.headers.get("content-type", "application/x-protobuf"))
        self.send_header("content-length", "0")
        self.end_headers()
    def log_message(self, *a): pass

HTTPServer(("127.0.0.1", 4319), H).serve_forever()
