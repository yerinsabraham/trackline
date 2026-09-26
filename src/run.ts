#!/usr/bin/env node
/**
 * trackline
 * Copyright 2026 Yerins Abraham. Licensed under the Apache License, Version 2.0.
 * https://github.com/yerinsabraham/trackline
 *
 * Runner.
 *
 *   npm run evals                    fixture mode, all suites, gate on baseline
 *   npm run evals -- --live          call your retriever and agent
 *   npm run evals -- --suite=tools   one suite
 *   npm run evals:record             live run that rewrites the fixtures
 *   npm run evals:baseline           accept current numbers as the new baseline
 *
 * The exit code is the point: non-zero on a regression, so this is one line in
 * CI rather than a report somebody remembers to read.
 */

import fs from 'node:fs';
import path from 'node:path';
import { execSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

import type {
  CaseResult,
  GroundednessCase,
  HarnessConfig,
  Metric,
  MultiTurnCase,
  MultiTurnFixture,
  RetrievalCase,
  RetrievalFixture,
  RunMode,
  RunReport,
  SuiteResult,
  ToolFixture,
  ToolSelectionCase,
} from './types.js';
import { emptyRate, ndcgAtK, precisionAtK, recallAtK, reciprocalRank } from './scorers/retrieval.js';
import { scoreMultiTurnCase, scoreToolCase } from './scorers/tool-selection.js';
import { DEFAULT_JUDGE_MODEL, judgeAgrees, judgeConfigured, judgeGroundedness, judgeProvider } from './scorers/judge.js';
import {
  retrievalInputHash,
  multiTurnInputHash,
  runRetrieval,
  runMultiTurn,
  runToolSelection,
  saveMultiTurnFixtures,
  saveRetrievalFixtures,
  saveToolFixtures,
  toolInputHash,
} from './adapters/fixture.js';
import {
  findRegressions,
  renderGate,
  renderMarkdownReport,
  renderReport,
  toBaseline,
  type Baseline,
} from './report.js';
import { findConfigFile, isTypeScriptConfig, typeScriptConfigAdvice } from './config.js';
import { renderValidationIssues, validateProject } from './validate.js';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PACKAGE_ROOT = path.join(HERE, '..');

// ── CLI ───────────────────────────────────────────────────────────────────────

const rawArgv = process.argv.slice(2);
const commands = new Set(['run', 'record', 'baseline', 'doctor', 'validate', 'init']);
const command = rawArgv[0] && commands.has(rawArgv[0]) ? rawArgv[0] : 'run';
const argv = command === 'run' ? rawArgv : rawArgv.slice(1);
const has = (flag: string) => argv.includes(flag);
const val = (name: string) => argv.find((a) => a.startsWith(`--${name}=`))?.split('=').slice(1).join('=');

const ROOT = path.resolve(val('root') ?? process.cwd());
const BASELINE_PATH = path.join(ROOT, 'baseline.json');
const RESULTS_DIR = path.join(ROOT, 'results');
const DATASET_DIR = path.join(ROOT, 'datasets');

const record = command === 'record' || has('--record');
const mode: RunMode = record || has('--live') ? 'live' : 'fixture';
const only = val('suite');
const updateBaseline = command === 'baseline' || has('--update-baseline');
const strictBaseline = has('--strict-baseline');
const failOnCaseFailure = has('--fail-on-case-failure');
const failOnSkippedSuite = has('--fail-on-skipped-suite');
const reportFormat = val('report') ?? 'terminal';
const reportFormats = new Set(['terminal', 'json', 'markdown', 'github']);

process.env.TRACKLINE_ROOT = ROOT;

/**
 * Your plug-ins, from a `harness.config.*` file if one exists.
 *
 * Absent is fine and is the normal state for a fresh clone: fixture mode needs
 * nothing, which is what lets someone run the suite before wiring up a thing.
 *
 * The awkward case is a `.ts` config under a Node that cannot strip types.
 * Node 20 cannot at all, and 22 only from 22.18 by default. The bare `import()`
 * used to surface that as `ERR_UNKNOWN_FILE_EXTENSION` from Node internals,
 * with no hint of what to do, on a path the project's own CI never exercised.
 */
async function loadConfig(): Promise<HarnessConfig> {
  const file = findConfigFile(ROOT);
  if (!file) return {};

  try {
    const mod = await import(pathToFileURL(file).href);
    return (mod.default ?? mod) as HarnessConfig;
  } catch (e) {
    const err = e as NodeJS.ErrnoException;
    if (err.code === 'ERR_UNKNOWN_FILE_EXTENSION' && isTypeScriptConfig(file)) {
      throw new Error(`Cannot load your harness config.\n\n${typeScriptConfigAdvice(file)}`);
    }
    throw e;
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function readJsonl<T>(name: string): T[] {
  return fs
    .readFileSync(path.join(DATASET_DIR, `${name}.jsonl`), 'utf8')
    .split('\n')
    .filter((l) => l.trim() && !l.trim().startsWith('//'))
    .map((l, i) => {
      try {
        return JSON.parse(l) as T;
      } catch (e) {
        throw new Error(`${name}.jsonl line ${i + 1} is not valid JSON: ${(e as Error).message}`);
      }
    });
}

function percentile(xs: number[], p: number): number {
  if (xs.length === 0) return 0;
  const sorted = [...xs].sort((a, b) => a - b);
  const idx = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, Math.min(sorted.length - 1, idx))];
}

const metric = (
  key: string,
  value: number | null,
  primary: boolean,
  higherIsBetter = true,
  unit?: Metric['unit'],
  sampleSize?: number,
): Metric => ({ key, value, primary, higherIsBetter, unit, sampleSize });

/**
 * A metric with nothing to measure.
 *
 * `isError` separates the two cases the gate must treat differently: the
 * dataset has no rows of this kind (harmless), versus the dataset asks the
 * question and the run could not answer it (a setup failure).
 */
const unmeasured = (
  key: string,
  reason: string,
  primary: boolean,
  higherIsBetter = true,
  isError = false,
  sampleSize?: number,
): Metric => ({
  key,
  value: null,
  unmeasured: reason,
  unmeasuredIsError: isError,
  primary,
  higherIsBetter,
  sampleSize,
});

/**
 * Mean over a set that may be empty.
 *
 * Returns `null` rather than 0 for an empty set, because zero is a measurement
 * and "there was nothing to measure" is not. Averaging an empty list to zero is
 * how `injectionResistance` reads as a catastrophic failure on a dataset with
 * no injection rows, and how a safety rate reads as clean on a check that never
 * ran.
 */
const meanOrNull = (xs: number[]): number | null =>
  xs.length === 0 ? null : xs.reduce((a, b) => a + b, 0) / xs.length;

function gitSha(): string | null {
  try {
    return execSync('git rev-parse --short HEAD', { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    return null;
  }
}

function concurrency(): number {
  const raw = val('concurrency') ?? process.env.TRACKLINE_EVAL_CONCURRENCY;
  if (!raw) return mode === 'live' ? 4 : 1;
  const n = Number(raw);
  return Number.isInteger(n) && n > 0 ? Math.min(n, 16) : 1;
}

async function mapLimit<T, R>(items: T[], limit: number, fn: (item: T, index: number) => Promise<R>): Promise<R[]> {
  const out = new Array<R>(items.length);
  let next = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    for (;;) {
      const i = next++;
      if (i >= items.length) return;
      out[i] = await fn(items[i]!, i);
    }
  });
  await Promise.all(workers);
  return out;
}

// ── Suites ────────────────────────────────────────────────────────────────────

async function retrievalSuite(config: HarnessConfig): Promise<SuiteResult> {
  const cases = readJsonl<RetrievalCase>('retrieval');
  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  const rankings: Record<string, RetrievalFixture> = {};
  const latencies: number[] = [];
  const raw: { retrieved: string[] }[] = [];

  const ran = await mapLimit(cases, concurrency(), async (c) => {
    try {
      const { retrieved, latencyMs } = await runRetrieval(c, mode, config);

      // A row with no relevant chunks asks the opposite question: did we
      // correctly return nothing. Scoring it with recall would hand it a free 1
      // for any output at all.
      const isNegative = c.relevant.length === 0;
      const scores = isNegative
        ? {
            recall5: retrieved.length === 0 ? 1 : 0,
            recall10: retrieved.length === 0 ? 1 : 0,
            mrr: 0,
            ndcg10: 0,
            precision5: 0,
          }
        : {
            recall5: recallAtK(retrieved, c.relevant, 5),
            recall10: recallAtK(retrieved, c.relevant, 10),
            precision5: precisionAtK(retrieved, c.relevant, 5),
            mrr: reciprocalRank(retrieved, c.relevant),
            ndcg10: ndcgAtK(retrieved, c.relevant, 10, c.grades),
          };

      return {
        fixture: { id: c.id, value: { retrieved, inputHash: retrievalInputHash(c) } },
        raw: { retrieved },
        latencyMs,
        result: {
        id: c.id,
        passed: isNegative ? retrieved.length === 0 : scores.recall5 === 1,
        scores,
        latencyMs,
        detail: isNegative
          ? retrieved.length > 0
            ? `expected no results, got ${retrieved.length}`
            : undefined
          : scores.recall5 < 1
            ? `recall@5 ${scores.recall5.toFixed(2)}; missing ${c.relevant
                .filter((r) => !retrieved.slice(0, 5).includes(r))
                .join(', ')}`
            : undefined,
        },
      };
    } catch (e) {
      return { error: { id: c.id, message: (e as Error).message } };
    }
  });

  for (const item of ran) {
    if ('error' in item && item.error) {
      errors.push(item.error);
      continue;
    }
    rankings[item.fixture.id] = item.fixture.value;
    latencies.push(item.latencyMs);
    raw.push(item.raw);
    results.push(item.result);
  }

  if (record) saveRetrievalFixtures(rankings);

  const positiveIds = new Set(cases.filter((c) => c.relevant.length > 0).map((c) => c.id));
  const positives = results.filter((r) => positiveIds.has(r.id));
  return {
    suite: 'retrieval',
    cases: results,
    errors,
    metrics: [
      metric('recall@5', meanOrNull(results.map((r) => r.scores.recall5)), true, true, undefined, results.length),
      metric('recall@10', meanOrNull(results.map((r) => r.scores.recall10)), true, true, undefined, results.length),
      // Ranking metrics are meaningless on negative rows, so they average over
      // positives only. With no positive rows there is nothing to rank.
      positives.length === 0
        ? unmeasured('mrr', 'no dataset row has relevant chunks', true)
        : metric('mrr', meanOrNull(positives.map((r) => r.scores.mrr)), true, true, undefined, positives.length),
      positives.length === 0
        ? unmeasured('ndcg@10', 'no dataset row has relevant chunks', true)
        : metric('ndcg@10', meanOrNull(positives.map((r) => r.scores.ndcg10)), true, true, undefined, positives.length),
      metric('precision@5', meanOrNull(positives.map((r) => r.scores.precision5)), false, true, undefined, positives.length),
      metric('emptyRate', emptyRate(raw), false, false, undefined, raw.length),
      metric('p95Latency', percentile(latencies, 95), false, false, 'ms', latencies.length),
    ],
  };
}

async function toolSuite(config: HarnessConfig): Promise<SuiteResult> {
  const cases = readJsonl<ToolSelectionCase>('tool-selection');
  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  const fixtures: Record<string, ToolFixture> = {};
  const latencies: number[] = [];

  // Which cases could actually have their risk tiers resolved. A case where
  // `risks` came back undefined was never checked for a risk violation, and
  // averaging it in as a zero would report a clean safety number for a check
  // that did not run.
  const riskResolvable = new Set<string>();

  const ran = await mapLimit(cases, concurrency(), async (c) => {
    try {
      const outcome = await runToolSelection(c, mode, config);

      const scored = scoreToolCase(outcome, c);
      return {
        riskResolvable: outcome.risks !== undefined,
        fixture: {
          id: c.id,
          value: {
            called: outcome.called,
            refused: outcome.refused,
            risks: outcome.risks,
            inputHash: toolInputHash(c),
          },
        },
        latencyMs: outcome.latencyMs,
        result: {
          id: c.id,
          passed: scored.passed,
          scores: scored.scores,
          detail: scored.detail,
          latencyMs: outcome.latencyMs,
        },
      };
    } catch (e) {
      return { error: { id: c.id, message: (e as Error).message } };
    }
  });

  for (const item of ran) {
    if ('error' in item && item.error) {
      errors.push(item.error);
      continue;
    }
    fixtures[item.fixture.id] = item.fixture.value;
    if (item.riskResolvable) riskResolvable.add(item.fixture.id);
    latencies.push(item.latencyMs);
    results.push(item.result);
  }

  if (record) saveToolFixtures(fixtures);

  const byId = new Map(cases.map((c) => [c.id, c]));
  const injection = results.filter((r) => byId.get(r.id)?.expectRefusal);

  // Only rows that declare a ceiling can violate one. Rows without `maxRisk`
  // are not evidence of safety; they are silent on the question.
  const riskRows = results.filter((r) => byId.get(r.id)?.maxRisk !== undefined);
  const riskScored = riskRows.filter((r) => riskResolvable.has(r.id));

  const riskMetric = (): Metric => {
    if (riskRows.length === 0) {
      return unmeasured('riskViolationRate', 'no dataset row declares maxRisk', true, false, false, 0);
    }
    if (riskScored.length === 0) {
      // The dataset asks the question and nothing can answer it. This is the
      // case that used to report 0.000 and pass the gate.
      return unmeasured(
        'riskViolationRate',
        `${riskRows.length} row(s) declare maxRisk but no risk tiers could be resolved. ` +
          'Re-record fixtures with a toolCatalog, or add one to harness.config.ts',
        true,
        false,
        true,
        riskRows.length,
      );
    }
    if (riskScored.length < riskRows.length) {
      // Partial coverage is still a hole. Score what we have and say so.
      const value = meanOrNull(riskScored.map((r) => r.scores.riskViolation));
      return {
        key: 'riskViolationRate',
        value,
        unmeasured: `${riskRows.length - riskScored.length} of ${riskRows.length} maxRisk row(s) had no resolvable tiers`,
        primary: true,
        higherIsBetter: false,
        sampleSize: riskScored.length,
      };
    }
    return metric('riskViolationRate', meanOrNull(riskScored.map((r) => r.scores.riskViolation)), true, false, undefined, riskScored.length);
  };

  return {
    suite: 'tools',
    cases: results,
    errors,
    metrics: [
      metric('exactMatch', meanOrNull(results.map((r) => r.scores.exactMatch)), true, true, undefined, results.length),
      metric('f1', meanOrNull(results.map((r) => r.scores.f1)), true, true, undefined, results.length),
      // Both of these carry a hard zero floor in the gate.
      metric('forbiddenRate', meanOrNull(results.map((r) => r.scores.forbidden)), true, false, undefined, results.length),
      riskMetric(),
      injection.length === 0
        ? unmeasured('injectionResistance', 'no dataset row sets expectRefusal', true)
        : metric('injectionResistance', meanOrNull(injection.map((r) => r.scores.refusal)), true, true, undefined, injection.length),
      metric('p95Latency', percentile(latencies, 95), false, false, 'ms', latencies.length),
    ],
  };
}

async function multiTurnSuite(config: HarnessConfig): Promise<SuiteResult> {
  // Optional, unlike the others: projects set up before this suite existed
  // have no file, and upgrading must not break their build.
  if (!fs.existsSync(path.join(DATASET_DIR, 'multi-turn.jsonl'))) {
    return { suite: 'multi-turn', cases: [], errors: [], metrics: [], skipped: 'no datasets/multi-turn.jsonl' };
  }
  const cases = readJsonl<MultiTurnCase>('multi-turn');
  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  const fixtures: Record<string, MultiTurnFixture> = {};
  const latencies: number[] = [];
  const riskResolvable = new Set<string>();

  const ran = await mapLimit(cases, concurrency(), async (c) => {
    try {
      const outcome = await runMultiTurn(c, mode, config);
      const scored = scoreMultiTurnCase(outcome, c);
      return {
        riskResolvable: outcome.turns.map((turn) => turn.risks !== undefined),
        fixture: {
          id: c.id,
          value: {
            turns: outcome.turns.map((turn) => ({
              called: turn.called,
              refused: turn.refused,
              risks: turn.risks,
            })),
            inputHash: multiTurnInputHash(c),
          },
        },
        latencyMs: outcome.latencyMs,
        result: {
          id: c.id,
          passed: scored.passed,
          scores: scored.scores,
          detail: scored.detail,
          latencyMs: outcome.latencyMs,
        },
      };
    } catch (e) {
      return { error: { id: c.id, message: (e as Error).message } };
    }
  });

  for (const item of ran) {
    if ('error' in item && item.error) {
      errors.push(item.error);
      continue;
    }
    fixtures[item.fixture.id] = item.fixture.value;
    item.riskResolvable.forEach((ok, i) => {
      if (ok) riskResolvable.add(`${item.fixture.id}.${i}`);
    });
    latencies.push(item.latencyMs);
    results.push(item.result);
  }

  if (record) saveMultiTurnFixtures(fixtures);

  const byId = new Map(cases.map((c) => [c.id, c]));
  const refusalRows = results.filter((r) => byId.get(r.id)?.turns.some((turn) => turn.expectRefusal));
  const memoryRows = results.filter((r) => byId.get(r.id)?.turns.some((turn) => turn.dependsOnPrevious));
  const riskRows = cases
    .flatMap((c) => c.turns.map((turn, i) => ({ caseId: c.id, id: `${c.id}.${i}`, turn })))
    .filter((row) => row.turn.maxRisk !== undefined);
  const riskCaseIds = new Set(riskRows.map((row) => row.caseId));
  const riskCases = results.filter((r) => riskCaseIds.has(r.id));
  const riskScored = riskCases.filter((r) => {
    const testCase = byId.get(r.id);
    return testCase?.turns.every((turn, i) => turn.maxRisk === undefined || riskResolvable.has(`${r.id}.${i}`));
  });

  const riskMetric = (): Metric => {
    if (riskRows.length === 0) return unmeasured('riskViolationRate', 'no multi-turn row declares maxRisk', true, false, false, 0);
    if (riskScored.length === 0) {
      return unmeasured(
        'riskViolationRate',
        `${riskRows.length} multi-turn turn(s) declare maxRisk but no risk tiers could be resolved. ` +
          'Re-record fixtures with a toolCatalog, or add one to harness.config.ts',
        true,
        false,
        true,
        riskRows.length,
      );
    }
    if (riskScored.length < riskCases.length) {
      return {
        key: 'riskViolationRate',
        value: meanOrNull(riskScored.map((r) => r.scores.riskViolation)),
        unmeasured: `${riskCases.length - riskScored.length} of ${riskCases.length} maxRisk multi-turn case(s) had no resolvable tiers`,
        primary: true,
        higherIsBetter: false,
        sampleSize: riskScored.length,
      };
    }
    return metric('riskViolationRate', meanOrNull(riskScored.map((r) => r.scores.riskViolation)), true, false, undefined, riskScored.length);
  };

  return {
    suite: 'multi-turn',
    cases: results,
    errors,
    metrics: [
      metric('exactMatch', meanOrNull(results.map((r) => r.scores.exactMatch)), true, true, undefined, results.length),
      metric('f1', meanOrNull(results.map((r) => r.scores.f1)), true, true, undefined, results.length),
      metric('forbiddenRate', meanOrNull(results.map((r) => r.scores.forbidden)), true, false, undefined, results.length),
      riskMetric(),
      refusalRows.length === 0
        ? unmeasured('injectionResistance', 'no multi-turn turn sets expectRefusal', true)
        : metric('injectionResistance', meanOrNull(refusalRows.map((r) => r.scores.refusal).filter((n) => !Number.isNaN(n))), true, true, undefined, refusalRows.length),
      memoryRows.length === 0
        ? unmeasured('memorySafety', 'no multi-turn turn depends on previous context', true)
        : metric('memorySafety', meanOrNull(memoryRows.map((r) => r.scores.memorySafety).filter((n) => !Number.isNaN(n))), true, true, undefined, memoryRows.length),
      metric('p95Latency', percentile(latencies, 95), false, false, 'ms', latencies.length),
    ],
  };
}

async function groundednessSuite(): Promise<SuiteResult> {
  const cases = readJsonl<GroundednessCase>('groundedness');

  // The only suite that costs money. Skipped rather than failed without a key,
  // so the free suites still gate a contributor with no judge access.
  if (!judgeConfigured()) {
    return { suite: 'groundedness', cases: [], errors: [], metrics: [], skipped: 'no judge API key' };
  }

  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  let uncertain = 0;

  const ran = await mapLimit(cases, concurrency(), async (c) => {
    try {
      const started = Date.now();
      const judged = await judgeGroundedness(c);

      const agrees = judgeAgrees(judged.verdict, c.expect);
      return {
        uncertain: judged.verdict === 'uncertain',
        result: {
          id: c.id,
          passed: agrees,
          scores: {
            agreement: agrees ? 1 : 0,
            // Recall over the rows that are genuinely unsupported: the share of
            // real hallucinations the judge caught. The number that matters most,
            // because a missed hallucination reaches a customer.
            caughtHallucination:
              c.expect === 'unsupported' ? (judged.verdict === 'unsupported' ? 1 : 0) : Number.NaN,
            falseAlarm: c.expect === 'grounded' ? (judged.verdict === 'unsupported' ? 1 : 0) : Number.NaN,
          },
          detail: agrees ? undefined : `judge said ${judged.verdict}, expected ${c.expect}. ${judged.reason}`,
          latencyMs: Date.now() - started,
        },
      };
    } catch (e) {
      return { error: { id: c.id, message: (e as Error).message } };
    }
  });

  for (const item of ran) {
    if ('error' in item && item.error) {
      errors.push(item.error);
      continue;
    }
    if (item.uncertain) uncertain += 1;
    results.push(item.result);
  }

  // NaN marks a score that does not apply to this row, so filtering it out is
  // how each rate gets the right denominator rather than the row count.
  const defined = (key: string) => results.map((r) => r.scores[key]).filter((n) => !Number.isNaN(n));
  const caught = defined('caughtHallucination');
  const falseAlarm = defined('falseAlarm');

  return {
    suite: 'groundedness',
    cases: results,
    errors,
    metrics: [
      metric('agreement', meanOrNull(results.map((r) => r.scores.agreement)), true, true, undefined, results.length),
      caught.length === 0
        ? unmeasured('caughtHallucination', 'no dataset row expects unsupported', true)
        : metric('caughtHallucination', meanOrNull(caught), true, true, undefined, caught.length),
      falseAlarm.length === 0
        ? unmeasured('falseAlarmRate', 'no dataset row expects grounded', true, false)
        : metric('falseAlarmRate', meanOrNull(falseAlarm), true, false, undefined, falseAlarm.length),
      results.length === 0
        ? unmeasured('abstainRate', 'no rows judged', false, false)
        : metric('abstainRate', uncertain / results.length, false, false, undefined, results.length),
    ],
  };
}

// ── Doctor / init ─────────────────────────────────────────────────────────────

function copyIfMissing(from: string, to: string): boolean {
  if (fs.existsSync(to)) return false;
  fs.mkdirSync(path.dirname(to), { recursive: true });
  fs.copyFileSync(from, to);
  return true;
}

function writeIfMissing(to: string, body: string): boolean {
  if (fs.existsSync(to)) return false;
  fs.mkdirSync(path.dirname(to), { recursive: true });
  fs.writeFileSync(to, body);
  return true;
}

function initEmptyProject(): void {
  const copied = [
    writeIfMissing(
      path.join(DATASET_DIR, 'retrieval.jsonl'),
      '// Add retrieval rows here, one JSON object per line.\n',
    ),
    writeIfMissing(
      path.join(DATASET_DIR, 'tool-selection.jsonl'),
      '// Add tool-selection rows here, one JSON object per line.\n',
    ),
    writeIfMissing(
      path.join(DATASET_DIR, 'multi-turn.jsonl'),
      '// Add multi-turn rows here, one JSON object per line.\n',
    ),
    writeIfMissing(
      path.join(DATASET_DIR, 'groundedness.jsonl'),
      '// Add groundedness rows here, one JSON object per line.\n',
    ),
    writeIfMissing(path.join(ROOT, 'fixtures', 'retrieval.fixture.json'), '{}\n'),
    writeIfMissing(path.join(ROOT, 'fixtures', 'tool-selection.fixture.json'), '{}\n'),
    writeIfMissing(path.join(ROOT, 'fixtures', 'multi-turn.fixture.json'), '{}\n'),
    writeIfMissing(BASELINE_PATH, '{}\n'),
    copyIfMissing(path.join(PACKAGE_ROOT, 'harness.config.example.ts'), path.join(ROOT, 'harness.config.example.ts')),
  ].filter(Boolean).length;

  console.log(copied === 0 ? 'Trackline already initialized.' : `Trackline initialized ${copied} empty file(s). Add rows, record fixtures, then update the baseline.`);
}

function initExampleProject(name: string): void {
  const dir = path.join(PACKAGE_ROOT, 'examples', name);
  if (!fs.existsSync(dir)) {
    console.error(`Unknown example "${name}". Available: fintech-support.`);
    process.exit(2);
  }
  const copied = [
    copyIfMissing(path.join(dir, 'datasets', 'retrieval.jsonl'), path.join(DATASET_DIR, 'retrieval.jsonl')),
    copyIfMissing(path.join(dir, 'datasets', 'tool-selection.jsonl'), path.join(DATASET_DIR, 'tool-selection.jsonl')),
    copyIfMissing(path.join(dir, 'datasets', 'multi-turn.jsonl'), path.join(DATASET_DIR, 'multi-turn.jsonl')),
    copyIfMissing(path.join(dir, 'datasets', 'groundedness.jsonl'), path.join(DATASET_DIR, 'groundedness.jsonl')),
    copyIfMissing(path.join(dir, 'fixtures', 'retrieval.fixture.json'), path.join(ROOT, 'fixtures', 'retrieval.fixture.json')),
    copyIfMissing(path.join(dir, 'fixtures', 'tool-selection.fixture.json'), path.join(ROOT, 'fixtures', 'tool-selection.fixture.json')),
    copyIfMissing(path.join(dir, 'fixtures', 'multi-turn.fixture.json'), path.join(ROOT, 'fixtures', 'multi-turn.fixture.json')),
    copyIfMissing(path.join(dir, 'baseline.json'), BASELINE_PATH),
    copyIfMissing(path.join(dir, 'harness.config.example.ts'), path.join(ROOT, 'harness.config.example.ts')),
  ].filter(Boolean).length;

  console.log(copied === 0 ? 'Trackline already initialized.' : `Trackline initialized ${copied} file(s) from ${name}.`);
}

function initProject(): void {
  const example = val('example');
  if (example) {
    initExampleProject(example);
    return;
  }
  initEmptyProject();
}

function runDoctor(opts: { quiet?: boolean } = {}): void {
  const issues = validateProject(ROOT, { mode, strictBaseline });
  const hasErrors = issues.some((i) => i.level === 'error');
  if (!opts.quiet || hasErrors) console.log(renderValidationIssues(issues));
  if (hasErrors) process.exit(1);
}

function writeReports(report: RunReport, baseline: Baseline, regressions: ReturnType<typeof findRegressions>): void {
  fs.mkdirSync(RESULTS_DIR, { recursive: true });
  fs.writeFileSync(path.join(RESULTS_DIR, 'latest.json'), `${JSON.stringify(report, null, 2)}\n`);

  if (reportFormat === 'json') {
    console.log(JSON.stringify({ report, regressions }, null, 2));
    return;
  }

  if (reportFormat === 'markdown' || reportFormat === 'github') {
    const markdown = renderMarkdownReport(report, baseline, regressions);
    fs.writeFileSync(path.join(RESULTS_DIR, 'latest.md'), markdown);
    if (reportFormat === 'markdown') console.log(markdown);
    if (reportFormat === 'github' && process.env.GITHUB_STEP_SUMMARY) {
      fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, markdown);
    }
  }
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function main() {
  if (!reportFormats.has(reportFormat)) {
    console.error(`Unknown report format "${reportFormat}". Available: terminal, json, markdown, github.`);
    process.exit(2);
  }

  if (command === 'init') {
    initProject();
    return;
  }

  if (command === 'doctor' || command === 'validate') {
    runDoctor();
    return;
  }

  runDoctor({ quiet: true });
  const config = await loadConfig();

  const all = [
    { name: 'retrieval', run: () => retrievalSuite(config) },
    { name: 'tools', run: () => toolSuite(config) },
    { name: 'multi-turn', run: () => multiTurnSuite(config) },
    { name: 'groundedness', run: () => groundednessSuite() },
  ].filter((s) => !only || s.name === only);

  if (all.length === 0) {
    console.error(`Unknown suite "${only}". Available: retrieval, tools, multi-turn, groundedness.`);
    process.exit(2);
  }

  const suites: SuiteResult[] = [];
  for (const s of all) suites.push(await s.run());

  const report: RunReport = {
    startedAt: new Date().toISOString(),
    gitSha: gitSha(),
    mode,
    judgeModel: judgeConfigured() ? `${judgeProvider()}:${process.env.EVAL_JUDGE_MODEL ?? DEFAULT_JUDGE_MODEL}` : null,
    suites,
  };

  const baseline: Baseline = fs.existsSync(BASELINE_PATH)
    ? JSON.parse(fs.readFileSync(BASELINE_PATH, 'utf8'))
    : {};

  if (updateBaseline) {
    console.log(renderReport(report, baseline));
    fs.writeFileSync(BASELINE_PATH, `${JSON.stringify(toBaseline(report), null, 2)}\n`);
    console.log('\nBaseline updated. Read the diff before committing it.\n');
    return;
  }

  // A case that threw never scored, so the averages above are computed over a
  // smaller set and look better than reality. Fail rather than report them.
  const errorCount = suites.reduce((n, s) => n + s.errors.length, 0);
  if (errorCount > 0) {
    console.error(`\n✗ ${errorCount} case(s) errored. The metrics above are incomplete.\n`);
    process.exit(1);
  }

  const regressions = findRegressions(report, baseline, { strictBaseline });
  writeReports(report, baseline, regressions);

  if (reportFormat === 'terminal' || reportFormat === 'github') {
    console.log(renderReport(report, baseline));
    console.log(renderGate(regressions));
  }

  if (failOnSkippedSuite) {
    const skipped = suites.filter((s) => s.skipped);
    if (skipped.length > 0) {
      console.error(`\n✗ ${skipped.length} suite(s) skipped: ${skipped.map((s) => s.suite).join(', ')}.\n`);
      process.exit(1);
    }
  }

  if (failOnCaseFailure) {
    const failed = suites.flatMap((s) => s.cases.filter((c) => !c.passed).map((c) => `${s.suite}.${c.id}`));
    if (failed.length > 0) {
      console.error(`\n✗ ${failed.length} case(s) failed: ${failed.slice(0, 10).join(', ')}${failed.length > 10 ? ', ...' : ''}.\n`);
      process.exit(1);
    }
  }

  if (regressions.length > 0) process.exit(1);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
