import fs from 'node:fs';
const raw = fs.readFileSync(0, 'utf8');
let e; try { e = JSON.parse(raw); } catch { process.exit(0); }
if (JSON.stringify(e.tool_input ?? {}).includes('secrets.env')) {
  process.stderr.write('BLOCKED: secrets.env is off limits. Use config.local.json instead.');
  process.exit(2);
}
process.exit(0);
