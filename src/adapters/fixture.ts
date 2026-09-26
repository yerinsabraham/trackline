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
import { createHash } from 'node:crypto';
import type {
  HarnessConfig,
  MultiTurnCase,
  MultiTurnFixture,
  MultiTurnOutcome,
  RetrievalCase,
  RetrievalFixture,
  RiskTier,
  RunMode,
  ToolFixture,
  ToolSelectionCase,
  ToolSelectionOutcome,
} from '../types.js';

// ── Input hashing ─────────────────────────────────────────────────────────────

/**
 * A fingerprint of what the system under test was actually asked.
 *
 * Fixtures key on case id alone, so editing a row's query while keeping its id
 * leaves a stale recording that replays without complaint, answering a question
 * the dataset no longer asks. This is what makes that detectable.
 *
 * It covers the *inputs* only, never the expected answers. Changing `relevant`
 * or `forbidden` changes how a recording is scored; it does not make the
 * recording untrue. Hashing the answer key would demand a re-record every time
 * the grading was refined, and a re-record nobody needed is a re-record nobody
 * reads.
 *
 * Twelve hex characters: enough that a collision is not a practical concern,
 * short enough to read in a diff.
 */
function hashInputs(parts: unknown): string {
  return createHash('sha256').update(JSON.stringify(parts)).digest('hex').slice(0, 12);
}

export function retrievalInputHash(testCase: RetrievalCase): string {
  return hashInputs({ query: testCase.query, segments: testCase.segments ?? null });
}

export function toolInputHash(testCase: ToolSelectionCase): string {
  return hashInputs({ utterance: testCase.utterance, available: testCase.available });
}

export function multiTurnInputHash(testCase: MultiTurnCase): string {
  return hashInputs({
    turns: testCase.turns.map((turn) => ({ utterance: turn.utterance, available: turn.available })),
  });
}

/**
 * A recorded ranking, in either the current shape or the pre-hash bare array.
 *
 * Old fixtures keep working. They simply cannot be checked for staleness, and
 * `doctor` says so rather than pretending the check passed.
 */
export function readRetrievalFixture(value: unknown): RetrievalFixture | null {
  if (Array.isArray(value) && value.every((v) => typeof v === 'string')) {
    return { retrieved: value as string[] };
  }
  if (value && typeof value === 'object' && Array.isArray((value as RetrievalFixture).retrieved)) {
    return value as RetrievalFixture;
  }
  return null;
}

/** Message shared by the runner and `doctor` so they never drift apart. */
export function staleFixtureMessage(kind: string, id: string): string {
  return (
    `Fixture for ${kind} case "${id}" was recorded against different inputs. ` +
    'The dataset row was edited after it was recorded, so this replay answers a question the row no longer asks. ' +
    'Re-record, read the diff, then update the baseline.'
  );
}

const projectRoot = () => path.resolve(process.env.TRACKLINE_ROOT ?? process.cwd());
const retrievalFixtures = () => path.join(projectRoot(), 'fixtures', 'retrieval.fixture.json');
const toolFixtures = () => path.join(projectRoot(), 'fixtures', 'tool-selection.fixture.json');
const multiTurnFixtures = () => path.join(projectRoot(), 'fixtures', 'multi-turn.fixture.json');

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

let retrievalCache: Record<string, unknown> | null = null;
let toolCache: Record<string, ToolFixture> | null = null;
let multiTurnCache: Record<string, MultiTurnFixture> | null = null;

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
    retrievalCache ??= readFixtures<unknown>(retrievalFixtures());
    const raw = retrievalCache[testCase.id];
    if (raw === undefined) {
      throw new Error(`No fixture for retrieval case "${testCase.id}". Re-record or remove the row.`);
    }
    const recorded = readRetrievalFixture(raw);
    if (!recorded) {
      throw new Error(`Fixture for retrieval case "${testCase.id}" is malformed. Re-record it.`);
    }
    // A hash that disagrees means the row was edited after recording. Replaying
    // it would score a question the dataset no longer asks. An absent hash is a
    // pre-hash fixture: it replays, and `doctor` reports that it cannot be
    // checked.
    if (recorded.inputHash && recorded.inputHash !== retrievalInputHash(testCase)) {
      throw new Error(staleFixtureMessage('retrieval', testCase.id));
    }
    return { retrieved: recorded.retrieved.slice(0, topK), latencyMs: 0 };
  }

  if (!config.retriever) {
    throw new Error('A live retrieval run needs `retriever` in your harness config. See config.example.ts.');
  }

  const started = Date.now();
  const retrieved = await config.retriever(testCase.query, { topK, segments: testCase.segments });
  return { retrieved, latencyMs: Date.now() - started };
}

export function saveRetrievalFixtures(rankings: Record<string, RetrievalFixture>): void {
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
    if (recorded.inputHash && recorded.inputHash !== toolInputHash(testCase)) {
      throw new Error(staleFixtureMessage('tool', testCase.id));
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

// ── Multi-turn tool selection ────────────────────────────────────────────────

export async function runMultiTurn(
  testCase: MultiTurnCase,
  mode: RunMode,
  config: HarnessConfig,
): Promise<MultiTurnOutcome & { latencyMs: number }> {
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
    multiTurnCache ??= readFixtures<MultiTurnFixture>(multiTurnFixtures());
    const recorded = multiTurnCache[testCase.id];
    if (!recorded) {
      throw new Error(`No fixture for multi-turn case "${testCase.id}". Re-record or remove the row.`);
    }
    if (recorded.inputHash && recorded.inputHash !== multiTurnInputHash(testCase)) {
      throw new Error(staleFixtureMessage('multi-turn', testCase.id));
    }
    return {
      turns: recorded.turns.map((turn) => ({
        ...turn,
        risks: turn.risks ?? resolveRisks(turn.called),
      })),
      latencyMs: 0,
    };
  }

  if (!config.multiTurnToolSelector) {
    throw new Error('A live multi-turn run needs `multiTurnToolSelector` in your harness config. See config.example.ts.');
  }

  const started = Date.now();
  const outcome = await config.multiTurnToolSelector(testCase);
  return {
    turns: outcome.turns.map((turn) => ({ ...turn, risks: resolveRisks(turn.called) })),
    latencyMs: Date.now() - started,
  };
}

export function saveMultiTurnFixtures(outcomes: Record<string, MultiTurnFixture>): void {
  writeFixtures(multiTurnFixtures(), outcomes);
  multiTurnCache = null;
}
