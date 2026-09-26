import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const EXAMPLE = path.join(ROOT, 'examples', 'fintech-support');

// The repo gates itself on the fintech sample at the root, and
// `init --example=fintech-support` ships the copy under examples/. Two copies
// drift silently: the repo's CI stays green while users get stale rows.
const SHARED = [
  'datasets/retrieval.jsonl',
  'datasets/tool-selection.jsonl',
  'datasets/multi-turn.jsonl',
  'datasets/groundedness.jsonl',
  'fixtures/retrieval.fixture.json',
  'fixtures/tool-selection.fixture.json',
  'fixtures/multi-turn.fixture.json',
  'baseline.json',
];

describe('fintech-support example', () => {
  for (const file of SHARED) {
    it(`matches the root copy of ${file}`, () => {
      assert.equal(
        fs.readFileSync(path.join(EXAMPLE, file), 'utf8'),
        fs.readFileSync(path.join(ROOT, file), 'utf8'),
        `${file} differs between the repo root and examples/fintech-support; copy the one you meant to change`,
      );
    });
  }
});
