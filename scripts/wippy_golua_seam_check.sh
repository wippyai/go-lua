#!/usr/bin/env bash
set -euo pipefail

SEAM_DIR="${WIPPY_GOLUA_SEAM_DIR:-${WIPPY_ROOT:?set WIPPY_ROOT}/wippy-golua-seam}"
GOLUA_DIR="${GOLUA_DIR:-${WIPPY_ROOT:?set WIPPY_ROOT}/go-lua}"
GOCACHE="${GOCACHE:-/tmp/go-build-cache-wippy-golua-seam}"
LOG="${LOG:-/tmp/wippy-golua-seam-check.log}"
SEAM_PACKAGES="${SEAM_PACKAGES:-./runtime/lua/code/...}"
SEAM_MODE="${SEAM_MODE:-test}"

if [[ ! -d "$SEAM_DIR" ]]; then
	echo "missing seam checkout: $SEAM_DIR" >&2
	exit 2
fi

if [[ ! -d "$GOLUA_DIR" ]]; then
	echo "missing go-lua checkout: $GOLUA_DIR" >&2
	exit 2
fi

cd "$SEAM_DIR"

branch="$(git branch --show-current)"
replace_line="replace github.com/wippyai/go-lua => $GOLUA_DIR"
if ! grep -Fxq "$replace_line" go.mod; then
	echo "missing local go-lua replace in $SEAM_DIR/go.mod" >&2
	echo "expected: $replace_line" >&2
	exit 2
fi

mkdir -p "$GOCACHE" "$(dirname "$LOG")"

set +e
case "$SEAM_MODE" in
	test)
		# shellcheck disable=SC2086 # SEAM_PACKAGES intentionally accepts a package list.
		env GOCACHE="$GOCACHE" go test $SEAM_PACKAGES >"$LOG" 2>&1
		;;
	build)
		# shellcheck disable=SC2086 # SEAM_PACKAGES intentionally accepts a package list.
		env GOCACHE="$GOCACHE" go build $SEAM_PACKAGES >"$LOG" 2>&1
		;;
	*)
		echo "unknown SEAM_MODE: $SEAM_MODE (want test or build)" >&2
		exit 2
		;;
esac
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
	echo "wippy/go-lua seam passed on $branch"
	exit 0
fi

echo "wippy/go-lua seam failed on $branch"
echo "mode: $SEAM_MODE"
echo "packages: $SEAM_PACKAGES"
echo "log: $LOG"
sed -n '1,120p' "$LOG"
exit "$status"
