#!/usr/bin/env bash
# Release trackline: ./scripts/release.sh 0.11.0
#
# The order matters. The release commit names platform packages at a version
# npm does not have yet, so package-lock.json cannot match it until they are
# published, and `npm ci` on a strict npm (Node 24's) refuses the mismatch.
# Pushed to main as it was, that commit failed CI on every release.
#
# So: the tag goes first, alone, and the Release workflow publishes from it.
# main is pushed only once the lockfile records what was published, and CI
# only ever runs on a commit whose lockfile matches.
#
# The changelog entry for the version must already be in docs/CHANGELOG.md.
set -euo pipefail

v="${1:?usage: scripts/release.sh VERSION (e.g. 0.11.0)}"
[[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "not a version: $v" >&2; exit 1; }
cd "$(dirname "$0")/.."

[ -z "$(git status --porcelain)" ] || { echo "the working tree is not clean" >&2; exit 1; }
[ "$(git branch --show-current)" = main ] || { echo "release from main" >&2; exit 1; }
grep -q "^## $v " docs/CHANGELOG.md || { echo "docs/CHANGELOG.md has no entry for $v" >&2; exit 1; }
old=$(node -p "require('./package.json').version")

# 1. The release commit: every version in step.
node - "$v" <<'EOF'
const fs = require('fs');
const v = process.argv[2];
const edit = (f, fn) => { const j = JSON.parse(fs.readFileSync(f, 'utf8')); fn(j); fs.writeFileSync(f, JSON.stringify(j, null, 2) + '\n'); };
edit('package.json', (j) => {
  j.version = v;
  for (const k of Object.keys(j.optionalDependencies ?? {})) if (k.startsWith('@trackline/')) j.optionalDependencies[k] = v;
});
for (const d of fs.readdirSync('packages')) edit(`packages/${d}/package.json`, (j) => { j.version = v; });
edit('package-lock.json', (j) => { j.version = v; j.packages[''].version = v; });
EOF
git add package.json package-lock.json packages/*/package.json
git commit -q -m "Release $v"
git tag "v$v"

# 2. The tag alone: the Release workflow publishes with provenance.
git push -q origin "v$v"
echo "tag v$v pushed; waiting for npm to have it (the Release workflow publishes)"
# Every package, not a sample: npm leaves an optional package it cannot find
# yet out of the lockfile without a word (0.11.0 lost darwin-x64 that way).
pkgs="trackline $(node -p "Object.keys(require('./package.json').optionalDependencies).join(' ')")"
published() { for p in $pkgs; do [ "$(npm view "$p@$v" version 2>/dev/null)" = "$v" ] || return 1; done; }
for _ in $(seq 1 60); do published && break; sleep 15; done
published || {
  echo "not every package of $v is on npm after 15 minutes. Check the Release workflow; main was not pushed." >&2
  exit 1
}

# 3. The lockfile, now that there is something to record, then main.
npm install --package-lock-only --ignore-scripts >/dev/null
node - "$v" <<'EOF2'
const lock = require('./package-lock.json');
const want = Object.keys(require('./package.json').optionalDependencies);
const missing = want.filter((p) => lock.packages[`node_modules/${p}`]?.version !== process.argv[2]);
if (missing.length) { console.error(`the lockfile is missing ${missing.join(', ')}; main was not pushed`); process.exit(1); }
EOF2
git add package-lock.json
git commit -q -m "Record the published $v platform packages in the lockfile"
git push -q origin main
echo "released $v (was $old); main pushed with a lockfile that matches"
