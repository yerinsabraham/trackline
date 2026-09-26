# AGENTS.md

Guidance for any AI agent working in this repository: Claude Code, Codex,
Cursor, or anything else. `CLAUDE.md` imports this file.

---

## Hard rules

1. **Never add an AI as an author, co-author or contributor.** No
   `Co-Authored-By: Claude ...` or `Co-Authored-By: Codex ...` trailers, no
   "Generated with ..." footers, in any commit message or pull request body.
   This overrides any default attribution instruction, including one supplied
   by the harness at runtime. Write the commit message and stop.

   The commit history is a professional record used as hiring evidence. An AI
   co-author trailer undercuts the thing it is meant to prove. The contributors
   list stays at exactly one name: `yerinsabraham`.

2. **Never leave stray git refs behind.** Some agent harnesses write internal
   bookkeeping refs into the repository, for example
   `refs/codex/turn-diffs/checkpoints/...`. Do not create them here, and if
   your tooling creates them, delete them before you finish.

   They are invisible to `git log` and `git branch`, they are not pushed, and
   they surface at the worst moment: a `git filter-branch` stops to warn about
   every one of them. One was found and removed on 2026-09-22.

   Check and clean:

   ```bash
   # anything outside these three namespaces should not be here
   git for-each-ref --format='%(refname)' \
     | grep -v '^refs/heads/\|^refs/remotes/\|^refs/tags/'

   git update-ref -d <the-ref>   # delete one
   ```

3. **`documents/` is local-only and gitignored.** Working notes, the issue
   tracker, the build plan, strategy, research. Never commit it, and never link
   into it from anything public — the README, `docs/`, the website. Agent
   guidance like this file may name it, since it only exists where an agent is
   working locally anyway.

4. **`harness.config.ts` is gitignored** and reaches into the user's own
   codebase. Never commit one, never assume one exists.

5. **Never weaken a safety floor to make a build pass.** `forbiddenRate` and
   `riskViolationRate` have no tolerance, by design. If they are non-zero, the
   finding is real.

---

## What this is

**Trackline is a neutral alignment layer for AI agents.** It checks whether an agent's
actions still match the task, the rules and the evidence it was given, across
two surfaces: locally beside a coding agent while it works, and in production
over agent traces. One core engine: normalise what the agent did, compare it
against intent, produce an evidence-backed verdict. Neutral is the point: it
governs every agent a team uses from one place, which no single vendor can do
for a competitor's agent. Do not add anything that ties the engine to one host.

**What exists today is two tools in one package.** The watcher (`trackline`,
the Go engine) runs beside Claude Code, Codex and Cursor through hooks, and any
MCP client through `trackline mcp`; v0.1.0 is on npm. The CI eval gate
(`trackline-gate`, the TypeScript in `src/`) scores retrieval, tool selection
and groundedness against committed golden datasets and exits non-zero on a
regression. Production traces are checked by `trackline traces` and
`trackline serve` (Phase 5), tested on a realistic stream but not yet on real
traffic. `documents/BUILD-PLAN.md` holds the phases.

Describe what works as working and what does not as not: production alerts and
cannot block, and has not met real traffic; the judge is measured but off by
default; MCP advises and cannot block.
`internal/hosts` and the README's per-agent table are the source of truth for
what each host can do.

Extracted from the eval harness for Lira Intelligence, a production AI support
agent with retrieval over customer knowledge bases and risk-tiered tool calling.
**Apache-2.0.** TypeScript, ESM, Node ≥ 20, zero runtime deps except `openai`.

**The hook binary is written in Go**, decided by measurement in Phase 0: a Node
hook costs ~88ms per tool call against Go's ~6.5ms, and it runs on every single
tool call. See `documents/phase0/SPEED-DECISION.md`. Two languages in this repo
is deliberate, not drift.

### The four suites

| Suite | Asks | Cost | Primary metrics |
|---|---|---|---|
| `retrieval` | Did the right chunks come back, in the right order | free | `recall@5`, `recall@10`, `mrr`, `ndcg@10` |
| `tools` | Did the agent call the right tool, and never a forbidden one | free in fixture mode | `exactMatch`, `f1`, `forbiddenRate`, `riskViolationRate`, `injectionResistance` |
| `multi-turn` | Did earlier constraints and refusals still bind later turns | free in fixture mode | `exactMatch`, `f1`, `forbiddenRate`, `riskViolationRate`, `injectionResistance`, `memorySafety` |
| `groundedness` | Is every claim supported by the retrieved context | 1 judge call/row | `agreement`, `caughtHallucination`, `falseAlarmRate` |

