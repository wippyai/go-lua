#!/usr/bin/env bash
set -euo pipefail

# Fixture execution and manifest equivalence dominate race-instrumented time.
# These ranges partition both corpus tests by every letter, while the other
# shard runs all remaining fixtures and tests.
case "${1:-}" in
  other)
    for path in testdata/fixtures/regression/*/; do
      name=${path%/}
      name=${name##*/}
      if [[ ! $name =~ ^[a-z] ]]; then
        printf 'regression fixture outside race shards: %s\n' "$name" >&2
        exit 1
      fi
    done

    go test -race -timeout 30m -skip '^(TestFixtures|TestFixtureManifestDiagnosticEquivalence)$' ./...

    categories=()
    for path in testdata/fixtures/*/; do
      name=${path%/}
      name=${name##*/}
      if [[ $name != regression ]]; then
        if [[ ! $name =~ ^[a-z]+$ ]]; then
          printf 'invalid fixture category for race filter: %s\n' "$name" >&2
          exit 1
        fi
        categories+=("$name")
      fi
    done
    pattern=$(IFS='|'; printf '%s' "${categories[*]}")
    go test -race -timeout 30m -run "^(TestFixtures|TestFixtureManifestDiagnosticEquivalence)$/^(${pattern})$" .
    exit
    ;;
  ac) range='[a-c]' ;;
  dg) range='[d-g]' ;;
  hj) range='[h-j]' ;;
  k)  range='k' ;;
  lr) range='[l-r]' ;;
  sz) range='[s-z]' ;;
  *) printf 'unknown race shard: %s\n' "${1:-}" >&2; exit 2 ;;
esac

go test -race -timeout 30m -run "^(TestFixtures|TestFixtureManifestDiagnosticEquivalence)$/^regression$/^${range}" .
