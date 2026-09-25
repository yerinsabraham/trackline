# Changelog

What changed in each release of `trackline`, newest first. The website's
changelog page is built from this file.

## 0.4.0 · 2026-09-25

- **Reply from your phone.** A task or a session can be continued from the
  app: the reply goes to the same Claude Code or Codex session on your laptop,
  with the same permissions and sandbox as a new one. Works for sessions you
  started in the terminal too.
- **macOS permission prompts.** When an agent started from your phone seems
  to be held by a Files and Folders prompt on the Mac (Documents, Desktop,
  Downloads, iCloud Drive), the app says so, so a task does not look frozen.

## 0.3.0 · 2026-09-25

- **Remote prompts.** Send a task from your phone to Claude Code or Codex on
  your own laptop, and watch it run. `trackline remote enable` turns it on for
  one project and pairs your passkey by a code shown on the laptop. Every
  message is signed by your passkey and checked on the laptop, so the server
  that carries it cannot write one. Remote sessions block writes to secrets and
  new dependencies, and run only where trackline is watching.
  `trackline remote disable --all` turns it off everywhere and forgets the
  passkeys.
- **An optional account.** `trackline connect` links a project to a trackline
  account and a live dashboard: what the agent is doing, its replies, and
  alerts. `trackline account` and `trackline disconnect`. Nothing uploads
  without it.
- **Setup.** `trackline init` sets up every installed agent when not told
  which, and `trackline status` says which agent is set up but has never
  reported, and why.

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
