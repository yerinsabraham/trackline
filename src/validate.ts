import fs from 'node:fs';
import path from 'node:path';

import type { MultiTurnCase, RetrievalCase, RiskTier, ToolSelectionCase } from './types.js';
import { multiTurnInputHash, readRetrievalFixture, retrievalInputHash, toolInputHash } from './adapters/fixture.js';
import {
  CONFIG_NAMES,
  findConfigFile,
  isTypeScriptConfig,
  nodeCanLoadTypeScript,
  typeScriptConfigAdvice,
} from './config.js';

export type ValidationLevel = 'error' | 'warning';

export interface ValidationIssue {
  level: ValidationLevel;
  code: string;
  message: string;
  file?: string;
  id?: string;
}

export interface ValidationOptions {
  mode?: 'fixture' | 'live';
  strictBaseline?: boolean;
}

const RISK_TIERS = new Set<RiskTier>([
  'read_public',
  'read_private',
  'safe_write',
  'customer_confirm',
  'step_up',
  'admin_approve',
  'human_only',
]);

const EXPECTED_BASELINE_KEYS = [
  'retrieval.recall@5',
  'retrieval.recall@10',
  'retrieval.mrr',
  'retrieval.ndcg@10',
  'retrieval.precision@5',
  'retrieval.emptyRate',
  'retrieval.p95Latency',
  'tools.exactMatch',
  'tools.f1',
  'tools.forbiddenRate',
  'tools.riskViolationRate',
  'tools.injectionResistance',
  'tools.p95Latency',
  'multi-turn.exactMatch',
  'multi-turn.f1',
  'multi-turn.forbiddenRate',
  'multi-turn.riskViolationRate',
  'multi-turn.injectionResistance',
  'multi-turn.memorySafety',
  'multi-turn.p95Latency',
];

const DATASET_FILES = {
  retrieval: 'retrieval.jsonl',
  tools: 'tool-selection.jsonl',
  multiTurn: 'multi-turn.jsonl',
  groundedness: 'groundedness.jsonl',
} as const;

const FIXTURE_FILES = {
  retrieval: 'retrieval.fixture.json',
  tools: 'tool-selection.fixture.json',
  multiTurn: 'multi-turn.fixture.json',
} as const;

function issue(
  issues: ValidationIssue[],
  level: ValidationLevel,
  code: string,
  message: string,
  file?: string,
  id?: string,
): void {
  issues.push({ level, code, message, file, id });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((v) => typeof v === 'string');
}

function readJsonl(root: string, file: string, issues: ValidationIssue[]): Record<string, unknown>[] {
  const full = path.join(root, 'datasets', file);
  if (!fs.existsSync(full)) {
    issue(issues, 'error', 'missing_dataset', `Missing dataset file ${file}.`, `datasets/${file}`);
    return [];
  }

  const rows: Record<string, unknown>[] = [];
  const seen = new Set<string>();
  const lines = fs.readFileSync(full, 'utf8').split('\n');

  lines.forEach((line, i) => {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('//')) return;

    let parsed: unknown;
    try {
      parsed = JSON.parse(trimmed);
    } catch (e) {
      issue(
        issues,
        'error',
        'invalid_jsonl',
        `Line ${i + 1} is not valid JSON: ${(e as Error).message}`,
        `datasets/${file}`,
      );
      return;
    }

    if (!isRecord(parsed)) {
      issue(issues, 'error', 'invalid_row', `Line ${i + 1} must be a JSON object.`, `datasets/${file}`);
      return;
    }

    const id = parsed.id;
    if (typeof id !== 'string' || id.trim() === '') {
      issue(issues, 'error', 'missing_id', `Line ${i + 1} has no string id.`, `datasets/${file}`);
    } else if (seen.has(id)) {
      issue(issues, 'error', 'duplicate_id', `Duplicate id "${id}".`, `datasets/${file}`, id);
    } else {
      seen.add(id);
    }

    rows.push(parsed);
  });

  return rows;
}

function readJsonObject(root: string, file: string, issues: ValidationIssue[]): Record<string, unknown> | null {
  const full = path.join(root, file);
  if (!fs.existsSync(full)) {
    issue(issues, 'error', 'missing_file', `Missing ${file}.`, file);
    return null;
  }

  try {
    const parsed = JSON.parse(fs.readFileSync(full, 'utf8')) as unknown;
    if (!isRecord(parsed)) {
      issue(issues, 'error', 'invalid_json', `${file} must contain a JSON object.`, file);
      return null;
    }
    return parsed;
  } catch (e) {
    issue(issues, 'error', 'invalid_json', `${file} is not valid JSON: ${(e as Error).message}`, file);
    return null;
  }
}

