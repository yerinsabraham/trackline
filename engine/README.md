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

Working, and published as the `trackline` npm package.

- **Five checks** on every action, all arithmetic, none a model: `off-limits`,
  `dependency-added`, `scope`, `diff-size`, `repetition`. Warn mode by default;
  each can be set to ask or auto.
- **Three hosts through hooks** (Claude Code, Codex, Cursor) and **any MCP
  client** through `trackline mcp`, which advises and cannot block.
  `internal/hosts` says what each can and cannot do, with the evidence.
- **One judge**, off by default and never on the hook's path: `trackline
  review` asks a model whether each turn's work served its request. Measured in
  [experiment 6](../docs/experiments/06-the-judge-measured.md).

Production traces (the OpenTelemetry adapter) are parsed but not yet watched;
that is Phase 5.

```bash
go test ./... -race
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
| Cursor | `Write` | structured, but a one-line edit arrives as the whole file |
| any host | a shell tool | a command line, read by `internal/shell` where it can be |

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
   Codex `turn_id`, Cursor `generation_id`.
5. Keep the untouched payload in `Raw` so a wrong normalisation can be diagnosed
   from a replay.
6. **Return an error rather than a half-filled event.** A hook that guesses on
   malformed input is worse than one that declines.

Then add your host to `TestHostsAgreeOnTheSameFact` in
`internal/adapter/adapter_test.go`. The same fact from any host must produce the
same normalised event; that test is what keeps the rest of the engine
host-agnostic.

Build from a captured payload, never from the host's documentation. Cursor's
docs were wrong in three places, and Codex renamed its shell tool between
versions. Teach `internal/intent` the host's transcript, or intent-dependent
checks go blind without saying so, which happened to Codex for three phases.
And give the host an entry in `internal/hosts`: a test fails without one.

## Layout

```
cmd/hook/               the binary an agent runs before every tool call
cmd/trackline/          the CLI a person runs: init, status, doctor, review, mcp...
cmd/inspect/            replay a recording and print what the engine saw
internal/event/         the normalised event
internal/intent/        human turns, and reading them from each host's transcript
internal/verdict/       outcomes, findings, evidence
internal/signal/        the one interface every check implements, and the checks
internal/engine/        runs signals, contains no judgement of its own
internal/runner/        one pass: parse, read intent, check, apply approvals, decide
internal/session/       record and replay
internal/adapter/       per-host normalisation, and the cross-host test
internal/shell/         what a shell command touches, or that it cannot tell
internal/config/        .trackline.json and the project's rules files
internal/override/      approvals: once, or project-wide
internal/judge/         the model check, off by default, never in the hook
internal/hosts/         what each host can and cannot do, with evidence
internal/mcp/           the MCP server (advisory)
internal/install/       wiring the hook into each host, and proving it fires
internal/walkthrough/   turns a recorded session into a readable story (trackline show)
internal/e2e/           a raw payload through record, replay and judgement, end to end
```

## Rules for anything added here

- **Never panic.** On Codex a hook that crashes is treated as permission to
  proceed, so a panicking check is a silently disabled one. `engine.Run`
  converts a panic into `CannotMeasure`, but that is a net, not a licence.
- **Never reach for the filesystem, the network or the clock inside a check.**
  Take them as dependencies, so a replay gives the same answer every time.
- **Returning `CannotMeasure` is always better than guessing.**
