#!/usr/bin/env node
/**
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
  RetrievalCase,
  RunMode,
  RunReport,
  SuiteResult,
  ToolSelectionCase,
} from './types.js';
import { emptyRate, ndcgAtK, precisionAtK, recallAtK, reciprocalRank } from './scorers/retrieval.js';
import { scoreToolCase } from './scorers/tool-selection.js';
import { DEFAULT_JUDGE_MODEL, judgeAgrees, judgeGroundedness } from './scorers/judge.js';
import {
  runRetrieval,
  runToolSelection,
  saveRetrievalFixtures,
  saveToolFixtures,
} from './adapters/fixture.js';
import {
  findRegressions,
  renderGate,
  renderMarkdownReport,
  renderReport,
  toBaseline,
  type Baseline,
} from './report.js';
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

process.env.EVALGATE_ROOT = ROOT;

/**
 * Your plug-ins, from `harness.config.ts` if it exists.
 *
 * Absent is fine and is the normal state for a fresh clone: fixture mode needs
 * nothing, which is what lets someone run the suite before wiring up a thing.
 */
async function loadConfig(): Promise<HarnessConfig> {
  for (const name of ['harness.config.ts', 'harness.config.js']) {
    const file = path.join(ROOT, name);
    if (fs.existsSync(file)) {
      const mod = await import(pathToFileURL(file).href);
      return (mod.default ?? mod) as HarnessConfig;
    }
  }
  return {};
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

const mean = (xs: number[]) => (xs.length === 0 ? 0 : xs.reduce((a, b) => a + b, 0) / xs.length);

function percentile(xs: number[], p: number): number {
  if (xs.length === 0) return 0;
  const sorted = [...xs].sort((a, b) => a - b);
  const idx = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, Math.min(sorted.length - 1, idx))];
}

const metric = (
  key: string,
  value: number,
  primary: boolean,
  higherIsBetter = true,
  unit?: Metric['unit'],
): Metric => ({ key, value, primary, higherIsBetter, unit });

function gitSha(): string | null {
  try {
    return execSync('git rev-parse --short HEAD', { encoding: 'utf8' }).trim();
  } catch {
    return null;
  }
}

// ── Suites ────────────────────────────────────────────────────────────────────

async function retrievalSuite(config: HarnessConfig): Promise<SuiteResult> {
  const cases = readJsonl<RetrievalCase>('retrieval');
  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  const rankings: Record<string, string[]> = {};
  const latencies: number[] = [];
  const raw: { retrieved: string[] }[] = [];

  for (const c of cases) {
    try {
      const { retrieved, latencyMs } = await runRetrieval(c, mode, config);
      rankings[c.id] = retrieved;
      latencies.push(latencyMs);
      raw.push({ retrieved });

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

      results.push({
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
      });
    } catch (e) {
      errors.push({ id: c.id, message: (e as Error).message });
    }
  }

  if (record) saveRetrievalFixtures(rankings);

  const positiveIds = new Set(cases.filter((c) => c.relevant.length > 0).map((c) => c.id));
  const positives = results.filter((r) => positiveIds.has(r.id));
  return {
    suite: 'retrieval',
    cases: results,
    errors,
    metrics: [
      metric('recall@5', mean(results.map((r) => r.scores.recall5)), true),
      metric('recall@10', mean(results.map((r) => r.scores.recall10)), true),
      metric('mrr', mean(positives.map((r) => r.scores.mrr)), true),
      metric('ndcg@10', mean(positives.map((r) => r.scores.ndcg10)), true),
      metric('precision@5', mean(positives.map((r) => r.scores.precision5)), false),
      metric('emptyRate', emptyRate(raw), false, false),
      metric('p95Latency', percentile(latencies, 95), false, false, 'ms'),
    ],
  };
}

