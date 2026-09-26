import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { validateProject } from '../src/validate.js';
import { multiTurnInputHash, retrievalInputHash, toolInputHash } from '../src/adapters/fixture.js';

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
    path.join(root, 'datasets', 'multi-turn.jsonl'),
    '{"id":"mt-1","turns":[{"utterance":"remember this","available":["kb_search"],"expected":[],"maxRisk":"read_public"},{"utterance":"now answer","available":["kb_search"],"expected":["kb_search"],"maxRisk":"read_public","dependsOnPrevious":true}]}\n',
  );
  fs.writeFileSync(
    path.join(root, 'datasets', 'groundedness.jsonl'),
    '{"id":"gnd-1","question":"q","context":["c"],"answer":"a","expect":"grounded"}\n',
  );
  fs.writeFileSync(
    path.join(root, 'fixtures', 'retrieval.fixture.json'),
    JSON.stringify({
      'ret-1': { retrieved: ['doc#0'], inputHash: retrievalInputHash({ id: 'ret-1', query: 'q', relevant: ['doc#0'] }) },
    }) + '\n',
  );
  // `risks: {}` means resolved-and-nothing-privileged, which is a measurement.
  // Omitting the key means unresolved, which is the hole `risk_unresolvable`
  // exists to catch.
  fs.writeFileSync(
    path.join(root, 'fixtures', 'tool-selection.fixture.json'),
    JSON.stringify({
      'tool-1': {
        called: [],
        refused: false,
        risks: {},
        inputHash: toolInputHash({ id: 'tool-1', utterance: 'hello', available: ['kb_search'], expected: [] }),
      },
    }) + '\n',
  );
  fs.writeFileSync(
    path.join(root, 'fixtures', 'multi-turn.fixture.json'),
    JSON.stringify({
      'mt-1': {
        turns: [
          { called: [], refused: false, risks: {} },
          { called: ['kb_search'], refused: false, risks: { kb_search: 'read_public' } },
        ],
        inputHash: multiTurnInputHash({
          id: 'mt-1',
          turns: [
            { utterance: 'remember this', available: ['kb_search'], expected: [], maxRisk: 'read_public' },
            {
              utterance: 'now answer',
              available: ['kb_search'],
              expected: ['kb_search'],
              maxRisk: 'read_public',
              dependsOnPrevious: true,
            },
          ],
        }),
      },
    }) + '\n',
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
      'multi-turn.exactMatch': 1,
      'multi-turn.f1': 1,
      'multi-turn.forbiddenRate': 0,
      'multi-turn.riskViolationRate': 0,
      'multi-turn.injectionResistance': 1,
      'multi-turn.memorySafety': 1,
      'multi-turn.p95Latency': 0,
    }),
  );
  return root;
}

