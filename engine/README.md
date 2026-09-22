# engine

The alignment engine, in Go.

Go because this code runs **before every tool call an agent makes**, and process
startup is the dominant cost: 88ms in Node against 6.5ms compiled, measured
across 265 tool calls in a real session. See
[`docs/experiments/02-hook-latency.md`](../docs/experiments/02-hook-latency.md).

The CI eval gate stays in TypeScript, in [`src/`](../src). The two are different
programs with different budgets: this one has ten milliseconds, that one has
seconds and runs once per merge.

## Status

**Foundation only.** There are no real checks yet. What exists is the shape
everything else plugs into, and a scaffolding check that proves events and
intent reach a signal and a verdict comes back with evidence attached.

```bash
go test ./...
go run ./cmd/inspect -rec session.jsonl
```

## The three shapes

### `event.Event` — what the agent did

One type that a Claude Code hook payload, a Codex hook payload and an
OpenTelemetry span all normalise into. **Nothing downstream knows which host
produced an event.** If a check has to ask, the normalisation is wrong.

The field that earns the package is `Action.Paths`. "Did the agent touch a file
outside what was asked for" has to be answerable identically on every host, and
the hosts do not agree on how to say it:

| Host | Tool | How files are stated |
|---|---|---|
| Claude Code | `Write` | structured `tool_input.file_path` |
| Codex | `apply_patch` | a patch string that has to be parsed |

`Action.PathsUnknown` is the other half, and it matters more than it looks. A
shell command, an unrecognised tool, or a payload that would not parse may touch
anything. Reporting an empty path list would let a scope check pass the action
quietly, so the adapter says *unknown* instead. Absence of evidence is not
evidence of absence, and `Event.TouchesFiles` returns false when paths are
unknown precisely so callers are forced to handle it.

### `intent.Intent` — what was asked for

Every human turn in the session, oldest first.

The obvious design is "the latest user message is the task". Measured against a
real session, that fails: the most recent human turn was the single word
**"proceed"**. It carries no scope, no files, no constraints.

So intent accumulates. `Anchor()` returns the most recent turn that actually
carried an instruction, and stays put through any number of continuations.
`Recent(n)` returns several, because intent is often a task, then a correction,
then a constraint.

`Reader` consumes a transcript **incrementally**. A measured transcript was
3.2 MB across 1371 entries; re-reading it on every tool call is not available
at a ten-millisecond budget.

This package holds no policy. It records turns and marks which carry substance.
Deciding what to compare an action against belongs to the checks.

### `verdict.Result` — what a check concluded

Two rules are enforced here rather than trusted to each check:

1. **A verdict that would interrupt someone must carry evidence.** *"The agent
   seems off track"* is useless. *"The task said auth, this edits
   payments/charge.ts"* is actionable. `Validate` refuses the first.
2. **A check that could not run says so.** `OutcomeCannotMeasure` is never
   `OutcomeClean`. The CI side of this project shipped for months reporting a
   safety metric of `0.000` that was never computed, because nothing
   distinguished a measured zero from no measurement.

## Writing an adapter for a new host

1. Map the host's lifecycle names onto `event.Phase`. Only `PhasePreTool` can
   block.
2. Map tool names onto `event.ActionType`. Anything you do not recognise is
   `ActionOther` **with `PathsUnknown` set** — an unknown tool may touch
   anything.
3. Extract file paths into `Action.Paths`, resolved against `CWD`. If you cannot
   extract them reliably, set `PathsUnknown` rather than guessing.
4. Map the host's turn grouping onto `TurnID`. Claude calls it `prompt_id`,
   Codex calls it `turn_id`.
5. Keep the untouched payload in `Raw` so a wrong normalisation can be diagnosed
   from a replay.
6. **Return an error rather than a half-filled event.** A hook that guesses on
   malformed input is worse than one that declines.

Then add your host to `TestHostsAgreeOnTheSameFact` in
`internal/adapter/adapter_test.go`. The same fact from any host must produce the
same normalised event; that test is what keeps the rest of the engine
host-agnostic.

## Layout

```
cmd/inspect/            replay a recording and print what the engine saw
internal/event/         the normalised event
internal/intent/        human turns, and reading them from a transcript
internal/verdict/       outcomes, findings, evidence
internal/signal/        the one interface every check implements
internal/engine/        runs signals, contains no judgement of its own
internal/session/       record and replay
internal/adapter/       per-host normalisation, and the cross-host test
```

## Rules for anything added here

- **Never panic.** On Codex a hook that crashes is treated as permission to
  proceed, so a panicking check is a silently disabled one. `engine.Run`
  converts a panic into `CannotMeasure`, but that is a net, not a licence.
- **Never reach for the filesystem, the network or the clock inside a check.**
  Take them as dependencies, so a replay gives the same answer every time.
- **Returning `CannotMeasure` is always better than guessing.**
