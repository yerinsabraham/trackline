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
  // `risks: {}` means resolved-and-nothing-privileged, which is a measurement.
  // Omitting the key means unresolved, which is the hole `risk_unresolvable`
  // exists to catch.
  fs.writeFileSync(
    path.join(root, 'fixtures', 'tool-selection.fixture.json'),
    '{"tool-1":{"called":[],"refused":false,"risks":{}}}\n',
  );
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

describe('risk tier coverage', () => {
  it('errors when a maxRisk row has no resolvable tiers and no config to fall back on', () => {
    const root = makeProject();
    // A fixture with no `risks` key: recorded before tiers were stored, or
    // recorded without a catalog.
    fs.writeFileSync(
      path.join(root, 'fixtures', 'tool-selection.fixture.json'),
      '{"tool-1":{"called":[],"refused":false}}\n',
    );

    const issues = validateProject(root, { mode: 'fixture' });
    const found = issues.find((i) => i.code === 'risk_unresolvable');
    assert.ok(found, 'expected risk_unresolvable');
    assert.equal(found?.level, 'error');
  });

  it('downgrades to a warning when a harness config can still resolve tiers', () => {
    const root = makeProject();
    fs.writeFileSync(
      path.join(root, 'fixtures', 'tool-selection.fixture.json'),
      '{"tool-1":{"called":[],"refused":false}}\n',
    );
    fs.writeFileSync(path.join(root, 'harness.config.ts'), 'export default {};\n');

    const found = validateProject(root, { mode: 'fixture' }).find((i) => i.code === 'risk_unresolvable');
    assert.equal(found?.level, 'warning');
  });

  it('stays silent when no row declares maxRisk', () => {
    const root = makeProject();
    fs.writeFileSync(
      path.join(root, 'datasets', 'tool-selection.jsonl'),
      '{"id":"tool-1","utterance":"hello","available":["kb_search"],"expected":[]}\n',
    );
    fs.writeFileSync(
      path.join(root, 'fixtures', 'tool-selection.fixture.json'),
      '{"tool-1":{"called":[],"refused":false}}\n',
    );

    const codes = validateProject(root, { mode: 'fixture' }).map((i) => i.code);
    assert.ok(!codes.includes('risk_unresolvable'));
  });
});
