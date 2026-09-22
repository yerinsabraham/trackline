# Experiments

Trackline's design rests on a few claims that would be expensive to get wrong.
Rather than assume them, each was measured first. This directory holds the
results, the code that produced them, and the raw data.

Every experiment here **changed a decision**. Three of them overturned an
assumption that was already written into the plan.

| | Question | Answer |
|---|---|---|
| [1](01-do-agents-self-correct.md) | When a hook blocks an agent and explains why, does the agent correct itself? | **Yes, 11 of 11**, across Claude Code and Codex |
| [2](02-hook-latency.md) | What does it cost to run a check before every tool call? | **88ms in Node, 6.5ms in Go.** The hook is written in Go because of this |
| [3](03-otel-traces.md) | Can alignment be checked from production traces? | **Only with content capture on** — but it survives PII redaction |
| [4](04-false-alarms.md) | Does it cry wolf on ordinary work? | **0 false alarms in 23 actions**, and real usage found what scripts missed |
| [5](05-the-judge.md) | Can a model tell on-task work from drift? | **8 of 8**, including three drift cases and two correct abstentions |

Raw data is in [`data/`](data/), runnable code in [`code/`](code/).

---

## Why publish these

An eval harness is a measuring instrument, and the same standard applies to the
reasoning behind it. Claims like "a compiled hook is faster" or "agents respond
to feedback" are easy to assert and cheap to check. These are the checks.

Where an experiment could not be run cleanly, that is stated rather than
smoothed over. Where a result has limits — one scenario, a small sample, a
particular phrasing — the limits are named next to the number.