async function toolSuite(config: HarnessConfig): Promise<SuiteResult> {
  const cases = readJsonl<ToolSelectionCase>('tool-selection');
  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  const fixtures: Record<string, { called: string[]; refused: boolean }> = {};
  const latencies: number[] = [];

  for (const c of cases) {
    try {
      const outcome = await runToolSelection(c, mode, config);
      fixtures[c.id] = { called: outcome.called, refused: outcome.refused };
      latencies.push(outcome.latencyMs);

      const scored = scoreToolCase(outcome, c);
      results.push({
        id: c.id,
        passed: scored.passed,
        scores: scored.scores,
        detail: scored.detail,
        latencyMs: outcome.latencyMs,
      });
    } catch (e) {
      errors.push({ id: c.id, message: (e as Error).message });
    }
  }

  if (record) saveToolFixtures(fixtures);

  const injection = results.filter((r) => cases.find((c) => c.id === r.id)?.expectRefusal);

  return {
    suite: 'tools',
    cases: results,
    errors,
    metrics: [
      metric('exactMatch', mean(results.map((r) => r.scores.exactMatch)), true),
      metric('f1', mean(results.map((r) => r.scores.f1)), true),
      // Both of these carry a hard zero floor in the gate.
      metric('forbiddenRate', mean(results.map((r) => r.scores.forbidden)), true, false),
      metric('riskViolationRate', mean(results.map((r) => r.scores.riskViolation)), true, false),
      metric('injectionResistance', mean(injection.map((r) => r.scores.refusal)), true),
      metric('p95Latency', percentile(latencies, 95), false, false, 'ms'),
    ],
  };
}

async function groundednessSuite(): Promise<SuiteResult> {
  const cases = readJsonl<GroundednessCase>('groundedness');

  // The only suite that costs money. Skipped rather than failed without a key,
  // so the free suites still gate a contributor with no judge access.
  if (!process.env.OPENAI_API_KEY) {
    return { suite: 'groundedness', cases: [], errors: [], metrics: [], skipped: 'no OPENAI_API_KEY' };
  }

  const results: CaseResult[] = [];
  const errors: SuiteResult['errors'] = [];
  let uncertain = 0;

  for (const c of cases) {
    try {
      const started = Date.now();
      const judged = await judgeGroundedness(c);
      if (judged.verdict === 'uncertain') uncertain += 1;

      const agrees = judgeAgrees(judged.verdict, c.expect);
      results.push({
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
      });
    } catch (e) {
      errors.push({ id: c.id, message: (e as Error).message });
    }
  }

  const defined = (key: string) => results.map((r) => r.scores[key]).filter((n) => !Number.isNaN(n));

  return {
    suite: 'groundedness',
    cases: results,
    errors,
    metrics: [
      metric('agreement', mean(results.map((r) => r.scores.agreement)), true),
      metric('caughtHallucination', mean(defined('caughtHallucination')), true),
      metric('falseAlarmRate', mean(defined('falseAlarm')), true, false),
      metric('abstainRate', results.length ? uncertain / results.length : 0, false, false),
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

function initProject(): void {
  const copied = [
    copyIfMissing(path.join(PACKAGE_ROOT, 'datasets', 'retrieval.jsonl'), path.join(DATASET_DIR, 'retrieval.jsonl')),
    copyIfMissing(path.join(PACKAGE_ROOT, 'datasets', 'tool-selection.jsonl'), path.join(DATASET_DIR, 'tool-selection.jsonl')),
    copyIfMissing(path.join(PACKAGE_ROOT, 'datasets', 'groundedness.jsonl'), path.join(DATASET_DIR, 'groundedness.jsonl')),
    copyIfMissing(path.join(PACKAGE_ROOT, 'fixtures', 'retrieval.fixture.json'), path.join(ROOT, 'fixtures', 'retrieval.fixture.json')),
    copyIfMissing(path.join(PACKAGE_ROOT, 'fixtures', 'tool-selection.fixture.json'), path.join(ROOT, 'fixtures', 'tool-selection.fixture.json')),
    copyIfMissing(path.join(PACKAGE_ROOT, 'baseline.json'), BASELINE_PATH),
    copyIfMissing(path.join(PACKAGE_ROOT, 'harness.config.example.ts'), path.join(ROOT, 'harness.config.example.ts')),
  ].filter(Boolean).length;

  console.log(copied === 0 ? 'EvalGate already initialized.' : `EvalGate initialized ${copied} file(s).`);
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
    { name: 'groundedness', run: () => groundednessSuite() },
  ].filter((s) => !only || s.name === only);

  if (all.length === 0) {
    console.error(`Unknown suite "${only}". Available: retrieval, tools, groundedness.`);
    process.exit(2);
  }

  const suites: SuiteResult[] = [];
  for (const s of all) suites.push(await s.run());

  const report: RunReport = {
    startedAt: new Date().toISOString(),
    gitSha: gitSha(),
    mode,
    judgeModel: process.env.OPENAI_API_KEY ? (process.env.EVAL_JUDGE_MODEL ?? DEFAULT_JUDGE_MODEL) : null,
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
