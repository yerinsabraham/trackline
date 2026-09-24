---
title: MCP
order: 7
description: Reaching agents with no hooks, and the limit that comes with it.
---

# MCP

Agents trackline has no hook for can still ask it before they act, through MCP.

```bash
trackline mcp --root /path/to/your/project
```

It serves two tools:

- **check_action**: "may I write this file / run this command / install this
  package?" The answer is the one the hook would give: proceed, stop, or ask.
- **get_rules**: the protected paths, how each check acts, and the project's
  rules files.

Add it to your client's MCP server config:

```json
{
  "mcpServers": {
    "trackline": {
      "command": "trackline",
      "args": ["mcp", "--root", "/path/to/your/project"]
    }
  }
}
```

**Always pass `--root`.** Some clients start servers far from the project, and
trackline would then check the wrong folder. Every answer names the project it
checked, so you can see if that happens.

## The limit

**The agent chooses whether to ask.** A hook sees every action whether the
agent likes it or not. An MCP tool is consulted, and an agent that does not
consult it is not watched. MCP widens reach to agents without hooks. It does
not replace them.
