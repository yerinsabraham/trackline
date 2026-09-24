# Changelog

What changed in each release of `trackline`, newest first. The website's
changelog page is built from this file.

## 0.2.0 · 2026-09-24

- **Cursor.** `trackline init --host cursor` wires the hook into Cursor. It sees
  the agent once the agent is started inside the project.
- **Any MCP client.** `trackline mcp` runs an MCP server with `check_action` and
  `get_rules`, so an agent can ask before it acts. It advises. It cannot block.
- **Production traces.** `trackline traces` checks OTLP exports, protobuf or JSON.
  `trackline serve` receives them live and can post an alert. Both check tool
  policy (tools that must never be called, and approvals that must come first),
  outages, loops and drift. In production it alerts and never stops anything.
- **The judge, per action.** The optional judge now rules on each action rather
  than the work as a whole. `trackline review --json` gives machine-readable
  output, and the judge runs through Claude, Codex or Cursor's CLI.
- **Codex.** Reads Codex transcripts, including codex-cli 0.155's Bash tool and
  headless requests, instead of reading an unknown format as silence.
- **Fixes.** Commands that can write anywhere are no longer reported as reads. An
  unreadable rules file is reported instead of read as no rules. Every
  subcommand answers `--help` instead of running.

## 0.1.0 · 2026-09-23

The first release, for Claude Code and Codex.

- **The hook.** Runs before every action the agent takes, in about 14 ms.
- **The checks.** Off-limits paths, new dependencies, scope, diff size and
  repetition.
- **Modes.** Warn, ask or auto, set per check, with remembered approvals:
  `trackline allow`, `allowed` and `revoke`.
- **Commands.** `init`, `status`, `doctor`, `show` and `review`.
- **The judge.** Optional and off by default, for the one question counting
  cannot answer: whether the work serves the request.
- **Install.** `npm install -g trackline`, with a prebuilt binary for each
  platform.
