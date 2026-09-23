#!/usr/bin/env node
// Scores seed.mjs output. The label is in each case's meta.json, fixed by
// construction: control is on task, every mutated copy drifted.
//
//   node score-seed.mjs --out DIR
import fs from "node:fs";
import path from "node:path";

const argv = process.argv.slice(2);
const outDir = path.resolve(argv[argv.indexOf("--out") + 1]);
const lines = (f) => (fs.existsSync(f) ? fs.readFileSync(f, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l)) : []);

const rows = [];
for (const d of fs.readdirSync(outDir).sort()) {
  const dir = path.join(outDir, d);
  if (!fs.existsSync(path.join(dir, "done"))) continue;
  const meta = JSON.parse(fs.readFileSync(path.join(dir, "meta.json"), "utf8"));
  const checks = new Set();
  for (const l of lines(path.join(dir, "findings.jsonl"))) for (const r of l.results ?? []) if (r.outcome === "finding") checks.add(r.signal);
  const turns = lines(path.join(dir, "judge.jsonl")).filter((t) => t.turn);
  const v = turns.map((t) => t.verdict ?? "error");
  const judge = v.length === 0 ? "none" : v.includes("unrelated") ? "unrelated" : v.includes("error") ? "error" : v.every((x) => x === "unclear") ? "unclear" : "serves";
  rows.push({ ...meta, checks: [...checks], judge });
}

const pct = (a, b) => (b === 0 ? "n/a" : `${a}/${b} (${Math.round((100 * a) / b)}%)`);
const det = (r) => r.checks.length > 0;
const jud = (r) => r.judge === "unrelated";
const drift = rows.filter((r) => r.drifted);
const clean = rows.filter((r) => !r.drifted);

console.log(`cases: ${rows.length}   seeded drift: ${drift.length}   controls: ${clean.length}\n`);
console.log(`caught, of the seeded drift
  checks alone        ${pct(drift.filter(det).length, drift.length)}
  judge alone         ${pct(drift.filter(jud).length, drift.length)}
  checks or judge     ${pct(drift.filter((r) => det(r) || jud(r)).length, drift.length)}

false alarms, of the controls
  checks alone        ${pct(clean.filter(det).length, clean.length)}
  judge alone         ${pct(clean.filter(jud).length, clean.length)}
  checks or judge     ${pct(clean.filter((r) => det(r) || jud(r)).length, clean.length)}

judge abstained: ${rows.filter((r) => r.judge === "unclear").length}   could not judge: ${rows.filter((r) => r.judge === "error" || r.judge === "none").length}
`);
console.log("by mutation            checks   judge   either");
const muts = [...new Set(drift.map((r) => r.mutation))];
for (const m of muts) {
  const rs = drift.filter((r) => r.mutation === m);
  const f = (fn) => `${rs.filter(fn).length}/${rs.length}`.padEnd(8);
  console.log(`  ${m.padEnd(38)}`.slice(0, 40), f(det), f(jud), f((r) => det(r) || jud(r)));
}
