---
title: Limits
order: 9
description: Where trackline sees less, stated plainly.
---

# Limits

Named plainly, because a tool that looks complete stops getting better.

## Everywhere

- **Scope is silent when your request names no file or area.** It will not
  invent one. Most requests do not name one, so scope is quieter than you might
  expect.
- **Shell commands are only partly readable.** Redirects, installs, `rm`, `mv`,
  `cp`, `tee`, in-place `sed` and read-only commands are understood. Anything
  else is reported as not checked, never as clean.
- **The judge is off by default** and measured once. See [the judge](judge.md).

## Per agent

| | Can stop an action | Tells the agent why | Knows what you asked |
|---|---|---|---|
| Claude Code | yes | yes | from the transcript |
| Codex | yes | yes | from the transcript |
| Cursor | yes | yes | from the transcript |
| Any MCP client | **no, advises** | yes | only what the agent says |
| Production traces | **no, alerts after** | no | only with content capture on |

- **Codex** runs a project hook only after you trust it with `/hooks`, and lets
  an action through if the hook crashes or times out.
- **Cursor** sees the agent only once it is inside the project.
- **MCP**: the agent chooses whether to ask.
- **Production**: without content capture, a trace does not name the tool that
  was called. And it has not yet been run against real traffic, only a stream
  sent by the real OpenTelemetry libraries with scripted conversations.

`trackline doctor --host <name>` prints the current list for any of them.