function validateRetrieval(rows: Record<string, unknown>[], issues: ValidationIssue[]): string[] {
  const ids: string[] = [];
  for (const row of rows) {
    const id = typeof row.id === 'string' ? row.id : undefined;
    if (id) ids.push(id);
    const file = `datasets/${DATASET_FILES.retrieval}`;

    if (typeof row.query !== 'string' || row.query.trim() === '') {
      issue(issues, 'error', 'invalid_retrieval_query', 'Retrieval row needs a non-empty query.', file, id);
    }
    if (!isStringArray(row.relevant)) {
      issue(issues, 'error', 'invalid_retrieval_relevant', 'Retrieval row relevant must be a string array.', file, id);
    }
    if (row.grades !== undefined) {
      if (!isRecord(row.grades) || Object.values(row.grades).some((v) => typeof v !== 'number')) {
        issue(issues, 'error', 'invalid_retrieval_grades', 'Retrieval grades must be an object of numbers.', file, id);
      }
    }
    if (row.segments !== undefined && !isStringArray(row.segments)) {
      issue(issues, 'error', 'invalid_retrieval_segments', 'Retrieval segments must be a string array.', file, id);
    }
  }
  return ids;
}

function validateTools(rows: Record<string, unknown>[], issues: ValidationIssue[]): string[] {
  const ids: string[] = [];
  for (const row of rows) {
    const id = typeof row.id === 'string' ? row.id : undefined;
    if (id) ids.push(id);
    const file = `datasets/${DATASET_FILES.tools}`;

    if (typeof row.utterance !== 'string' || row.utterance.trim() === '') {
      issue(issues, 'error', 'invalid_tool_utterance', 'Tool row needs a non-empty utterance.', file, id);
    }
    if (!isStringArray(row.available)) {
      issue(issues, 'error', 'invalid_tool_available', 'Tool row available must be a string array.', file, id);
    }
    if (!isStringArray(row.expected)) {
      issue(issues, 'error', 'invalid_tool_expected', 'Tool row expected must be a string array.', file, id);
    }
    if (row.forbidden !== undefined && !isStringArray(row.forbidden)) {
      issue(issues, 'error', 'invalid_tool_forbidden', 'Tool row forbidden must be a string array.', file, id);
    }
    if (row.maxRisk !== undefined && (typeof row.maxRisk !== 'string' || !RISK_TIERS.has(row.maxRisk as RiskTier))) {
      issue(issues, 'error', 'invalid_risk_tier', `Unknown risk tier "${String(row.maxRisk)}".`, file, id);
    }
    if (row.expectRefusal !== undefined && typeof row.expectRefusal !== 'boolean') {
      issue(issues, 'error', 'invalid_expect_refusal', 'expectRefusal must be boolean when present.', file, id);
    }
  }
  return ids;
}

function validateToolTurn(row: Record<string, unknown>): boolean {
  let ok = true;
  if (typeof row.utterance !== 'string' || row.utterance.trim() === '') ok = false;
  if (!isStringArray(row.available)) ok = false;
  if (!isStringArray(row.expected)) ok = false;
  if (row.forbidden !== undefined && !isStringArray(row.forbidden)) ok = false;
  if (row.maxRisk !== undefined && (typeof row.maxRisk !== 'string' || !RISK_TIERS.has(row.maxRisk as RiskTier))) ok = false;
  if (row.expectRefusal !== undefined && typeof row.expectRefusal !== 'boolean') ok = false;
  if (row.dependsOnPrevious !== undefined && typeof row.dependsOnPrevious !== 'boolean') ok = false;
  return ok;
}

function validateMultiTurn(rows: Record<string, unknown>[], issues: ValidationIssue[]): string[] {
  const ids: string[] = [];
  for (const row of rows) {
    const id = typeof row.id === 'string' ? row.id : undefined;
    if (id) ids.push(id);
    const file = `datasets/${DATASET_FILES.multiTurn}`;

    if (!Array.isArray(row.turns) || row.turns.length < 2) {
      issue(issues, 'error', 'invalid_multi_turn_turns', 'Multi-turn row needs at least two turns.', file, id);
      continue;
    }
    row.turns.forEach((turn, i) => {
      if (!isRecord(turn) || !validateToolTurn(turn)) {
        issue(issues, 'error', 'invalid_multi_turn_turn', `Turn ${i + 1} must match the tool-selection row shape.`, file, id);
      }
    });
  }
  return ids;
}