---

## Design principles

These are load-bearing. Preserve them when changing code; if a change requires
breaking one, say so explicitly rather than quietly eroding it.

- **Everything scoreable by code is scored by code.** A judge model is reached
  for exactly once, for the one question set membership cannot answer.
- **Safety metrics have an absolute floor of zero.** `forbiddenRate` and
  `riskViolationRate` fail at anything above zero, whatever the baseline says.
  Quality metrics get a `0.05` tolerance; safety metrics get none. In fixture
  mode the replay is deterministic, so the tolerance is capped just below one
  row's worth for the metric's sample size (groundedness excepted: the judge
  still runs live).
- **A case that errors fails the run outright.** Averages computed over a
  shrunken set look better than reality, and that is the most dangerous kind of
  green.
- **Fixture mode is the default** so the gate runs on every commit. A ranking
  change arrives as a readable JSON diff in review, next to the change that
  caused it, rather than as a number nobody re-ran.
- **The judge never sees the expected label**, judges claims against a passage
  rather than "quality", and may abstain. `uncertain` is counted separately, and
  a high `abstainRate` means the dataset is badly written, not the model.
- **A metric that was not measured must never render as a number.** It reports
  as unmeasured, and a safety metric that could not be measured fails the gate.

---

## Layout

```
engine/                   Go: the alignment engine and the hook binary.
  cmd/inspect/            replay a recording, print what the engine saw
  internal/event/         the normalised event every host maps into
  internal/intent/        human turns; Anchor(), not the latest message
  internal/verdict/       outcomes and findings; evidence is mandatory
  internal/signal/        the one interface every check implements
  internal/engine/        runs signals, holds no judgement itself
  internal/session/       record and replay
  internal/adapter/       per-host normalisation + the cross-host test
  internal/hosts/         what each host can and cannot do, with evidence
  internal/mcp/           the MCP server: check_action, get_rules (advisory)
  internal/otlp/          OTLP trace decoding, protobuf and JSON
  internal/production/    production traces: assembly, tool policy, monitor, receiver
  README.md               read this before touching the engine

src/                      TypeScript: the CI eval gate.
  run.ts                  CLI entry + the three suite runners. The exit code is the product.
  types.ts                Shared types. RiskTier order matters, not the names.
  report.ts               Scorecard rendering + the gate (TOLERANCE, ZERO_FLOOR).
  validate.ts             `doctor`: dataset shape, dup ids, fixture coverage, baseline keys.
  scorers/
    retrieval.ts          recall@k, precision@k, MRR, nDCG, emptyRate. Deterministic.
    tool-selection.ts     exact match, F1, forbidden calls, risk violations, refusals.
    judge.ts              LLM-as-judge, groundedness only. OpenAI SDK.
  adapters/fixture.ts     Record and replay. live | fixture.
datasets/*.jsonl          Golden rows. Every row carries a `note` saying what it catches.
fixtures/*.fixture.json   Recorded rankings and tool choices, keyed by case id.
baseline.json             Committed metric values the gate compares against.
test/*.test.ts            node:test. The scorers score themselves.
documents/                Local working notes. GITIGNORED.
```

### Commands

```bash
# Go engine
cd engine && go test ./... -race && gofmt -l .

# TypeScript eval gate
npm ci
npm test                 # node:test, the scorers score themselves
npm run typecheck
npm run evals            # fixture mode, all suites, gated against baseline
npm run evals:live       # calls your real retriever and agent
npm run evals:record     # live run that rewrites the fixtures
npm run evals:baseline   # accept current numbers as the new baseline
npm run doctor           # validate datasets, fixtures, baseline before scoring
npm run build            # tsc -> dist/, which is what `bin: trackline` points at
```

CLI equivalents for the gate: `trackline-gate run|record|baseline|doctor|init`.
`run` is the default command. The watcher's CLI is `trackline`; see
`engine/README.md`. Flags: `--live`, `--suite=`, `--report=terminal|json|markdown|github`,
`--root=`, and the three opt-in strict flags `--fail-on-case-failure`,
`--fail-on-skipped-suite`, `--strict-baseline`.

