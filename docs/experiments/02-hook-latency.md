# What a check before every tool call costs

**Answer: 88ms in Node, 6.5ms in Go. The hook is written in Go because of this.**

---

## Why it matters more than it looks

A `PreToolUse` hook runs **before every single tool call**, synchronously,
because it has to be able to block. Its startup cost is paid every time.

A single real session measured here made **265 tool calls**. So the cost is not
per-call milliseconds:

| Per call | Per session |
|---|---|
| 85ms | **22.5 seconds** |
| 20ms | 5.3 seconds |
| 5ms | 1.3 seconds |

Twenty-two seconds of lag, spread out as a pause before every action. That is
how a tool gets uninstalled.

## Method

Four equivalent hooks, each doing the same job: read stdin, parse the event,
check whether the tool input touches a forbidden path, exit 2 with a reason if
so. 40 runs each after a warm-up, median and p95 reported. Code in
[`code/latency/`](code/latency/).

## Results

| Runtime | median | p95 | per 265 calls |
|---|---|---|---|
| Node | 88.0ms | 135.6ms | 23.3s |
| Python 3 | 44.6ms | 87.8ms | 11.8s |
| Go client → warm Node daemon | 8.7ms | 15.7ms | 2.3s |
| **Go, compiled** | **6.5ms** | **7.3ms** | **1.7s** |
| C, compiled | 4.5ms | 6.4ms | 1.2s |

All four block correctly. The difference is entirely process startup.

## Why Go

**13.6x faster than Node, and far more predictable.** The p95 matters more than
the median, because lag is felt at the worst case:

- Node: median 88ms, **p95 136ms** — a 48ms spread
- Go: median 6.5ms, **p95 7.3ms** — a 0.8ms spread

Node's worst case is 19x Go's. Consistency is worth as much as speed here.

**It ships as one file with nothing to install.** All five targets cross-compile
from a single machine, around 2MB each: darwin/arm64, darwin/amd64, linux/amd64,
linux/arm64, windows/amd64. No runtime for the user to install and no version to
get wrong.

## Why not the alternatives

**C** was fastest and rejected anyway. Saving 2ms over Go costs manual memory
management and hand-rolled JSON parsing, in code whose entire job is parsing
untrusted JSON on a security-sensitive path. Wrong trade.

**A thin client talking to a warm daemon** was the interesting option, because
it would keep all logic in one language. At 8.7ms it is fast enough. Rejected
because:

- **p95 is 15.7ms, more than double its own median.** The socket round trip adds
  variance exactly where it hurts.
- It introduces a class of bug that otherwise does not exist: is the daemon
  running, did it crash, is the socket stale, who restarts it, what about
  Windows named pipes.
- When the daemon is down the client must fail open. Codex hooks already fail
  open on crash. Stacking a second fail-open path into a safety tool is the
  wrong direction.

## Consequence

Two languages, deliberately, because there are two performance regimes:

| Component | Language | Budget |
|---|---|---|
| Hook binary and deterministic checks | **Go** | under 10ms, every tool call |
| CI eval gate | TypeScript | seconds are fine, runs once per merge |

They are different programs. What is shared between them is design — baselines,
tolerances, safety metrics having no tolerance — not code.

---

## Follow-up: what the real hook actually costs

The measurements above compare bare runtimes doing trivial work. The shipped
hook does considerably more on every call: load configuration, read project
rules, read the session transcript, consult per-turn state, run five checks,
and write two files.

Measured on the real binary, five checks enabled:

| | median | p95 | per 265-call session |
|---|---|---|---|
| **trackline hook, all checks** | **11.5ms** | 13.4ms | **3.1s** |
| the same logic in Node would be | ~90ms+ | — | ~24s |

**The 10ms target in this document was derived from a trivial hook and the real
one does not meet it.** Recording that rather than moving the goalposts: 11.5ms
is the number, and it is still 8x better than the runtime it replaced.

### One design change the measurement forced

The first implementation of the checks that need memory — "how many files has
this request touched", "has this action been tried before" — read the session
recording back on every call. That made latency grow with the session:

| session length | latency |
|---|---|
| 0 events | 10.8ms |
| 150 events | 12.6ms |
| 300 events | 15.1ms |
| 600 events | 18.7ms |

Every call paid for the whole session, so the total cost was O(n²) and the tool
got slower the longer someone worked. That is the wrong shape for something
that runs before every action, and it would have been invisible in a short test.

The fix was to stop reading the recording on the hot path. A small state file
holds only the current turn and resets when the request changes, so reads are
bounded by how much an agent does in one request rather than by the length of
the session. The full recording is still written and is still what replay reads;
nothing time-critical touches it.

| session length | after |
|---|---|
| 0 events | 10.8ms |
| 150 events | 11.2ms |
| 300 events | 11.7ms |
| 600 events | 12.2ms |

A regression test asserts the state file stays small no matter how long the
session runs.
