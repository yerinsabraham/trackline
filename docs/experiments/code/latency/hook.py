import json, sys
raw = sys.stdin.read()
try:
    e = json.loads(raw)
except Exception:
    sys.exit(0)
if "secrets.env" in json.dumps(e.get("tool_input", {})):
    sys.stderr.write("BLOCKED: secrets.env is off limits. Use config.local.json instead.")
    sys.exit(2)
sys.exit(0)
