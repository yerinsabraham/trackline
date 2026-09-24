---
title: Configuration
order: 8
description: Every field in .trackline.json.
---

# Configuration

One file, `.trackline.json`, at the root of your project. Every field is
optional; anything you leave out keeps its default.

```json
{
  "mode": "warn",
  "modes": { "off-limits": "auto", "scope": "ask" },
  "offLimits": ["config/prod.yaml", "migrations/**"],
  "ruleFiles": ["AGENTS.md", "CLAUDE.md", ".cursorrules"],
  "disabled": ["diff-size"],
  "tools": {
    "never": ["drop_database"],
    "requireApproval": { "fintech_change_limit": "request_human_approval" }
  },
  "judge": { "provider": "cli", "binary": "codex" }
}
```

| Field | Default | Meaning |
|---|---|---|
| `mode` | `warn` | the mode for any check without its own |
| `modes` | none | a mode per check: `warn`, `ask` or `auto` ([modes](modes.md)) |
| `offLimits` | secrets and keys | extra protected patterns, **added** to the defaults; a leading `!` makes an exception |
| `ruleFiles` | `AGENTS.md`, `CLAUDE.md`, `.cursorrules` | where your project's rules are read from |
| `disabled` | none | checks that should not run |
| `tools` | none | the production [tool policy](tool-policy.md) |
| `judge` | off | `provider` (`cli` or `http`), and `binary`, or `baseUrl`, `model` and `apiKeyEnv` |

Check names: `off-limits`, `dependency-added`, `scope`, `diff-size`,
`repetition`, `tool-policy`.

For the http judge, `apiKeyEnv` names an environment variable that holds the
key, so a key is never written into a file that gets committed.

## If the file is broken

A config file that will not parse is reported on every action and never stops
your work: the defaults carry on. A rules file that exists but cannot be read is
reported too, naming the file, rather than treated as a project with no rules.
