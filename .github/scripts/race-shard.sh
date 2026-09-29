#!/usr/bin/env bash
set -euo pipefail

# Regression fixtures dominate race-instrumented time. These ranges partition
# every letter, while the other shard runs all remaining fixtures and tests.
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

    # The fixpoint replay oracle runs in its own non-race CI job.
    go test -race -timeout 30m -skip '^(TestFixtures|TestFixturesFixpointReplay)$' ./...

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
    go test -race -timeout 30m -run "^TestFixtures$/^(${pattern})$" .
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

go test -race -timeout 30m -run "^TestFixtures$/^regression$/^${range}" .
