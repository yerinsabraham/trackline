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

## See it on your dashboard

Optional. To see your agents at [trackline.dev](https://trackline.dev/app) and
send them tasks from your phone, in each project:

```bash
trackline connect
```

It asks whether your messages to the agent and its replies go with what
trackline finds, then, the first time on a computer, opens the site to approve
it. Once a computer is linked, `trackline init` offers this for each new
project.

Sending from a device that has no passkey paired with your computer, such as a
borrowed laptop or a browser without passkeys, needs a send password. Set it on
the computer, once:

```bash
trackline remote password
```

Your computer checks it, as it checks a passkey. It needs 12 or more characters
and turns off after 5 wrong tries. Unlike a passkey, someone who stole the
server's data could try guessing it offline, so it is off until you set it.

## Updating

```bash
npm install -g trackline@latest
```

The app says when a newer release is out, and when a project needs
`trackline init` for hooks a release added.

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
