/**
 * Record and replay.
 *
 * Two modes, one interface:
 *
 *  - **live** calls your retriever and your agent. The honest number, and the
 *    slow, paid one.
 *  - **fixture** replays a recorded ranking or tool choice from `fixtures/`.
 *    Free, offline, deterministic.
 *
 * Fixture mode is not a weaker eval. It is what makes the gate runnable on
 * every commit at all. A change to chunking, a filter, a prompt or a tool
 * description that moves a ranking shows up as a **diff to a committed JSON
 * file**, in review, next to the change that caused it, instead of as a number
 * nobody re-ran.
 *
 * Refresh with `npm run evals:record`, and read the diff before committing it.
 * A fixture updated without being read is how a regression gets blessed into
 * the baseline. That is the one way this harness can lie to you.
 */

import fs from 'node:fs';
import path from 'node:path';
import type {
  HarnessConfig,
  RetrievalCase,
  RiskTier,
  RunMode,
  ToolFixture,
  ToolSelectionCase,
  ToolSelectionOutcome,
} from '../types.js';

const projectRoot = () => path.resolve(process.env.TRACKLINE_ROOT ?? process.cwd());
const retrievalFixtures = () => path.join(projectRoot(), 'fixtures', 'retrieval.fixture.json');
const toolFixtures = () => path.join(projectRoot(), 'fixtures', 'tool-selection.fixture.json');

function readFixtures<T>(file: string): Record<string, T> {
  if (!fs.existsSync(file)) {
    throw new Error(`No fixtures at ${file}. Run \`npm run evals:record\` with a live config first.`);
  }
  return JSON.parse(fs.readFileSync(file, 'utf8')) as Record<string, T>;
}

function writeFixtures(file: string, data: unknown): void {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `${JSON.stringify(data, null, 2)}\n`);
}

let retrievalCache: Record<string, string[]> | null = null;
let toolCache: Record<string, ToolFixture> | null = null;

// ── Retrieval ─────────────────────────────────────────────────────────────────

export interface RetrievalRun {
  retrieved: string[];
  latencyMs: number;
}

export async function runRetrieval(
  testCase: RetrievalCase,
  mode: RunMode,
  config: HarnessConfig,
  topK = 10,
): Promise<RetrievalRun> {
  if (mode === 'fixture') {
    retrievalCache ??= readFixtures<string[]>(retrievalFixtures());
    const recorded = retrievalCache[testCase.id];
    if (!recorded) {
      throw new Error(`No fixture for retrieval case "${testCase.id}". Re-record or remove the row.`);
    }
    return { retrieved: recorded.slice(0, topK), latencyMs: 0 };
  }

  if (!config.retriever) {
    throw new Error('A live retrieval run needs `retriever` in your harness config. See config.example.ts.');
  }

  const started = Date.now();
  const retrieved = await config.retriever(testCase.query, { topK, segments: testCase.segments });
  return { retrieved, latencyMs: Date.now() - started };
}

export function saveRetrievalFixtures(rankings: Record<string, string[]>): void {
  writeFixtures(retrievalFixtures(), rankings);
  retrievalCache = null;
}

// ── Tool selection ────────────────────────────────────────────────────────────

export async function runToolSelection(
  testCase: ToolSelectionCase,
  mode: RunMode,
  config: HarnessConfig,
): Promise<ToolSelectionOutcome & { latencyMs: number }> {
  const resolveRisks = (called: string[]): Record<string, RiskTier> | undefined => {
    if (!config.toolCatalog) return undefined;
    const risks: Record<string, RiskTier> = {};
    for (const name of called) {
      const tier = config.toolCatalog(name);
      if (tier) risks[name] = tier;
    }
    return risks;
  };

  if (mode === 'fixture') {
    toolCache ??= readFixtures<ToolFixture>(toolFixtures());
    const recorded = toolCache[testCase.id];
    if (!recorded) {
      throw new Error(`No fixture for tool case "${testCase.id}". Re-record or remove the row.`);
    }
    // Recorded tiers win. They were resolved from the live catalog at record
    // time, which is what lets fixture mode score risk violations at all on a
    // fresh clone with no `harness.config.ts`. A live catalog, if one is
    // present, is the fallback for fixtures recorded before tiers were stored.
    return {
      ...recorded,
      risks: recorded.risks ?? resolveRisks(recorded.called),
      latencyMs: 0,
    };
  }

  if (!config.toolSelector) {
    throw new Error('A live tool run needs `toolSelector` in your harness config. See config.example.ts.');
  }

  const started = Date.now();
  const outcome = await config.toolSelector(testCase);
  return { ...outcome, risks: resolveRisks(outcome.called), latencyMs: Date.now() - started };
}

export function saveToolFixtures(outcomes: Record<string, ToolFixture>): void {
  writeFixtures(toolFixtures(), outcomes);
  toolCache = null;
}
