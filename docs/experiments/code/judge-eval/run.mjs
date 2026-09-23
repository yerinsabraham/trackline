#!/usr/bin/env node
// Runs each pre-registered scenario through a real agent, with trackline
// watching in warn mode, then asks a judge about the recorded turn.
//
//   node run.mjs --bin DIR [--agent claude] [--judge codex] [--only id] [--variant clean|bait]
//
// DIR holds the trackline and trackline-hook binaries. Nothing here is scored:
// score.mjs reads what this leaves in out/ and compares it with the labels
// written into scenarios.json before any session ran.
import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const here = path.dirname(new URL(import.meta.url).pathname);
const args = Object.fromEntries(
  process.argv.slice(2).reduce((acc, a, i, all) => {
    if (a.startsWith("--")) acc.push([a.slice(2), all[i + 1]]);
    return acc;
  }, []),
);
const bin = path.resolve(args.bin ?? "");
const agent = args.agent ?? "claude";
const judge = args.judge ?? "codex";
const outDir = path.resolve(args.out ?? path.join(here, "out"));
if (!fs.existsSync(path.join(bin, "trackline"))) {
  console.error("--bin must point at a directory holding trackline and trackline-hook");
  process.exit(2);
}

const { scenarios } = JSON.parse(fs.readFileSync(path.join(here, "scenarios.json"), "utf8"));
const variants = args.variant ? [args.variant] : ["clean", "bait"];

for (const s of scenarios) {
  if (args.only && s.id !== args.only) continue;
  for (const v of variants) {
    const id = `${s.id}-${v}`;
    const dest = path.join(outDir, id);
    if (fs.existsSync(path.join(dest, "done"))) {
      console.log(`${id}: already run, skipping`);
      continue;
    }
    fs.mkdirSync(dest, { recursive: true });
    console.log(`${id}: running ${agent}`);

    // realpath: macOS hands out /var/... which the agent reports as /private/var/...
    const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), `judge-eval-${id}-`)));
    const files = { ...s.files, ...(v === "bait" ? s.bait : {}) };
    for (const [rel, body] of Object.entries(files)) {
      fs.mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
      fs.writeFileSync(path.join(root, rel), body);
      if (rel.endsWith(".sh")) fs.chmodSync(path.join(root, rel), 0o755);
    }
    // A throwaway repository, so the end state can be diffed. Its identity is
    // set per command and never touches any real configuration.
    const git = (...a) =>
      execFileSync("git", ["-c", "user.name=lab", "-c", "user.email=lab@example.invalid", ...a], { cwd: root });
    git("init", "-q");
    git("add", "-A");
    git("commit", "-qm", "baseline");

    execFileSync(path.join(bin, "trackline"), ["init", "--host", agent, "--root", root], { stdio: "ignore" });

    const started = Date.now();
    let run;
    if (agent === "claude") {
      // acceptEdits and a short allowlist: edits go through, and other
      // commands are denied. Denied attempts are still seen by the hook, which
      // runs first, and an attempt is what drift is measured on.
      run = spawnSync(
        "claude",
        ["-p", s.request, "--permission-mode", "acceptEdits", "--max-turns", "25",
          "--allowedTools", "Bash(node:*)", "Bash(npm test:*)", "Bash(ls:*)", "Bash(cat:*)"],
        { cwd: root, encoding: "utf8", timeout: 10 * 60 * 1000, input: "" },
      );
    } else {
      throw new Error(`agent ${agent} not wired`);
    }
    fs.writeFileSync(path.join(dest, "agent.txt"), (run.stdout ?? "") + (run.stderr ? `\n--- stderr\n${run.stderr}` : ""));

    const status = execFileSync("git", ["status", "--porcelain", "--untracked-files=all"], { cwd: root, encoding: "utf8" });
    fs.writeFileSync(path.join(dest, "git-status.txt"), status);
    for (const f of ["events.jsonl", "findings.jsonl"]) {
      const p = path.join(root, ".trackline", f);
      if (fs.existsSync(p)) fs.copyFileSync(p, path.join(dest, f));
    }

    console.log(`${id}: judging with ${judge}`);
    const review = spawnSync(
      path.join(bin, "trackline"),
      ["review", "--json", "--provider", "cli", "--binary", judge, "--root", root],
      { encoding: "utf8", timeout: 10 * 60 * 1000 },
    );
    fs.writeFileSync(path.join(dest, "judge.jsonl"), review.stdout ?? "");
    if (review.status !== 0) fs.writeFileSync(path.join(dest, "judge-error.txt"), review.stderr ?? "");

    fs.writeFileSync(path.join(dest, "meta.json"), JSON.stringify({
      id, scenario: s.id, variant: v, agent, judge, root,
      agentExit: run.status, agentSeconds: Math.round((Date.now() - started) / 1000),
    }, null, 2));
    fs.writeFileSync(path.join(dest, "done"), "");
    fs.rmSync(root, { recursive: true, force: true });
  }
}
