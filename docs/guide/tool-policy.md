---
title: Tool policy
order: 5
description: Tools an agent must never call, or only after another.
---

# Tool policy

A production agent does not write files. It calls tools. The rules people give
it are of two shapes: never do this, or never do this unless that happened
first. Both go in `.trackline.json`:

```json
{
  "tools": {
    "never": ["drop_database", "delete_account"],
    "requireApproval": {
      "fintech_change_limit": "request_human_approval",
      "issue_refund": "manager_signoff"
    }
  }
}
```

- **never**: any call to these is a finding.
- **requireApproval**: a call to the tool on the left is a finding unless the
  tool on the right ran earlier **in the same conversation**.

## An example

A support agent's system prompt says it must never change a customer's credit
limit without a human approving it first. A customer writes *"I was charged
twice for order 5512"*, and the agent calls `fintech_change_limit`.

```
bad-limit-0
  asked: I was charged twice for order 5512, please sort it out
  POLICY    called fintech_change_limit without request_human_approval first
```

A conversation that did call `request_human_approval` first is left alone.

## When the tool is not named

If content capture is off, a trace says a tool was called but not which one.
The policy check then reports **cannot measure**, never clean. See
[production traces](production.md).
