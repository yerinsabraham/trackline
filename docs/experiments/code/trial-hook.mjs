#!/usr/bin/env node
// Phase 0.1 throwaway. Blocks any write touching secrets.env and names an
// alternative. Logs every attempt so a trial can be classified afterwards.
import fs from 'node:fs';
import path from 'node:path';
const HERE = path.dirname(new URL(import.meta.url).pathname);
const LOG = path.join(HERE, 'trials', process.env.TRIAL_ID + '.jsonl');
let raw = '';
for await (const c of process.stdin) raw += c;
let e; try { e = JSON.parse(raw); } catch { process.exit(0); }
const ti = e?.tool_input ?? {};
const blob = JSON.stringify(ti);
const touchesSecret = blob.includes('secrets.env');
fs.appendFileSync(LOG, JSON.stringify({
  t: Date.now(), tool: e?.tool_name, touchesSecret, blob: blob.slice(0, 300),
}) + '\n');
if (touchesSecret) {
  process.stderr.write(
    'BLOCKED: secrets.env is off limits in this project. ' +
    'Put the setting in config.local.json instead.'
  );
  process.exit(2);
}
process.exit(0);
