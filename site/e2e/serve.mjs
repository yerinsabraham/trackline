// Serves the static build the way Vercel does: /app/session is app/session.html.
import { createReadStream, existsSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";

const ROOT = join(import.meta.dirname, "..", "out");
const TYPES = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon", ".woff2": "font/woff2", ".webmanifest": "application/manifest+json" };

createServer((req, res) => {
  const path = normalize(decodeURIComponent(new URL(req.url, "http://x").pathname));
  const found = [join(ROOT, path), `${join(ROOT, path)}.html`, join(ROOT, path, "index.html")]
    .find((p) => p.startsWith(ROOT) && existsSync(p) && statSync(p).isFile());
  if (!found) { res.writeHead(404); res.end(); return; }
  res.writeHead(200, { "content-type": TYPES[extname(found)] ?? "application/octet-stream" });
  createReadStream(found).pipe(res);
}).listen(Number(process.env.PORT ?? 4173));