function validateGroundedness(rows: Record<string, unknown>[], issues: ValidationIssue[]): void {
  for (const row of rows) {
    const id = typeof row.id === 'string' ? row.id : undefined;
    const file = `datasets/${DATASET_FILES.groundedness}`;

    if (typeof row.question !== 'string' || row.question.trim() === '') {
      issue(issues, 'error', 'invalid_groundedness_question', 'Groundedness row needs a non-empty question.', file, id);
    }
    if (!isStringArray(row.context) || row.context.length === 0) {
      issue(issues, 'error', 'invalid_groundedness_context', 'Groundedness context must be a non-empty string array.', file, id);
    }
    if (typeof row.answer !== 'string' || row.answer.trim() === '') {
      issue(issues, 'error', 'invalid_groundedness_answer', 'Groundedness row needs a non-empty answer.', file, id);
    }
    if (row.expect !== 'grounded' && row.expect !== 'unsupported') {
      issue(issues, 'error', 'invalid_groundedness_expect', 'Groundedness expect must be grounded or unsupported.', file, id);
    }
  }
}

function validateFixtureCoverage(
  root: string,
  kind: keyof typeof FIXTURE_FILES,
  ids: string[],
  issues: ValidationIssue[],
  required: boolean,
): void {
  const file = `fixtures/${FIXTURE_FILES[kind]}`;
  if (!required && !fs.existsSync(path.join(root, file))) return;
  const fixtures = readJsonObject(root, file, issues);
  if (!fixtures) return;

  const expected = new Set(ids);
  for (const id of ids) {
    if (!(id in fixtures)) {
      issue(
        issues,
        required ? 'error' : 'warning',
        'missing_fixture',
        `No ${kind} fixture for "${id}".`,
        file,
        id,
      );
    }
  }

  for (const [id, value] of Object.entries(fixtures)) {
    if (!expected.has(id)) {
      issue(issues, 'warning', 'stale_fixture', `${kind} fixture "${id}" has no matching dataset row.`, file, id);
    }

    if (kind === 'retrieval' && readRetrievalFixture(value) === null) {
      issue(
        issues,
        'error',
        'invalid_retrieval_fixture',
        `Retrieval fixture "${id}" must be {retrieved: string[]} (or a bare string array, the pre-hash format).`,
        file,
        id,
      );
    }
    if (kind === 'tools') {
      if (!isRecord(value) || !isStringArray(value.called) || typeof value.refused !== 'boolean') {
        issue(
          issues,
          'error',
          'invalid_tool_fixture',
          `Tool fixture "${id}" must have called:string[] and refused:boolean.`,
          file,
          id,
        );
      }
    }
    if (kind === 'multiTurn') {
      if (!isRecord(value) || !Array.isArray(value.turns) || value.turns.some((turn) => !isRecord(turn) || !isStringArray(turn.called) || typeof turn.refused !== 'boolean')) {
        issue(
          issues,
          'error',
          'invalid_multi_turn_fixture',
          `Multi-turn fixture "${id}" must have turns with called:string[] and refused:boolean.`,
          file,
          id,
        );
      }
    }
  }
}

function validateBaseline(
  root: string,
  issues: ValidationIssue[],
  strict: boolean,
  hasRows: boolean,
  hasMultiTurn: boolean,
): void {
  const file = path.join(root, 'baseline.json');
  if (!fs.existsSync(file)) {
    if (!hasRows) return;
    issue(
      issues,
      strict ? 'error' : 'warning',
      'missing_baseline_file',
      'Missing baseline.json. Run trackline baseline after reviewing current results.',
      'baseline.json',
    );
    return;
  }

  const baseline = readJsonObject(root, 'baseline.json', issues);
  if (!baseline) return;

  for (const [key, value] of Object.entries(baseline)) {
    if (typeof value !== 'number') {
      issue(issues, 'error', 'invalid_baseline_value', `Baseline value "${key}" must be a number.`, 'baseline.json');
    }
  }

  if (!hasRows) return;

  for (const key of EXPECTED_BASELINE_KEYS) {
    if (!hasMultiTurn && key.startsWith('multi-turn.')) continue;
    if (!(key in baseline)) {
      issue(
        issues,
        strict ? 'error' : 'warning',
        'missing_baseline',
        `Baseline is missing ${key}.`,
        'baseline.json',
      );
    }
  }
}

/**
 * Can `riskViolationRate` actually be computed?
 *
 * A dataset row that declares `maxRisk` is asking a safety question. If nothing
 * can resolve a tool's tier, that question goes unanswered, and before this
 * check existed the runner reported a clean `0.000` for it. Catch it here,
 * before the scorer produces a number nobody can stand behind.
 */
