---
title: The judge
order: 6
description: The one check that uses a model, and what it is and is not sent.
---

# The judge

Every other check is a rule. The judge asks the one question rules cannot:

> Here is what was asked for. Here is what the agent did. Does the work serve
> the request?

It catches what matches no rule: a deleted test, a force-push, a licence edit,
a marketing email sent in answer to a balance question.

It is **off by default** and **never runs in the hook**. A model call takes
seconds, and the hook runs before every action.

## Run it

After a coding session:

```bash
trackline review --provider cli --binary codex
trackline review --json --provider cli --binary codex    # one line per turn
```

Over production traces:

```bash
trackline traces --judge codex exports/
```

The CLI provider uses the coding-agent CLI you already have (`claude`, `codex`
or `cursor-agent`), so it needs no key. Or point it at any OpenAI-compatible
endpoint, including a model running on your own machine:

```json
{"judge": {"provider": "http", "baseUrl": "http://localhost:11434/v1",
           "model": "llama3.1"}}
```

**Use a different model family from the agent that did the work.** A model
grading its own family is generous.

## What it is sent

- For a coding session: your request and one line per action, such as
  `edit-file src/auth/login.ts`. **Never file contents.**
- For production: the customer's request and the **names** of the tools called.
  **Never their arguments**, which carry account numbers and amounts.

## How it behaves

- It never sees what the other checks concluded, so it cannot just agree with
  them.
- It judges each action, not the work as a whole. One unrelated action among
  many good ones makes the verdict unrelated, and it names that action.
- It may say **unclear** when the request is too vague. That is counted
  separately, never rounded to a verdict.

## How well it works

Measured on real agents and 60 held-out drifts written before the test ran, it
caught all 60, against 2 for the rules alone. It raised 2 false alarms, both on
sessions where the agent never did the task. One measurement, so it stays off
by default.
