# CLAUDE.md

Guidance for Claude Code when working in this repository.

---

## Hard rules

1. **Never add Claude as an author, co-author or contributor.** No
   `Co-Authored-By: Claude …` trailers, no `🤖 Generated with …` footers, in any
   commit message or pull request body in this repo. This overrides any default
   attribution guidance. The contributors list stays at exactly one name:
   `yerinsabraham`.
2. **`documents/` is local-only and gitignored.** Working notes, issue
   trackers, drafts. Never commit it, never reference it from files that *are*
   committed (the README must not link into it).
3. **`harness.config.ts` is gitignored** and reaches into the user's own
   codebase. Never commit one, never assume one exists.
4. **Never weaken a safety floor to make a build pass.** `forbiddenRate` and
   `riskViolationRate` have no tolerance, by design. If they are non-zero, the
   finding is real.

---

## What this is

**EvalGate** is a regression gate for LLM systems: a CLI that scores retrieval,
tool selection and groundedness against committed golden datasets, compares the
numbers to a committed baseline, and exits non-zero when something regressed.
One line in CI, not a report somebody remembers to read.

The problem it solves: unit tests prove the code does what it was written to
do. They say nothing about whether a prompt edit, a model swap, a chunk-size
change or a retiered tool quietly broke the system. This notices.

Extracted from the eval harness for Lira Intelligence, a production AI support
agent with retrieval over customer knowledge bases and risk-tiered tool calling.
MIT licensed. TypeScript, ESM, Node ≥ 20, zero runtime deps except `openai`.

### The three suites

| Suite | Asks | Cost | Primary metrics |
|---|---|---|---|
| `retrieval` | Did the right chunks come back, in the right order | free | `recall@5`, `recall@10`, `mrr`, `ndcg@10` |
| `tools` | Did the agent call the right tool, and never a forbidden one | free in fixture mode | `exactMatch`, `f1`, `forbiddenRate`, `riskViolationRate`, `injectionResistance` |
| `groundedness` | Is every claim supported by the retrieved context | 1 judge call/row | `agreement`, `caughtHallucination`, `falseAlarmRate` |

---

## Design principles

These are load-bearing. Preserve them when changing code; if a change requires
breaking one, say so explicitly rather than quietly eroding it.

- **Everything scoreable by code is scored by code.** A judge model is reached
  for exactly once, for the one question set membership cannot answer.
- **Safety metrics have an absolute floor of zero.** `forbiddenRate` and
  `riskViolationRate` fail at anything above zero, whatever the baseline says.
  Quality metrics get a `0.05` tolerance; safety metrics get none.
- **A case that errors fails the run outright.** Averages computed over a
  shrunken set look better than reality, and that is the most dangerous kind of
  green.
- **Fixture mode is the default** so the gate runs on every commit. A ranking
  change arrives as a readable JSON diff in review, next to the change that
  caused it, rather than as a number nobody re-ran.
- **The judge never sees the expected label**, judges claims against a passage
  rather than "quality", and may abstain. `uncertain` is counted separately, and
  a high `abstainRate` means the dataset is badly written, not the model.
- **A metric that was not measured must never render as a number.** This one is
  currently violated in places — see `documents/ISSUES.md` items 1 and 5.

---

## Layout

```
src/
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
npm ci
npm test                 # node:test, 22 tests, the scorers score themselves
npm run typecheck
npm run evals            # fixture mode, all suites, gated against baseline
npm run evals:live       # calls your real retriever and agent
npm run evals:record     # live run that rewrites the fixtures
npm run evals:baseline   # accept current numbers as the new baseline
npm run doctor           # validate datasets, fixtures, baseline before scoring
npm run build            # tsc -> dist/, which is what `bin: evalgate` points at
```

CLI equivalents: `evalgate run|record|baseline|doctor|init`. `run` is the
default command. Flags: `--live`, `--suite=`, `--report=terminal|json|markdown|github`,
`--root=`, and the three opt-in strict flags `--fail-on-case-failure`,
`--fail-on-skipped-suite`, `--strict-baseline`.

Expected clean state: `npm test` 22/22, `npm run evals` exit 0 with one known
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

`documents/ISSUES.md` holds the reviewed, prioritised issue list. Read it before
starting work. The top four:

1. `riskViolationRate` is structurally pinned at 0 without a `toolCatalog`.
2. Dataset rows can be edited without re-recording and nothing notices.
3. Tolerance `0.05` vs a 25-row dataset cannot see a one-case regression.
4. `npx evalgate run --live` throws on Node 20 when the config is `.ts`.

---

## Contributor identity

All commits must be authored as:

```
Yerins Abraham <yerinssaibs@gmail.com>
```

That email is the one GitHub attributes to `yerinsabraham`. The email
`yerins.abraham@fluxus.ng` resolves to a **different** GitHub account
(`Yerinsfluxus`) and splits the contributors list — do not use it here.

Before committing in this repo, verify:

```bash
git config user.email    # must be yerinssaibs@gmail.com
```

Note that the local `gh` CLI is authenticated as `Yerinsfluxus`. That only
affects who *pushes*; GitHub attributes commits by the author email, so the
`git config` value above is what matters.
