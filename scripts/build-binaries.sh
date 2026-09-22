#!/bin/bash
# Cross-compiles every platform into its own npm package.
#
# All five build from one machine, which is why the engine is Go: no
# per-platform CI matrix, and nothing for a user to install.
#
# Two binaries on purpose. Folding the hook into the main CLI was measured and
# reverted: the larger binary cost 2.6ms of extra startup on every tool call,
# a 19% regression on the one path where this project's whole speed argument
# lives.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

build() {
  local goos="$1" goarch="$2" npmos="$3" npmcpu="$4"
  local out="packages/trackline-$npmos-$npmcpu/bin"
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"

  mkdir -p "$out"
  (cd engine && GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" \
    -o "../$out/trackline$ext" ./cmd/trackline)
  (cd engine && GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" \
    -o "../$out/trackline-hook$ext" ./cmd/hook)
  printf "  %-22s %s\n" "$npmos-$npmcpu" "$(du -sh "$out" | cut -f1)"
}

build darwin  arm64 darwin arm64
build darwin  amd64 darwin x64
build linux   amd64 linux  x64
build linux   arm64 linux  arm64
build windows amd64 win32  x64
