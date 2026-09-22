/**
 * Config discovery.
 *
 * The bug these exist for: `npx trackline run --live` threw
 * ERR_UNKNOWN_FILE_EXTENSION out of Node internals whenever the config was
 * TypeScript and Node could not strip types — which is every Node 20, the
 * version the project's own CI pinned and never exercised on this path.
 */

import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import {
  CONFIG_NAMES,
  findConfigFile,
  isTypeScriptConfig,
  nodeCanLoadTypeScript,
  typeScriptConfigAdvice,
} from '../src/config.js';

const tmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'trackline-cfg-'));

describe('config discovery', () => {
  it('finds nothing in an empty project', () => {
    assert.equal(findConfigFile(tmp()), null);
  });

  it('finds a JavaScript config, which older Node can always load', () => {
    const root = tmp();
    fs.writeFileSync(path.join(root, 'harness.config.mjs'), 'export default {};\n');
    assert.equal(findConfigFile(root), path.join(root, 'harness.config.mjs'));
  });

  it('prefers TypeScript when both are present, matching the documented order', () => {
    const root = tmp();
    fs.writeFileSync(path.join(root, 'harness.config.mjs'), 'export default {};\n');
    fs.writeFileSync(path.join(root, 'harness.config.ts'), 'export default {};\n');
    assert.equal(findConfigFile(root), path.join(root, 'harness.config.ts'));
  });

  it('recognises which config extensions need type stripping', () => {
    assert.equal(isTypeScriptConfig('/x/harness.config.ts'), true);
    assert.equal(isTypeScriptConfig('/x/harness.config.mts'), true);
    assert.equal(isTypeScriptConfig('/x/harness.config.mjs'), false);
    assert.equal(isTypeScriptConfig('/x/harness.config.js'), false);
  });

  it('offers a loadable extension for every unloadable one', () => {
    // If every supported name needed type stripping there would be no way out
    // of the failure, and the advice would be a dead end.
    assert.ok(CONFIG_NAMES.some((n) => !isTypeScriptConfig(n)));
  });

  it('reports type-stripping support as a boolean, whatever Node reports', () => {
    // process.features.typescript is false, 'strip', 'transform', or undefined
    // on Node before 22.10. All four must collapse to a usable boolean.
    assert.equal(typeof nodeCanLoadTypeScript(), 'boolean');
  });

  it('names a concrete next step rather than restating the failure', () => {
    const advice = typeScriptConfigAdvice('/x/harness.config.ts');
    assert.match(advice, /harness\.config\.ts/);
    assert.match(advice, /22\.18/);
    assert.match(advice, /harness\.config\.mjs/);
    assert.match(advice, /tsx/);
    assert.ok(!advice.includes('ERR_UNKNOWN_FILE_EXTENSION'));
  });
});
