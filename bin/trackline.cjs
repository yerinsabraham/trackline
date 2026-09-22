#!/usr/bin/env node
/**
 * Finds and runs the trackline binary for this platform.
 *
 * The real command is a compiled Go program, and this shim exists only because
 * npm is how people expect to install a developer tool. It does as little as
 * possible: resolve the binary, hand over, get out of the way.
 *
 * A .cjs file, not .js: this package is ESM, so a .js shim using require fails
 * the moment anyone installs it. That is not visible from the repository and
 * only shows up on a real install, which is why one is run before release.
 *
 * Binaries arrive as optional dependencies, one package per platform, so npm
 * installs only the one that matches. Nobody downloads five platforms' worth to
 * use one, and there is no postinstall script fetching anything over the
 * network — which would fail offline, fail behind a proxy, and sit badly in a
 * tool whose subject is supply-chain caution.
 */
"use strict";

const { spawnSync } = require("node:child_process");
const path = require("node:path");

const PLATFORMS = ["darwin-arm64", "darwin-x64", "linux-x64", "linux-arm64", "win32-x64"];

/** Resolves a binary from its platform package, or null if that package is absent. */
function find(name) {
  const key = `${process.platform}-${process.arch}`;
  if (!PLATFORMS.includes(key)) return { error: "unsupported", key };

  const ext = process.platform === "win32" ? ".exe" : "";
  try {
    const pkg = require.resolve(`trackline-${key}/package.json`);
    return { binary: path.join(path.dirname(pkg), "bin", `${name}${ext}`) };
  } catch {
    return { error: "missing", key };
  }
}

function run(name, args) {
  const found = find(name);

  if (found.error === "unsupported") {
    console.error(
      `trackline has no binary for ${found.key}.\n\n` +
        `Supported: ${PLATFORMS.join(", ")}.\n` +
        `The engine is Go and builds anywhere, so this is a packaging gap rather\n` +
        `than a limitation: https://github.com/yerinsabraham/trackline`,
    );
    process.exit(1);
  }
  if (found.error === "missing") {
    console.error(
      `trackline is installed but the binary package for ${found.key} is not.\n\n` +
        `This usually means the install skipped optional dependencies. Try:\n` +
        `  npm install trackline-${found.key}\n\n` +
        `If that fails, please report it:\n` +
        `  https://github.com/yerinsabraham/trackline/issues`,
    );
    process.exit(1);
  }

  const res = spawnSync(found.binary, args, { stdio: "inherit" });
  if (res.error) {
    console.error(`trackline could not run: ${res.error.message}`);
    process.exit(1);
  }
  // The hook's exit code is a decision, not a status: 2 means block. It has to
  // survive this shim untouched.
  process.exit(res.status === null ? 1 : res.status);
}

module.exports = { run, find, PLATFORMS };

if (require.main === module) run("trackline", process.argv.slice(2));