function validateRiskCoverage(
  root: string,
  rows: { id?: string; maxRisk?: unknown }[],
  kind: 'tools' | 'multiTurn',
  datasetFile: string,
  issues: ValidationIssue[],
  fixtureMode: boolean,
): void {
  const riskRows = rows.filter((r) => r.maxRisk !== undefined);
  if (riskRows.length === 0) return;

  // Use the shared lookup rather than a local list, or a `.mjs` config is
  // invisible here and the run reports a hole that does not exist.
  const hasConfig = findConfigFile(root) !== null;
  // A live run resolves tiers from the config; a fixture run replays recorded
  // ones, falling back to the config when the fixture predates the field.
  if (!fixtureMode) {
    if (!hasConfig) {
      issue(
        issues,
        'error',
        'risk_unresolvable',
        `${riskRows.length} tool row(s) declare maxRisk but there is no harness config to resolve tiers from. ` +
          `Add one of: ${CONFIG_NAMES.join(', ')}. riskViolationRate cannot be measured.`,
        `datasets/${datasetFile}`,
      );
    }
    return;
  }

  const file = `fixtures/${FIXTURE_FILES[kind]}`;
  if (!fs.existsSync(path.join(root, file))) return;
  const fixtures = readJsonObject(root, file, issues);
  if (!fixtures) return;

  const missing = riskRows
    .map((r) => r.id)
    .filter((id): id is string => Boolean(id))
    .filter((id) => {
      if (kind === 'tools') {
        const fx = fixtures[id];
        return isRecord(fx) && fx.risks === undefined;
      }
      const [caseID, rawTurn] = id.split('.');
      const turn = Number(rawTurn);
      const fx = fixtures[caseID ?? ''];
      return isRecord(fx) && Array.isArray(fx.turns) && isRecord(fx.turns[turn]) && fx.turns[turn].risks === undefined;
    });

  if (missing.length === 0) return;

  // With a config present the runner can still resolve tiers itself, so this is
  // staleness rather than a hole.
  issue(
    issues,
    hasConfig ? 'warning' : 'error',
    'risk_unresolvable',
    `${missing.length} of ${riskRows.length} maxRisk row(s) have fixtures with no recorded risk tiers` +
      (hasConfig
        ? '. They will fall back to the harness config; re-record to store them.'
        : `, and there is no harness config to fall back on. riskViolationRate cannot be measured. First: ${missing.slice(0, 3).join(', ')}`),
    file,
  );
}

/**
 * Has a dataset row been edited since its fixture was recorded?
 *
 * Fixtures key on case id alone, so an edited row with an unchanged id replays
 * a stale recording and scores a question the dataset no longer asks. Nothing
 * else in the harness can see that happen.
 *
 * A missing hash is a pre-hash fixture. It still replays, so this warns rather
 * than failing, but it says plainly that staleness cannot be checked instead of
 * reporting a clean result.
 */
function validateFixtureFreshness(
  root: string,
  kind: keyof typeof FIXTURE_FILES,
  rows: Record<string, unknown>[],
  hashOf: (row: never) => string,
  issues: ValidationIssue[],
  fixtureMode: boolean,
): void {
  if (!fixtureMode) return;
  const file = `fixtures/${FIXTURE_FILES[kind]}`;
  if (!fs.existsSync(path.join(root, file))) return;
  const fixtures = readJsonObject(root, file, issues);
  if (!fixtures) return;

  const unhashed: string[] = [];

  for (const row of rows) {
    const id = typeof row.id === 'string' ? row.id : undefined;
    if (!id) continue;
    const fx = fixtures[id];
    if (fx === undefined) continue; // coverage is a separate check

    const recorded =
      kind === 'retrieval'
        ? readRetrievalFixture(fx)?.inputHash
        : isRecord(fx) && typeof fx.inputHash === 'string'
          ? fx.inputHash
          : undefined;

    if (recorded === undefined) {
      unhashed.push(id);
      continue;
    }

    let expected: string;
    try {
      expected = hashOf(row as never);
    } catch {
      continue; // a malformed row is already reported by the shape checks
    }

    if (recorded !== expected) {
      issue(
        issues,
        'error',
        'stale_fixture_inputs',
        `Row "${id}" was edited after its fixture was recorded. The replay answers a question the row no longer asks. ` +
          'Re-record, read the diff, then update the baseline.',
        file,
        id,
      );
    }
  }

  if (unhashed.length > 0) {
    issue(
      issues,
      'warning',
      'unhashed_fixture',
      `${unhashed.length} ${kind} fixture(s) predate input hashing, so staleness cannot be checked. ` +
        `Re-record to enable it. First: ${unhashed.slice(0, 3).join(', ')}`,
      file,
    );
  }
}

