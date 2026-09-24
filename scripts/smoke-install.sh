#!/bin/bash
# Installs trackline the way a user would, from freshly built packages, and runs it.
#
# The platform binaries under packages/*/bin are gitignored build output, so a
# manual check can quietly run yesterday's binary: they were found stale during
# a Phase 1-4 review, still passing because the old build was not broken, just
# old. This rebuilds everything first, then does what the release workflow
# does: pack, install into an empty directory, run the installed CLI.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

npm run build >/dev/null
./scripts/build-binaries.sh

platform=$(node -p 'process.platform + "-" + process.arch')
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

npm pack --pack-destination "$work" >/dev/null
(cd "packages/trackline-$platform" && npm pack --pack-destination "$work" >/dev/null)

mkdir "$work/app" && cd "$work/app"
npm init -y >/dev/null
npm install "$work"/*.tgz >/dev/null 2>&1

./node_modules/.bin/trackline --help 2>&1 | grep -q "alignment layer" \
  || { echo "the installed CLI did not run"; exit 1; }
./node_modules/.bin/trackline doctor
echo
echo "installed from fresh packages for $platform and ran"