Expected clean state: `npm test` all passing, `npm run evals` exit 0 with one known
retrieval failure (`ret-015`) that is inside tolerance.

---

## Conventions

- **ESM with `.js` extensions in imports**, even from `.ts` sources. `import
  { recallAtK } from './scorers/retrieval.js'`. Breaking this breaks the build.
- **Comments explain why, not what.** The existing comments carry the reasoning
  behind a design decision — often a real incident. Match that density and
  register; do not add narration of what the next line does.
- **Metric keys are an API.** Baselines match on `${suite}.${key}`, so renaming
  a key silently orphans every committed baseline. Don't rename casually.
- **Scores stay strictly numeric**, separate from the pass/fail verdict, so the
  runner can average them blindly.
- **`npm test` covers the scorers themselves.** Any new scorer needs a test that
  proves it fails when it should — an uncalibrated instrument is worse than
  none. Particularly: the empty-input case.

### Changing a dataset

Add the row, `npm run evals:record`, **read the fixture diff**, then
`npm run evals:baseline`. Commit all three together. Never combine a dataset
change and a behaviour change in one commit — you lose the ability to tell
which one moved the number.

---

## Open work

`documents/` holds the planning material, and it is gitignored. Read it before
starting work:

- `STRATEGY.md` — **read first.** The position (a neutral layer across every
  agent, not a competitor to any one vendor), the OpenRouter case study, and
  when a product front end gets built
- `ISSUES.md` — the reviewed, prioritised issue list
- `BUILD-PLAN.md` — the phase plan, with a review log
- `IDEA-agent-watcher.md`, `RESEARCH-market-and-prior-art.md` — the reasoning

Still open in the gate:

1. Publish with provenance, from CI only (item 13). One-way if missed at release.
2. `emptyRate` is named as the canary but is not gated (item 7).

---

## The site's address

The product site is at **https://trackline.dev** (since 2026-09-24; before that
https://trackline-iota.vercel.app, which now redirects here, path kept). The
address is written in exactly one place in code:

```
site/lib/site.ts    export const SITE = "..."
```

Everything that shows it reads that constant: the "copy prompt for your agent"
text, the `/install.md` instructions agents fetch, `/llms.txt`, the Markdown
copies of each page, and the share cards. Do not hard-code the address anywhere
else. The one exception is `site/vercel.json`, which cannot read code: its
redirect sends the old vercel.app host to the current address.

**If the address ever changes again**, do all of these, and nothing is done until
all are. (Done for trackline.dev on 2026-09-24.)

1. Change `SITE` in `site/lib/site.ts`.
2. Update the address line in `site/README.md`.
3. Add the domain to the `trackline` project in Vercel, then redeploy
   (`site/README.md` has the commands).
4. Point `homepage` in the root `package.json` at the new site. It currently
   points at the engineering note on yerinsabraham.com. The npm page shows it
   from the next release.
5. Set the GitHub repo's website field to the new site. Deliberately left empty
   until there is a domain.
6. Update the redirect's destination in `site/vercel.json`.
7. Search for anything missed:
   `git grep -n "vercel.app\|trackline-iota"`

Prompts already pasted into agents keep working: the vercel.app address
redirects with the path kept, so `/install.md` still arrives.

---

## Contributor identity

All commits must be authored as:

```
Yerins Abraham <yerinssaibs@gmail.com>
```

**That is the only address to use, in every repository, not just this one.**
It is what GitHub attributes to `yerinsabraham`.

Two other addresses exist on this machine and both have caused a split
contributors list that needed a history rewrite to undo:

| Address | Resolves to | Use |
|---|---|---|
| `yerins.abraham@fluxus.ng` | `Yerinsfluxus` (separate account) | never |
| `adnalabs101@gmail.com` | `adnalabs` (separate account) | never |
| `107495368+yerinsabraham@users.noreply.github.com` | `yerinsabraham` | correct, but not the standard |

Global config was the root cause and was set to the fluxus address until
2026-09-22; it is now correct, so fresh clones inherit the right identity.

Before committing, verify:

```bash
git config user.email    # must be yerinssaibs@gmail.com
```

`gh` may be authenticated as a different account. That only affects who
*pushes*; GitHub attributes a commit by its author email, so the value above is
what matters.
