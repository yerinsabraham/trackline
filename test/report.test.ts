import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { findRegressions, toBaseline } from '../src/report.js';
import type { RunReport } from '../src/types.js';

const cases = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `case-${i}`, passed: true, scores: {} }));

const report = (value: number, n = 0, sampleSize?: number, mode: RunReport['mode'] = 'fixture', suite = 'retrieval'): RunReport => ({
  startedAt: '2026-01-01T00:00:00.000Z',
  gitSha: 'abc123',
  mode,
  judgeModel: null,
  suites: [
    {
      suite,
      cases: cases(n),
      errors: [],
      metrics: [
        { key: 'recall@5', value, primary: true, higherIsBetter: true, sampleSize },
        { key: 'emptyRate', value: 0, primary: false, higherIsBetter: false },
      ],
    },
  ],
});

describe('gate regressions', () => {
  it('ignores new metrics by default', () => {
    assert.deepEqual(findRegressions(report(0.9), {}), []);
  });

  it('can fail when a primary metric has no baseline', () => {
    const regressions = findRegressions(report(0.9), {}, { strictBaseline: true });
    assert.equal(regressions.length, 1);
    assert.equal(regressions[0]?.kind, 'missing-baseline');
    assert.equal(regressions[0]?.key, 'retrieval.recall@5');
  });

  it('fails quality metrics that drift beyond tolerance', () => {
    const regressions = findRegressions(report(0.8), { 'retrieval.recall@5': 0.9 });
    assert.equal(regressions.length, 1);
    assert.equal(regressions[0]?.kind, 'tolerance');
  });

  it('does not hide a one-case regression on a small dataset', () => {
    const regressions = findRegressions(report(0.92, 25, 25), { 'retrieval.recall@5': 0.96 });
    assert.equal(regressions.length, 1);
    assert.equal(regressions[0]?.kind, 'tolerance');
  });

  it('uses the metric denominator when it differs from the suite size', () => {
    const regressions = findRegressions(report(0.75, 100, 20), { 'retrieval.recall@5': 0.8 });
    assert.equal(regressions.length, 1);
    assert.equal(regressions[0]?.kind, 'tolerance');
  });

  it('keeps the full tolerance for live runs, where a row can flip on noise', () => {
    assert.deepEqual(findRegressions(report(0.995, 200, 200, 'live'), { 'retrieval.recall@5': 1 }), []);
    assert.deepEqual(findRegressions(report(0.92, 25, 25, 'live'), { 'retrieval.recall@5': 0.96 }), []);
  });

  it('keeps the full tolerance for the judge even in fixture mode', () => {
    assert.deepEqual(findRegressions(report(0.92, 25, 25, 'fixture', 'groundedness'), { 'groundedness.recall@5': 0.96 }), []);
  });

  it('still fails a live run that drifts beyond the full tolerance', () => {
    assert.equal(findRegressions(report(0.9, 200, 200, 'live'), { 'retrieval.recall@5': 0.96 }).length, 1);
  });
});

// ── Not measured is not zero ──────────────────────────────────────────────────

const toolsReport = (metrics: RunReport['suites'][number]['metrics']): RunReport => ({
  startedAt: '2026-01-01T00:00:00.000Z',
  gitSha: 'abc123',
  mode: 'fixture',
  judgeModel: null,
  suites: [{ suite: 'tools', cases: [], errors: [], metrics }],
});

describe('unmeasured metrics', () => {
  it('fails the gate when a safety metric could not be measured', () => {
    const regressions = findRegressions(
      toolsReport([
        {
          key: 'riskViolationRate',
          value: null,
          unmeasured: 'no risk tiers could be resolved',
          unmeasuredIsError: true,
          primary: true,
          higherIsBetter: false,
        },
      ]),
      {},
    );
    assert.equal(regressions.length, 1);
    assert.equal(regressions[0]?.kind, 'unmeasured');
    assert.equal(regressions[0]?.current, null);
  });

  it('stays silent when a safety metric simply does not apply', () => {
    const regressions = findRegressions(
      toolsReport([
        {
          key: 'riskViolationRate',
          value: null,
          unmeasured: 'no dataset row declares maxRisk',
          unmeasuredIsError: false,
          primary: true,
          higherIsBetter: false,
        },
      ]),
      {},
    );
    assert.deepEqual(regressions, []);
  });

  it('never lets an unmeasured metric into the baseline', () => {
    const b = toBaseline(
      toolsReport([
        { key: 'exactMatch', value: 0.9, primary: true, higherIsBetter: true },
        {
          key: 'riskViolationRate',
          value: null,
          unmeasured: 'nothing to measure',
          primary: true,
          higherIsBetter: false,
        },
      ]),
    );
    assert.deepEqual(b, { 'tools.exactMatch': 0.9 });
  });

  it('does not compare an unmeasured quality metric against its old baseline', () => {
    // The dangerous reading: a metric that used to be 0.95 and is now absent
    // must not be scored as a drop to zero.
    const regressions = findRegressions(
      toolsReport([
        {
          key: 'injectionResistance',
          value: null,
          unmeasured: 'no dataset row sets expectRefusal',
          primary: true,
          higherIsBetter: true,
        },
      ]),
      { 'tools.injectionResistance': 0.95 },
    );
    assert.deepEqual(regressions, []);
  });
});