describe('project validation', () => {
  it('passes a complete fixture project', () => {
    const issues = validateProject(makeProject(), { mode: 'fixture', strictBaseline: true });
    assert.deepEqual(issues, []);
  });

  it('passes an empty project scaffold', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'trackline-empty-'));
    fs.mkdirSync(path.join(root, 'datasets'));
    fs.mkdirSync(path.join(root, 'fixtures'));
    for (const file of ['retrieval.jsonl', 'tool-selection.jsonl', 'multi-turn.jsonl', 'groundedness.jsonl']) {
      fs.writeFileSync(path.join(root, 'datasets', file), '// add rows here\n');
    }
    fs.writeFileSync(path.join(root, 'fixtures', 'retrieval.fixture.json'), '{}\n');
    fs.writeFileSync(path.join(root, 'fixtures', 'tool-selection.fixture.json'), '{}\n');
    fs.writeFileSync(path.join(root, 'fixtures', 'multi-turn.fixture.json'), '{}\n');
    fs.writeFileSync(path.join(root, 'baseline.json'), '{}\n');

    assert.deepEqual(validateProject(root, { mode: 'fixture', strictBaseline: true }), []);
  });

  it('passes a project from before multi-turn existed, even with a strict baseline', () => {
    const root = makeProject();
    fs.rmSync(path.join(root, 'datasets', 'multi-turn.jsonl'));
    fs.rmSync(path.join(root, 'fixtures', 'multi-turn.fixture.json'));
    const baselinePath = path.join(root, 'baseline.json');
    const baseline = JSON.parse(fs.readFileSync(baselinePath, 'utf8')) as Record<string, number>;
    for (const key of Object.keys(baseline)) if (key.startsWith('multi-turn.')) delete baseline[key];
    fs.writeFileSync(baselinePath, JSON.stringify(baseline));

    assert.deepEqual(validateProject(root, { mode: 'fixture', strictBaseline: true }), []);
  });

  it('still requires multi-turn fixtures once the dataset exists', () => {
    const root = makeProject();
    fs.rmSync(path.join(root, 'fixtures', 'multi-turn.fixture.json'));
    const issues = validateProject(root, { mode: 'fixture' });
    assert.ok(issues.some((i) => i.level === 'error' && i.file === 'fixtures/multi-turn.fixture.json'));
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

describe('fixture freshness', () => {
  // The failure this exists to catch: edit a row's inputs, keep its id, and the
  // stale recording replays while scoring a question the row no longer asks.
  it('errors when a retrieval row was edited after its fixture was recorded', () => {
    const root = makeProject();
    fs.writeFileSync(
      path.join(root, 'datasets', 'retrieval.jsonl'),
      '{"id":"ret-1","query":"a completely different question","relevant":["doc#0"]}\n',
    );

    const found = validateProject(root, { mode: 'fixture' }).find(
      (i) => i.code === 'stale_fixture_inputs',
    );
    assert.ok(found, 'expected stale_fixture_inputs');
    assert.equal(found?.level, 'error');
    assert.equal(found?.id, 'ret-1');
  });

  it('errors when a tool row utterance was edited after recording', () => {
    const root = makeProject();
    fs.writeFileSync(
      path.join(root, 'datasets', 'tool-selection.jsonl'),
      '{"id":"tool-1","utterance":"something else entirely","available":["kb_search"],"expected":[],"maxRisk":"read_public"}\n',
    );

    const found = validateProject(root, { mode: 'fixture' }).find(
      (i) => i.code === 'stale_fixture_inputs',
    );
    assert.equal(found?.level, 'error');
    assert.equal(found?.id, 'tool-1');
  });

  it('errors when a multi-turn utterance was edited after recording', () => {
    const root = makeProject();
    fs.writeFileSync(
      path.join(root, 'datasets', 'multi-turn.jsonl'),
      '{"id":"mt-1","turns":[{"utterance":"remember a different constraint","available":["kb_search"],"expected":[],"maxRisk":"read_public"},{"utterance":"now answer","available":["kb_search"],"expected":["kb_search"],"maxRisk":"read_public","dependsOnPrevious":true}]}\n',
    );

    const found = validateProject(root, { mode: 'fixture' }).find(
      (i) => i.code === 'stale_fixture_inputs',
    );
    assert.equal(found?.level, 'error');
    assert.equal(found?.id, 'mt-1');
  });

  it('does not fire when only the expected answers changed', () => {
    // Editing the answer key changes how a recording is scored. It does not make
    // the recording untrue, so it must not demand a re-record.
    const root = makeProject();
    fs.writeFileSync(
      path.join(root, 'datasets', 'retrieval.jsonl'),
      '{"id":"ret-1","query":"q","relevant":["doc#0","doc#1"],"grades":{"doc#0":3}}\n',
    );

    const codes = validateProject(root, { mode: 'fixture' }).map((i) => i.code);
    assert.ok(!codes.includes('stale_fixture_inputs'));
  });

  it('warns rather than fails on fixtures recorded before hashing existed', () => {
    const root = makeProject();
    fs.writeFileSync(path.join(root, 'fixtures', 'retrieval.fixture.json'), '{"ret-1":["doc#0"]}\n');

    const issues = validateProject(root, { mode: 'fixture' });
    const found = issues.find((i) => i.code === 'unhashed_fixture');
    assert.equal(found?.level, 'warning');
    assert.ok(!issues.some((i) => i.code === 'stale_fixture_inputs'));
  });

  it('still accepts the pre-hash bare-array retrieval format', () => {
    const root = makeProject();
    fs.writeFileSync(path.join(root, 'fixtures', 'retrieval.fixture.json'), '{"ret-1":["doc#0"]}\n');

    const codes = validateProject(root, { mode: 'fixture' }).map((i) => i.code);
    assert.ok(!codes.includes('invalid_retrieval_fixture'));
  });
});
