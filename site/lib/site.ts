export const GITHUB = "https://github.com/yerinsabraham/trackline";
export const EXPERIMENTS = `${GITHUB}/blob/main/docs/experiments`;

// The experiments, as the evidence page shows them. Each answer is the
// published result, word for word in substance; the write-up has the method
// and the limits.
export const evidence = [
  {
    n: 1,
    file: "01-do-agents-self-correct.md",
    question: "When an agent is blocked and told why, does it correct itself?",
    answer: "Yes, 11 times out of 11",
    method: "Claude Code and Codex, blocked from a protected file with a concrete alternative named.",
  },
  {
    n: 2,
    file: "02-hook-latency.md",
    question: "What does a check before every action cost?",
    answer: "About 14 ms, in Go",
    method: "88 ms in Node against 6.5 ms for a bare Go start, across 265 tool calls; the full hook runs in 11 to 14 ms.",
  },
  {
    n: 3,
    file: "03-otel-traces.md",
    question: "Can alignment be checked from production traces?",
    answer: "Only with content capture on, and it survives redaction",
    method: "The official OpenTelemetry instrumentation, captured under four configurations.",
  },
  {
    n: 4,
    file: "04-false-alarms.md",
    question: "Does it cry wolf on ordinary work?",
    answer: "0 false alarms in 23 actions",
    method: "Ordinary sessions replayed through every check, and real usage that found what scripts missed.",
  },
  {
    n: 5,
    file: "05-the-judge.md",
    question: "Can a model tell on-task work from drift?",
    answer: "8 of 8 on a calibration set",
    method: "Superseded by experiment 6, which measured it properly.",
  },
  {
    n: 6,
    file: "06-the-judge-measured.md",
    question: "Does the judge earn its place, on real agents and held-out drift?",
    answer: "60 of 60 held-out drifts, against 2 for the rules",
    method: "Three agents, labels committed before any session ran, a judge from a different model family.",
  },
  {
    n: 7,
    file: "07-production-traces.md",
    question: "Does the same engine watch a deployed agent?",
    answer: "Yes, with zero lines of the core changed",
    method: "45 pre-registered conversations sent by the real OpenTelemetry libraries: every labelled problem caught, none of 29 on-task flagged.",
  },
];
