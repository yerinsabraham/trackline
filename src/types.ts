/**
 * Shared types.
 *
 * A suite is a dataset plus a scorer. Every scorer returns metrics in the same
 * shape, so the report and the CI gate never need to know which suite produced
 * a number.
 *
 * Metrics are 0..1 unless `unit` says otherwise. `higherIsBetter: false` exists
 * for latency and cost, which regress upward. A gate has to know which
 * direction is bad or it will happily pass a change that doubled p95.
 */

/**
 * Risk tiers, least to most privileged.
 *
 * Defined here rather than imported so the harness stands alone. If your system
 * uses different tiers, replace this list and `RISK_ORDER` in
 * `scorers/tool-selection.ts`. Order is what matters, not the names.
 */
export type RiskTier =
  | 'read_public'
  | 'read_private'
  | 'safe_write'
  | 'customer_confirm'
  | 'step_up'
  | 'admin_approve'
  | 'human_only';

/** One measured number in a scorecard. */
export interface Metric {
  /** Stable key. Baselines match on `${suite}.${key}`, so never rename casually. */
  key: string;
  value: number;
  unit?: 'ms' | 'usd' | 'count';
  higherIsBetter: boolean;
  /**
   * Primary metrics gate the build. Secondary ones are reported and tracked but
   * never block a merge. Latency on a laptop is noise, not signal.
   */
  primary: boolean;
}

/** What one dataset row produced, kept so a failure can be read rather than guessed at. */
export interface CaseResult {
  id: string;
  passed: boolean;
  scores: Record<string, number>;
  detail?: string;
  latencyMs?: number;
}

export interface SuiteResult {
  suite: string;
  metrics: Metric[];
  cases: CaseResult[];
  /** Cases that threw rather than scored. A crashed case is never a passing case. */
  errors: { id: string; message: string }[];
  skipped?: string;
}

export type RunMode = 'fixture' | 'live';

export interface RunReport {
  startedAt: string;
  gitSha: string | null;
  mode: RunMode;
  judgeModel: string | null;
  suites: SuiteResult[];
}

// ── Dataset row shapes ────────────────────────────────────────────────────────

/**
 * Retrieval golden row.
 *
 * `relevant` holds chunk ids in whatever form your retriever reports. The
 * reference implementation uses `${sourceId}#${chunkIndex}`, deliberately not a
 * vector-store point id: a dataset should survive an infrastructure change and
 * break only when the documents do.
 */
export interface RetrievalCase {
  id: string;
  query: string;
  relevant: string[];
  /** Graded relevance for nDCG. Missing ids default to 1. */
  grades?: Record<string, number>;
  segments?: string[];
  note?: string;
}

/**
 * Tool-selection golden row.
 *
 * `forbidden` is the important half. Getting the right tool is table stakes.
 * Never reaching for a destructive one under pressure is what the risk model
 * exists for, and it is the thing a prompt change silently breaks.
 */
export interface ToolSelectionCase {
  id: string;
  utterance: string;
  available: string[];
  /** Tools that must be called. Empty means the correct move is to answer, not act. */
  expected: string[];
  forbidden?: string[];
  /** The strictest tier the model may reach for unprompted. */
  maxRisk?: RiskTier;
  /** True when the right behaviour is to refuse and explain. */
  expectRefusal?: boolean;
  note?: string;
}

/** Groundedness row, judged by an LLM. */
export interface GroundednessCase {
  id: string;
  question: string;
  /** The retrieved context the answer must stay inside. */
  context: string[];
  answer: string;
  expect: 'grounded' | 'unsupported';
  note?: string;
}

/** What the model actually did, however it was produced. */
export interface ToolSelectionOutcome {
  called: string[];
  refused: boolean;
  /** Risk tier of each called tool, resolved from your catalog. */
  risks?: Record<string, RiskTier>;
}

// ── The two things you plug in ────────────────────────────────────────────────

/**
 * Your retriever.
 *
 * Return chunk ids in rank order, best first, using the same id scheme your
 * golden dataset uses.
 */
export interface Retriever {
  (query: string, opts: { topK: number; segments?: string[] }): Promise<string[]>;
}

/**
 * Your agent, asked what it *would* call. It must not execute anything.
 *
 * An eval that actually runs `freeze_card` is not an eval, and the suite is
 * built around utterances designed to tempt a model into destructive calls.
 */
export interface ToolSelector {
  (testCase: ToolSelectionCase): Promise<{ called: string[]; refused: boolean }>;
}

/**
 * Risk tier lookup for a tool name.
 *
 * Return `undefined` for a tool you do not recognise. The scorer counts only
 * tiers it can resolve, so an unknown name reads as a selection failure, which
 * it is, and never as a fake safety pass.
 */
export interface ToolCatalog {
  (name: string): RiskTier | undefined;
}

export interface HarnessConfig {
  retriever?: Retriever;
  toolSelector?: ToolSelector;
  toolCatalog?: ToolCatalog;
}
