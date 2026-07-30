#!/usr/bin/env bash
# Reproduce P3 Stage 0 attribution in fresh analyzer processes. The test binary
# is compiled once so reported elapsed time and RSS exclude Go compilation.
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fixtures=(
  "realworld/transactional-saga-orchestrator-soundness"
  "realworld/wippy-scheduler-create-integration"
)

tmp="$(mktemp -d "${TMPDIR:-/tmp}/go-lua-p3-stage0.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
binary="$tmp/fixture.test"
GOCACHE="$tmp/go-build" go test -c -o "$binary" .

for fixture in "${fixtures[@]}"; do
  IFS=/ read -r group suite <<<"$fixture"
  printf '\n== fresh-process attribution: %s ==\n' "$fixture"
  env FIXTURE_STATS=1 FIXTURE_SEQUENTIAL=1 FIXTURE_WTO=1 \
    /usr/bin/time -f 'elapsed=%e maxrss_kb=%M exit=%x' \
    "$binary" -test.run "^TestFixtures$/^${group}$/^${suite}$/^check$" -test.count=1 -test.v
done
