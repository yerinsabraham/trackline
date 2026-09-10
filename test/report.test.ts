import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { findRegressions } from '../src/report.js';
import type { RunReport } from '../src/types.js';

const report = (value: number): RunReport => ({
  startedAt: '2026-01-01T00:00:00.000Z',
  gitSha: 'abc123',
  mode: 'fixture',
  judgeModel: null,
  suites: [
    {
      suite: 'retrieval',
      cases: [],
      errors: [],
      metrics: [
        { key: 'recall@5', value, primary: true, higherIsBetter: true },
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
});
