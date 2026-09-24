---
title: Modes and approvals
order: 3
description: Warn, ask or auto, per check, and how to say yes.
---

# Modes and approvals

Each check runs in one of three modes. You set them per check, so "never touch
`.env`" can stop the agent while "this looks out of scope" only asks.

| Mode | What happens |
|---|---|
| **warn** | Recorded. You read it with `trackline status` or `trackline show`. The agent is not interrupted. The default. |
| **ask** | The agent is stopped and told to ask you. If you say yes, it records your answer and carries on. |
| **auto** | The agent is stopped and told why, so it can correct itself. Only for findings that are certain: today that is **off-limits**. The heuristic checks never stop an agent in auto mode; use ask for those. |

```json
{
  "mode": "warn",
  "modes": {
    "off-limits": "auto",
    "scope": "ask"
  }
}
```

In `.trackline.json` at the root of your project.

## Why the agent is told, not just you

A rules file is read once at the start of a session and fades as the
conversation grows. Putting the violated rule back in front of the agent at the
moment it matters is what makes it correct itself. Measured: when blocked with a
reason and a concrete alternative, agents corrected themselves 11 times out of 11.

## Approvals

In ask mode, the agent is told the exact command that records your answer:

```bash
trackline allow scope "src/billing"              # for this request only
trackline allow scope "src/billing" --project    # until you revoke it
trackline allowed                                 # what has been approved
trackline revoke scope "src/billing"
```

Approvals are narrow on purpose. Approving `src/auth` does not approve
`src/authority`. Add `/**` to approve everything under a directory.

Agents cannot prompt you directly from a hook, so asking means stopping the
agent and telling it to ask you.
