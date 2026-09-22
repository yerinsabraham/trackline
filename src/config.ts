/**
 * Finding and loading `harness.config.*`.
 *
 * Its own module because both the runner and `doctor` need it, and putting it
 * in either would make them import each other.
 */

import fs from 'node:fs';
import path from 'node:path';

/**
 * Config file names, in the order they are looked for.
 *
 * `.ts` is first because it is what most people will write. It is also the only
 * one of these that can fail to load, which is the whole reason this module
 * exists.
 */
export const CONFIG_NAMES = [
  'harness.config.ts',
  'harness.config.mts',
  'harness.config.mjs',
  'harness.config.js',
] as const;

/** The first config file present in `root`, or null. */
export function findConfigFile(root: string): string | null {
  for (const name of CONFIG_NAMES) {
    const file = path.join(root, name);
    if (fs.existsSync(file)) return file;
  }
  return null;
}

/** True when the config file is TypeScript and therefore needs type stripping. */
export function isTypeScriptConfig(file: string): boolean {
  return /\.m?ts$/.test(file);
}

/**
 * Can this Node import a TypeScript file directly?
 *
 * Node exposes `process.features.typescript` from v22.10: `false` when type
 * stripping is unavailable, `'strip'` or `'transform'` when it is. It is
 * `undefined` on older versions, including Node 20, which is falsy and
 * therefore correct.
 */
export function nodeCanLoadTypeScript(): boolean {
  return Boolean((process.features as { typescript?: false | string }).typescript);
}

/**
 * What to do about a TypeScript config this Node cannot load.
 *
 * Shared by the runner and `doctor` so the advice never drifts between the
 * error you hit and the warning that predicted it.
 */
export function typeScriptConfigAdvice(file: string): string {
  return (
    `${path.basename(file)} is TypeScript, and this Node (${process.version}) cannot import it directly.\n` +
    'Pick one:\n' +
    '  - Run on Node 22.18 or newer, which strips types by default.\n' +
    '  - On Node 22.6 to 22.17, pass --experimental-strip-types.\n' +
    '  - Rename the config to harness.config.mjs and use plain JavaScript.\n' +
    '  - Run through a loader, for example: npx tsx node_modules/.bin/trackline run --live'
  );
}
