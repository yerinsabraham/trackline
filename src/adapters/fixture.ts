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
import { fileURLToPath } from 'node:url';
import type {
  HarnessConfig,
  RetrievalCase,
  RunMode,
  ToolSelectionCase,
  ToolSelectionOutcome,
} from '../types.js';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const FIXTURE_DIR = path.join(HERE, '..', '..', 'fixtures');

const RETRIEVAL_FIXTURES = path.join(FIXTURE_DIR, 'retrieval.fixture.json');
const TOOL_FIXTURES = path.join(FIXTURE_DIR, 'tool-selection.fixture.json');

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
let toolCache: Record<string, { called: string[]; refused: boolean }> | null = null;

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
    retrievalCache ??= readFixtures<string[]>(RETRIEVAL_FIXTURES);
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
  writeFixtures(RETRIEVAL_FIXTURES, rankings);
  retrievalCache = null;
}

// ── Tool selection ────────────────────────────────────────────────────────────

export async function runToolSelection(
  testCase: ToolSelectionCase,
  mode: RunMode,
  config: HarnessConfig,
): Promise<ToolSelectionOutcome & { latencyMs: number }> {
  const resolveRisks = (called: string[]) => {
    if (!config.toolCatalog) return undefined;
    const risks: Record<string, NonNullable<ReturnType<NonNullable<HarnessConfig['toolCatalog']>>>> = {};
    for (const name of called) {
      const tier = config.toolCatalog(name);
      if (tier) risks[name] = tier;
    }
    return risks;
  };

  if (mode === 'fixture') {
    toolCache ??= readFixtures<{ called: string[]; refused: boolean }>(TOOL_FIXTURES);
    const recorded = toolCache[testCase.id];
    if (!recorded) {
      throw new Error(`No fixture for tool case "${testCase.id}". Re-record or remove the row.`);
    }
    return { ...recorded, risks: resolveRisks(recorded.called), latencyMs: 0 };
  }

  if (!config.toolSelector) {
    throw new Error('A live tool run needs `toolSelector` in your harness config. See config.example.ts.');
  }

  const started = Date.now();
  const outcome = await config.toolSelector(testCase);
  return { ...outcome, risks: resolveRisks(outcome.called), latencyMs: Date.now() - started };
}

export function saveToolFixtures(outcomes: Record<string, { called: string[]; refused: boolean }>): void {
  writeFixtures(TOOL_FIXTURES, outcomes);
  toolCache = null;
}
