import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { validateProject } from '../src/validate.js';

function makeProject(): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'trackline-'));
  fs.mkdirSync(path.join(root, 'datasets'));
  fs.mkdirSync(path.join(root, 'fixtures'));
  fs.writeFileSync(
    path.join(root, 'datasets', 'retrieval.jsonl'),
    '{"id":"ret-1","query":"q","relevant":["doc#0"]}\n',
  );
  fs.writeFileSync(
    path.join(root, 'datasets', 'tool-selection.jsonl'),
    '{"id":"tool-1","utterance":"hello","available":["kb_search"],"expected":[],"maxRisk":"read_public"}\n',
  );
  fs.writeFileSync(
    path.join(root, 'datasets', 'groundedness.jsonl'),
    '{"id":"gnd-1","question":"q","context":["c"],"answer":"a","expect":"grounded"}\n',
  );
  fs.writeFileSync(path.join(root, 'fixtures', 'retrieval.fixture.json'), '{"ret-1":["doc#0"]}\n');
  fs.writeFileSync(path.join(root, 'fixtures', 'tool-selection.fixture.json'), '{"tool-1":{"called":[],"refused":false}}\n');
  fs.writeFileSync(
    path.join(root, 'baseline.json'),
    JSON.stringify({
      'retrieval.recall@5': 1,
      'retrieval.recall@10': 1,
      'retrieval.mrr': 1,
      'retrieval.ndcg@10': 1,
      'retrieval.precision@5': 1,
      'retrieval.emptyRate': 0,
      'retrieval.p95Latency': 0,
      'tools.exactMatch': 1,
      'tools.f1': 1,
      'tools.forbiddenRate': 0,
      'tools.riskViolationRate': 0,
      'tools.injectionResistance': 1,
      'tools.p95Latency': 0,
    }),
  );
  return root;
}

describe('project validation', () => {
  it('passes a complete fixture project', () => {
    const issues = validateProject(makeProject(), { mode: 'fixture', strictBaseline: true });
    assert.deepEqual(issues, []);
  });

  it('fails missing fixture coverage in fixture mode', () => {
    const root = makeProject();
    fs.writeFileSync(path.join(root, 'fixtures', 'retrieval.fixture.json'), '{}\n');
    const issues = validateProject(root, { mode: 'fixture' });
    assert.ok(issues.some((i) => i.level === 'error' && i.code === 'missing_fixture'));
  });

  it('does not require fixtures in live mode', () => {
    const root = makeProject();
    fs.rmSync(path.join(root, 'fixtures'), { recursive: true, force: true });
    const issues = validateProject(root, { mode: 'live' });
    assert.equal(issues.some((i) => i.level === 'error' && i.code === 'missing_file'), false);
  });
});
