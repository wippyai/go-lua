#!/usr/bin/env bash
set -euo pipefail

GO_LUA_DIR="${GO_LUA_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

usage() {
	printf 'usage: %s <baseline.jsonl> <current.jsonl> [out]\n' "$(basename "$0")" >&2
	printf 'env: DIFFREPORT_FORMAT=human|jsonl DIFFREPORT_FAIL_ON_NEW=0|1\n' >&2
	exit 2
}

abs_path() {
	case "$1" in
		/*) printf '%s\n' "$1" ;;
		*) printf '%s/%s\n' "$PWD" "$1" ;;
	esac
}

baseline="${1:-${DIFFREPORT_BASELINE:-}}"
current="${2:-${DIFFREPORT_CURRENT:-}}"
out="${3:-${DIFFREPORT_OUT:-}}"

if [[ -z "$baseline" || -z "$current" ]]; then
	usage
fi

baseline="$(abs_path "$baseline")"
current="$(abs_path "$current")"
if [[ -n "$out" ]]; then
	out="$(abs_path "$out")"
else
	out="$(mktemp "${TMPDIR:-/tmp}/wippy-diag-delta.XXXXXX")"
	trap 'rm -f "$out"' EXIT
fi

log="$(mktemp "${TMPDIR:-/tmp}/wippy-diag-delta-go-test.XXXXXX")"
trap 'rm -f "$log"; [[ "${3:-${DIFFREPORT_OUT:-}}" == "" ]] && rm -f "$out"' EXIT

set +e
(
	cd "$GO_LUA_DIR" &&
		DIFFREPORT_BASELINE="$baseline" \
		DIFFREPORT_CURRENT="$current" \
		DIFFREPORT_OUT="$out" \
		DIFFREPORT_FORMAT="${DIFFREPORT_FORMAT:-human}" \
		DIFFREPORT_FAIL_ON_NEW="${DIFFREPORT_FAIL_ON_NEW:-0}" \
		go test . -run '^TestWriteDiagnosticDiffReport$' -count=1
) >"$log" 2>&1
status=$?
set -e

if [[ "$status" -ne 0 ]]; then
	cat "$log" >&2
fi

if [[ -z "${3:-${DIFFREPORT_OUT:-}}" ]]; then
	cat "$out"
fi

exit "$status"