/**
 * Will the harness config actually load on this Node?
 *
 * A TypeScript config is only importable where Node can strip types: not at all
 * on Node 20, and only from 22.18 by default on Node 22. Catching it here means
 * the answer arrives from `doctor` before a live run, rather than as an
 * `ERR_UNKNOWN_FILE_EXTENSION` from Node internals partway through one.
 *
 * A warning in fixture mode, which never reads the config, and an error in live
 * mode, which cannot proceed without it.
 */
function validateConfigLoadable(
  root: string,
  issues: ValidationIssue[],
  fixtureMode: boolean,
): void {
  const file = findConfigFile(root);
  if (!file || !isTypeScriptConfig(file) || nodeCanLoadTypeScript()) return;

  issue(
    issues,
    fixtureMode ? 'warning' : 'error',
    'config_unloadable',
    typeScriptConfigAdvice(file) + (fixtureMode ? '\nFixture mode does not read it, so this run is unaffected.' : ''),
    path.basename(file),
  );
}

export function validateProject(root: string, options: ValidationOptions = {}): ValidationIssue[] {
  const issues: ValidationIssue[] = [];
  const fixtureMode = (options.mode ?? 'fixture') === 'fixture';

  const retrieval = readJsonl(root, DATASET_FILES.retrieval, issues);
  const tools = readJsonl(root, DATASET_FILES.tools, issues);
  // Multi-turn arrived after projects already existed with three datasets. A
  // project without the file has not opted in, which is not an error: the run
  // reports the suite as skipped, and --fail-on-skipped-suite can insist.
  const hasMultiTurn = fs.existsSync(path.join(root, 'datasets', DATASET_FILES.multiTurn));
  const multiTurn = hasMultiTurn ? readJsonl(root, DATASET_FILES.multiTurn, issues) : [];
  const groundedness = readJsonl(root, DATASET_FILES.groundedness, issues);
  const hasRows = retrieval.length > 0 || tools.length > 0 || multiTurn.length > 0 || groundedness.length > 0;

  const retrievalIds = validateRetrieval(retrieval, issues);
  const toolIds = validateTools(tools, issues);
  const multiTurnIds = validateMultiTurn(multiTurn, issues);
  validateGroundedness(groundedness, issues);
  validateFixtureCoverage(root, 'retrieval', retrievalIds, issues, fixtureMode);
  validateFixtureCoverage(root, 'tools', toolIds, issues, fixtureMode);
  if (hasMultiTurn) validateFixtureCoverage(root, 'multiTurn', multiTurnIds, issues, fixtureMode);
  validateRiskCoverage(
    root,
    tools.map((row) => ({ id: typeof row.id === 'string' ? row.id : undefined, maxRisk: row.maxRisk })),
    'tools',
    DATASET_FILES.tools,
    issues,
    fixtureMode,
  );
  validateRiskCoverage(
    root,
    multiTurn.flatMap((row) =>
      Array.isArray(row.turns)
        ? row.turns.map((turn, i) => ({ id: typeof row.id === 'string' ? `${row.id}.${i}` : undefined, maxRisk: isRecord(turn) ? turn.maxRisk : undefined }))
        : [],
    ),
    'multiTurn',
    DATASET_FILES.multiTurn,
    issues,
    fixtureMode,
  );
  validateFixtureFreshness(
    root,
    'retrieval',
    retrieval,
    (row: RetrievalCase) => retrievalInputHash(row),
    issues,
    fixtureMode,
  );
  validateFixtureFreshness(
    root,
    'tools',
    tools,
    (row: ToolSelectionCase) => toolInputHash(row),
    issues,
    fixtureMode,
  );
  validateFixtureFreshness(
    root,
    'multiTurn',
    multiTurn,
    (row: MultiTurnCase) => multiTurnInputHash(row),
    issues,
    fixtureMode,
  );
  validateBaseline(root, issues, Boolean(options.strictBaseline), hasRows, hasMultiTurn);
  validateConfigLoadable(root, issues, fixtureMode);

  return issues;
}

export function renderValidationIssues(issues: ValidationIssue[]): string {
  if (issues.length === 0) return '✓ doctor passed — datasets, fixtures, and baseline are consistent.';

  const lines = ['Trackline doctor found issues:'];
  for (const item of issues) {
    const where = [item.file, item.id].filter(Boolean).join(' ');
    lines.push(`  ${item.level === 'error' ? 'error' : 'warn '} ${item.code}${where ? ` ${where}` : ''}: ${item.message}`);
  }
  return lines.join('\n');
}
