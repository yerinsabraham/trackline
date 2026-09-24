import subprocess, time, os, statistics, json, sys
# Usage: python3 bench.py DIR, where DIR holds old-hook, bin/trackline-hook,
# bin/trackline, a connected project at shop/ and its account state at cfg/.
S=sys.argv[1]
proj=f"{S}/shop"
payload=json.dumps({"hook_event_name":"PreToolUse","session_id":"bench","cwd":proj,"tool_use_id":"b","tool_name":"Write","tool_input":{"file_path":f"{proj}/src/bench.ts","content":"x"}}).encode()
empty=f"{S}/cfg-empty"; os.makedirs(empty, exist_ok=True)
conds={
 "before C4":(f"{S}/old-hook", empty),
 "C4, not connected":(f"{S}/bin/trackline-hook", empty),
 "C4, connected":(f"{S}/bin/trackline-hook", f"{S}/cfg"),
}
res={k:[] for k in conds}
def run(k):
  b,cfg=conds[k]; env=dict(os.environ, TRACKLINE_CONFIG_DIR=cfg, TRACKLINE_API="http://127.0.0.1:4100/trackline/v1")
  t=time.perf_counter(); subprocess.run([b,"-host","claude"],input=payload,env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); return (time.perf_counter()-t)*1000
for k in conds:
  for _ in range(10): run(k)
for i in range(200):
  for k in conds: res[k].append(run(k))
for k,v in res.items():
  v.sort(); print(f"{k:20s} median {statistics.median(v):5.2f}ms  p95 {v[int(len(v)*0.95)]:5.2f}ms")
