---
title: Install
order: 1
description: Two commands, and how to check it is really running.
---

# Install

```bash
npm install -g trackline
cd your-project
trackline init
```

`init` wires trackline into Claude Code by default. For the others:

```bash
trackline init --host codex
trackline init --host cursor
```

It merges into the agent's existing settings and leaves everything else there
alone. It starts in **warn** mode: it notices things and writes them down, and
never interrupts you.

## Check that it fired

A hook that is set up wrong does not warn you. It just never runs. So after
your agent's next edit:

```bash
trackline status
```

If it says the hook has never run here, the setup is not working yet.

## Per agent

**Claude Code.** Hooks load mid-session, so there is nothing to restart.

**Codex.** Codex will not run a project's hook until you trust it. Run `/hooks`
inside Codex and approve trackline's, or it will silently never fire.

**Cursor.** Cursor loads project hooks only once the agent is inside the
project folder. Start the agent in the project. Anything it does before that is
not seen.

## Check the install without changing anything

```bash
trackline doctor --host cursor
```

`doctor` runs the hook binary against a test action, reads your config and
rules files, and prints what trackline can and cannot see in that agent.
