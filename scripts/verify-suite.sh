#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WIPPY_DIR="${WIPPY_DIR:-$HOME/wippy/wippy}"
WIPPY_BIN="${WIPPY_BIN:-/tmp/wippy-local}"

LINT_TARGET_SPECS=(
  "$WIPPY_DIR/tests/app::"
  "$HOME/wippy/session::"
  "$HOME/wippy/framework/src/test::"
  "$HOME/wippy/framework/src/actor/test::"
  "$HOME/wippy/framework/src/agent/src::wippy.lock"
  "$HOME/wippy/framework/src/bootloader::test/wippy.lock"
  "$HOME/wippy/docker-demo/src::../../wippy/wippy.lock"
  "$HOME/wippy/framework/src/llm/src::wippy.lock"
  "$HOME/wippy/framework/src/llm/test::wippy.lock"
  "$HOME/wippy/framework/src/migration::"
  "$HOME/wippy/framework/src/views::test/wippy.lock"
  "$HOME/wippy/framework/src/relay/test::"
)

status=0

section() {
  printf "\n== %s ==\n" "$1"
}

section "go-lua checker tests"
if ! (cd "$ROOT_DIR" && go test ./compiler/check/... -count=1); then
  status=1
fi

if [[ ! -d "$WIPPY_DIR" ]]; then
  echo "skip wippy checks: missing directory $WIPPY_DIR"
  exit "$status"
fi

section "build wippy binary"
if ! (cd "$WIPPY_DIR" && go build -o "$WIPPY_BIN" ./cmd/wippy); then
  status=1
  echo "skip lint checks: failed to build $WIPPY_BIN"
  exit "$status"
fi

section "wippy lint targets"
for spec in "${LINT_TARGET_SPECS[@]}"; do
  target="${spec%%::*}"
  lock_file="${spec##*::}"

  if [[ ! -d "$target" ]]; then
    echo "$target | SKIP (missing directory)"
    continue
  fi

  set +e
  if [[ -n "$lock_file" ]]; then
    raw="$(cd "$target" && "$WIPPY_BIN" lint --cache-reset --json --lock-file "$lock_file" 2>&1 | tr -d '\r')"
  else
    raw="$(cd "$target" && "$WIPPY_BIN" lint --cache-reset --json 2>&1 | tr -d '\r')"
  fi
  cmd_status=$?
  set -e

  json_line="$(printf '%s\n' "$raw" | grep '"error_count"' | tail -n 1 || true)"
  errors="$(printf '%s' "$json_line" | sed -n 's/.*"error_count":\([0-9][0-9]*\).*/\1/p')"
  warnings="$(printf '%s' "$json_line" | sed -n 's/.*"warning_count":\([0-9][0-9]*\).*/\1/p')"
  hints="$(printf '%s' "$json_line" | sed -n 's/.*"hint_count":\([0-9][0-9]*\).*/\1/p')"

  if [[ -z "$errors" || -z "$warnings" || -z "$hints" ]]; then
    echo "$target | FAIL (could not parse lint JSON)"
    status=1
    continue
  fi

  echo "$target | errors=$errors warnings=$warnings hints=$hints"
  if [[ "$cmd_status" -ne 0 || "$errors" -ne 0 ]]; then
    status=1
  fi
done

exit "$status"
