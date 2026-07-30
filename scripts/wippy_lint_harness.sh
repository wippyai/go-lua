#!/usr/bin/env bash
set -euo pipefail

GO_LUA_DIR="${GO_LUA_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DEFAULT_WIPPY_DIR="$HOME/wippy/wippy-golua-seam"
if [[ ! -d "$DEFAULT_WIPPY_DIR" ]]; then
	DEFAULT_WIPPY_DIR="$HOME/wippy/wippy"
fi
WIPPY_DIR="${WIPPY_DIR:-$DEFAULT_WIPPY_DIR}"

OUT_ROOT="${OUT_ROOT:-/tmp/wippy-golua-lint-harness}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="${OUT_DIR:-$OUT_ROOT/$RUN_ID}"
WIPPY_BIN="${WIPPY_BIN:-$OUT_DIR/wippy}"
GOCACHE="${GOCACHE:-/tmp/go-build-cache-wippy-golua}"
GOMODCACHE="${GOMODCACHE:-/tmp/go-mod-cache-wippy-golua}"
LINT_TIMEOUT="${LINT_TIMEOUT:-180s}"
LINT_RSS_LIMIT_MB="${LINT_RSS_LIMIT_MB:-6144}"
LINT_RSS_POLL_SECONDS="${LINT_RSS_POLL_SECONDS:-1}"
LINT_SAMPLE_LIMIT="${LINT_SAMPLE_LIMIT:-3}"
LINT_EXTRA_FLAGS="${LINT_EXTRA_FLAGS:-}"
LINT_NAMESPACE_MODE="${LINT_NAMESPACE_MODE:-auto}"
TARGET_LIMIT="${TARGET_LIMIT:-0}"
RUN_WIPPY_BUILD="${RUN_WIPPY_BUILD:-1}"
RUN_WIPPY_INSTALL="${RUN_WIPPY_INSTALL:-1}"
RUN_LINT="${RUN_LINT:-1}"
REQUIRE_LOCAL_REPLACE="${REQUIRE_LOCAL_REPLACE:-1}"
LINT_DELTA="${LINT_DELTA:-0}"
LINT_DELTA_BASELINE="${LINT_DELTA_BASELINE:-$GO_LUA_DIR/testdata/baselines/external_harness_diagnostics.jsonl}"
LINT_DELTA_FORMAT="${LINT_DELTA_FORMAT:-human}"
LINT_DELTA_FAIL_ON_NEW="${LINT_DELTA_FAIL_ON_NEW:-0}"

export GOCACHE GOMODCACHE
export GOMEMLIMIT="${GOMEMLIMIT:-4GiB}"
export GOGC="${GOGC:-100}"

DEFAULT_TARGET_SPECS=(
	"$WIPPY_DIR/tests/app::"
	"$HOME/wippy/session::"
	"$HOME/wippy/framework/src/test::"
	"$HOME/wippy/framework/src/actor/test::"
	"$HOME/wippy/framework/src/agent/src::"
	"$HOME/wippy/framework/src/bootloader::test/wippy.lock::wippy.bootloader"
	"$HOME/wippy/docker-demo::"
	"$HOME/wippy/framework/src/llm/src::../test/wippy.lock"
	"$HOME/wippy/framework/src/llm/test::"
	"$HOME/wippy/framework/src/migration::"
	"$HOME/wippy/framework/src/views::test/wippy.lock"
	"$HOME/wippy/framework/src/relay/test::"
)

TARGETS="${TARGETS:-${TARGET_SPECS:-}}"
if [[ -n "$TARGETS" ]]; then
	IFS=',' read -r -a TARGET_SPECS <<< "$TARGETS"
else
	TARGET_SPECS=("${DEFAULT_TARGET_SPECS[@]}")
fi

mkdir -p "$OUT_DIR" "$GOCACHE" "$GOMODCACHE"
SUMMARY_TSV="$OUT_DIR/summary.tsv"
CODE_TSV="$OUT_DIR/codes.tsv"
FAMILY_TSV="$OUT_DIR/families.tsv"
DIAGNOSTICS_JSONL="$OUT_DIR/diagnostics.jsonl"
: >"$SUMMARY_TSV"
: >"$CODE_TSV"
: >"$FAMILY_TSV"
: >"$DIAGNOSTICS_JSONL"

section() {
	printf '\n== %s ==\n' "$1"
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 2
}

safe_name() {
	printf '%s' "$1" | sed 's#^/##; s#[^A-Za-z0-9_.-]#_#g'
}

baseline_target_name() {
	local target="$1"
	local home_wippy="$HOME/wippy/"
	if [[ "$target" == "$home_wippy"* ]]; then
		printf '%s\n' "${target#$home_wippy}"
		return 0
	fi
	printf '%s\n' "$target"
}

infer_target_namespace() {
	local target="$1"
	local cfg="$target/wippy.yaml"
	local org
	local mod
	if [[ ! -f "$cfg" ]]; then
		return 0
	fi
	org="$(sed -n 's/^[[:space:]]*organization:[[:space:]]*//p' "$cfg" | head -n 1 | tr -d '"'\''')"
	mod="$(sed -n 's/^[[:space:]]*module:[[:space:]]*//p' "$cfg" | head -n 1 | tr -d '"'\''')"
	if [[ -n "$org" && -n "$mod" ]]; then
		printf '%s.%s\n' "$org" "$mod"
	fi
}

parse_target_spec() {
	local spec="$1"
	target="${spec%%::*}"
	lock_file=""
	target_ns=""
	if [[ "$spec" == *"::"* ]]; then
		local rest="${spec#*::}"
		lock_file="$rest"
		if [[ "$rest" == *"::"* ]]; then
			lock_file="${rest%%::*}"
			target_ns="${rest#*::}"
		fi
	fi
}

has_jq() {
	command -v jq >/dev/null 2>&1
}

extract_json_line() {
	grep '"error_count"' "$1" | tail -n 1 | sed 's/^[^{]*//'
}

json_count() {
	local json_file="$1"
	local field="$2"
	sed -n "s/.*\"$field\":\\([0-9][0-9]*\\).*/\\1/p" "$json_file" | tail -n 1
}

print_code_summary() {
	local json_file="$1"
	if ! has_jq || [[ ! -s "$json_file" ]]; then
		return 0
	fi
	jq -r '(.diagnostics // [])
		| group_by(.code)
		| sort_by(length)
		| reverse
		| .[:8][]
		| "  code \(.[0].code): \(length)"' "$json_file" 2>/dev/null || true
}

append_code_rows() {
	local target="$1"
	local json_file="$2"
	if ! has_jq || [[ ! -s "$json_file" ]]; then
		return 0
	fi
	jq -r --arg target "$target" '(.diagnostics // [])
		| group_by(.code)
		| sort_by(length)
		| reverse
		| .[]
		| [$target, .[0].code, length] | @tsv' "$json_file" >>"$CODE_TSV" 2>/dev/null || true
}

diagnostic_family_jq='
	def family:
		.message as $m
		| if $m | test("^argument [0-9]+ .* is any, not ") then "argument untrusted-any mismatch"
		  elif $m | test("^argument [0-9]+ .* is unknown, not ") then "argument unknown-value mismatch"
		  elif $m | test("^argument [0-9]+ .* is Error, not string") then "error-value passed as string"
		  elif $m | test("^argument [0-9]+ ") then "argument type mismatch"
		  elif $m | test("^cannot pass .* because it may be nil") then "optional argument without proof"
		  elif $m | test("^cannot assign .* because it is any") then "assignment untrusted-any mismatch"
		  elif $m | test("^cannot assign ") then "assignment type mismatch"
		  elif $m | test("^cannot return .* because it may be nil") then "optional return without proof"
		  elif $m | test("^returned value ") then "return type mismatch"
		  elif $m | test(" has no member ") then "missing member"
		  elif $m | test("^cannot call method on an optional value") then "optional method call"
		  elif $m | test("^unknown value ") then "unknown value"
		  elif $m | test("operand of `\\.\\.` may be nil") then "concat optional operand"
		  else $m
		  end;
'

print_family_summary() {
	local json_file="$1"
	if ! has_jq || [[ ! -s "$json_file" ]]; then
		return 0
	fi
	jq -r "$diagnostic_family_jq"'
		(.diagnostics // [])
		| map({family: family})
		| group_by(.family)
		| sort_by(length)
		| reverse
		| .[:8][]
		| "  family \(.[0].family): \(length)"' "$json_file" 2>/dev/null || true
}

append_family_rows() {
	local target="$1"
	local json_file="$2"
	if ! has_jq || [[ ! -s "$json_file" ]]; then
		return 0
	fi
	jq -r --arg target "$target" "$diagnostic_family_jq"'
		(.diagnostics // [])
		| map({family: family})
		| group_by(.family)
		| sort_by(length)
		| reverse
		| .[]
		| [$target, .[0].family, length] | @tsv' "$json_file" >>"$FAMILY_TSV" 2>/dev/null || true
}

append_diagnostic_rows() {
	local target="$1"
	local json_file="$2"
	local target_name
	local target_path
	if ! has_jq || [[ ! -s "$json_file" ]]; then
		return 0
	fi
	target_name="$(baseline_target_name "$target")"
	target_path="$(cd "$target" && pwd -P)"
	jq -c --arg target "$target_name" --arg target_path "$target_path" '(.diagnostics // [])[]
		| def raw_source_path: (.source_path // .file // .filename // .path // "");
		  def resolved_source_path:
			(raw_source_path | tostring) as $file
			| if $file == "" then $target_path
			  elif ($file | startswith("/")) then $file
			  else $target_path + "/" + $file
			  end;
		  {target:$target, source_path:resolved_source_path, entry_id, code, severity, line, column, message}' \
		"$json_file" >>"$DIAGNOSTICS_JSONL" 2>/dev/null || true
}

print_samples() {
	local json_file="$1"
	if ! has_jq || [[ ! -s "$json_file" || "$LINT_SAMPLE_LIMIT" -le 0 ]]; then
		return 0
	fi
	jq -r --argjson n "$LINT_SAMPLE_LIMIT" '(.diagnostics // [])[:$n][]
		| "  sample \(.entry_id // "?") \(.code // "?") L\(.line // 0):\(.column // 0) \(.message // "")"' \
		"$json_file" 2>/dev/null || true
}

run_timeout() {
	if command -v timeout >/dev/null 2>&1; then
		timeout "$LINT_TIMEOUT" "$@"
	else
		"$@"
	fi
}

lint_timeout_seconds() {
	case "$LINT_TIMEOUT" in
		"")
			printf '0\n'
			;;
		*s)
			printf '%s\n' "${LINT_TIMEOUT%s}"
			;;
		*m)
			printf '%s\n' "$(( ${LINT_TIMEOUT%m} * 60 ))"
			;;
		*h)
			printf '%s\n' "$(( ${LINT_TIMEOUT%h} * 3600 ))"
			;;
		*[!0-9]*)
			printf '0\n'
			;;
		*)
			printf '%s\n' "$LINT_TIMEOUT"
			;;
	esac
}

run_lint_to_log() {
	local target="$1"
	local raw_log="$2"
	local lock_file="$3"
	shift 3
	local -a flags=("$@")
	local rss_limit_kb=$((LINT_RSS_LIMIT_MB * 1024))
	local tmp_log="${raw_log}.tmp"
	local pid
	local cmd_status
	local timeout_seconds
	local start_seconds

	timeout_seconds="$(lint_timeout_seconds)"
	start_seconds="$SECONDS"

	(
		cd "$target" || exit 2
		if [[ -n "$lock_file" ]]; then
			exec "$WIPPY_BIN" lint --cache-reset --json "${flags[@]}" --lock-file "$lock_file"
		fi
		exec "$WIPPY_BIN" lint --cache-reset --json "${flags[@]}"
	) >"$tmp_log" 2>&1 &
	pid=$!

	while kill -0 "$pid" 2>/dev/null; do
		if [[ "$timeout_seconds" -gt 0 && $((SECONDS - start_seconds)) -ge "$timeout_seconds" ]]; then
			printf '\nlint killed: timeout=%s elapsed=%ss\n' "$LINT_TIMEOUT" "$((SECONDS - start_seconds))" >>"$tmp_log"
			kill "$pid" 2>/dev/null || true
			wait "$pid" 2>/dev/null || true
			tr -d '\r' <"$tmp_log" >"$raw_log"
			return 124
		fi
		if [[ "$LINT_RSS_LIMIT_MB" -gt 0 ]]; then
			local rss
			rss="$(ps -o rss= -p "$pid" 2>/dev/null | tr -d '[:space:]')"
			if [[ -n "$rss" && "$rss" -gt "$rss_limit_kb" ]]; then
				printf '\nlint killed: rss=%sKB exceeded limit=%sMB\n' "$rss" "$LINT_RSS_LIMIT_MB" >>"$tmp_log"
				kill "$pid" 2>/dev/null || true
				wait "$pid" 2>/dev/null || true
				tr -d '\r' <"$tmp_log" >"$raw_log"
				return 124
			fi
		fi
		sleep "$LINT_RSS_POLL_SECONDS"
	done

	wait "$pid"
	cmd_status=$?
	tr -d '\r' <"$tmp_log" >"$raw_log"
	return "$cmd_status"
}

[[ -d "$GO_LUA_DIR" ]] || die "missing go-lua directory: $GO_LUA_DIR"
[[ -d "$WIPPY_DIR" ]] || die "missing Wippy directory: $WIPPY_DIR"

section "harness"
printf 'go-lua: %s\n' "$GO_LUA_DIR"
printf 'wippy:  %s\n' "$WIPPY_DIR"
printf 'out:    %s\n' "$OUT_DIR"
printf 'limits: timeout=%s rss=%sMB GOMEMLIMIT=%s GOGC=%s install=%s ns=%s delta=%s\n' "$LINT_TIMEOUT" "$LINT_RSS_LIMIT_MB" "$GOMEMLIMIT" "$GOGC" "$RUN_WIPPY_INSTALL" "$LINT_NAMESPACE_MODE" "$LINT_DELTA"

if [[ "$LINT_DELTA" == "1" ]] && ! has_jq; then
	die "LINT_DELTA requires jq to normalize current diagnostics"
fi

if [[ "$REQUIRE_LOCAL_REPLACE" == "1" ]]; then
	replace_line="replace github.com/wippyai/go-lua => $GO_LUA_DIR"
	if ! grep -Fxq "$replace_line" "$WIPPY_DIR/go.mod"; then
		die "missing local go-lua replace in $WIPPY_DIR/go.mod; expected: $replace_line"
	fi
fi

status=0
delta_status=0

if [[ "$RUN_WIPPY_BUILD" == "1" ]]; then
	section "build wippy"
	if ! (cd "$WIPPY_DIR" && go build -o "$WIPPY_BIN" ./cmd/wippy); then
		status=1
		if [[ "$RUN_LINT" == "1" ]]; then
			printf 'skip lint: build failed\n'
			exit "$status"
		fi
	fi
fi

if [[ "$RUN_LINT" != "1" ]]; then
	exit "$status"
fi

section "lint targets"
printf 'target\terrors\twarnings\thints\tstatus\tjson\n' >>"$SUMMARY_TSV"
target_index=0
for spec in "${TARGET_SPECS[@]}"; do
	target_index=$((target_index + 1))
	if [[ "$TARGET_LIMIT" -gt 0 && "$target_index" -gt "$TARGET_LIMIT" ]]; then
		break
	fi

	parse_target_spec "$spec"

	if [[ ! -d "$target" ]]; then
		printf '%s | SKIP missing directory\n' "$target"
		printf '%s\t0\t0\t0\tSKIP\t\n' "$target" >>"$SUMMARY_TSV"
		continue
	fi

	if [[ "$LINT_NAMESPACE_MODE" == "auto" && -z "$target_ns" ]]; then
		target_ns="$(infer_target_namespace "$target")"
	fi

	if [[ "$RUN_WIPPY_INSTALL" == "1" ]]; then
		if [[ -n "$lock_file" ]]; then
			if ! (cd "$target" && run_timeout "$WIPPY_BIN" install --lock-file "$lock_file" >/dev/null 2>&1); then
				printf '%s | install failed\n' "$target"
				status=1
			fi
		else
			if ! (cd "$target" && run_timeout "$WIPPY_BIN" install >/dev/null 2>&1); then
				printf '%s | install failed\n' "$target"
				status=1
			fi
		fi
	fi

	log_base="$OUT_DIR/$(printf '%02d-%s' "$target_index" "$(safe_name "$target")")"
	raw_log="$log_base.raw"
	json_log="$log_base.json"

	extra_flags=()
	if [[ -n "$LINT_EXTRA_FLAGS" ]]; then
		# shellcheck disable=SC2206 # caller intentionally supplies split flags.
		extra_flags=($LINT_EXTRA_FLAGS)
	fi
	if [[ -n "$target_ns" && " ${extra_flags[*]} " != *" --ns "* ]]; then
		extra_flags+=(--ns "$target_ns")
	fi

	set +e
	run_lint_to_log "$target" "$raw_log" "$lock_file" "${extra_flags[@]}"
	cmd_status=$?
	set -e

	extract_json_line "$raw_log" >"$json_log" || true

	errors="$(json_count "$json_log" error_count || true)"
	warnings="$(json_count "$json_log" warning_count || true)"
	hints="$(json_count "$json_log" hint_count || true)"
	if [[ -z "$errors" || -z "$warnings" || -z "$hints" ]]; then
		printf '%s | FAIL parse json raw=%s\n' "$target" "$raw_log"
		printf '%s\t?\t?\t?\tPARSE_FAIL\t%s\n' "$target" "$json_log" >>"$SUMMARY_TSV"
		status=1
		continue
	fi

	target_status="OK"
	if [[ "$cmd_status" -ne 0 ]]; then
		target_status="CMD_FAIL"
	fi
	if [[ "$errors" -ne 0 ]]; then
		target_status="ERRORS"
	fi
	if [[ "$cmd_status" -ne 0 || "$errors" -ne 0 ]]; then
		status=1
	fi

	printf '%s | errors=%s warnings=%s hints=%s status=%s json=%s\n' "$target" "$errors" "$warnings" "$hints" "$target_status" "$json_log"
	print_code_summary "$json_log"
	print_family_summary "$json_log"
	print_samples "$json_log"
	append_diagnostic_rows "$target" "$json_log"
	append_code_rows "$target" "$json_log"
	append_family_rows "$target" "$json_log"
	printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$target" "$errors" "$warnings" "$hints" "$target_status" "$json_log" >>"$SUMMARY_TSV"
done

section "summary files"
printf '%s\n' "$SUMMARY_TSV"
printf '%s\n' "$CODE_TSV"
printf '%s\n' "$FAMILY_TSV"
printf '%s\n' "$DIAGNOSTICS_JSONL"

if [[ "$LINT_DELTA" == "1" ]]; then
	section "diagnostic delta"
	case "$LINT_DELTA_FORMAT" in
		json | jsonl)
			DELTA_REPORT="$OUT_DIR/diagnostic_delta.jsonl"
			;;
		*)
			DELTA_REPORT="$OUT_DIR/diagnostic_delta.txt"
			;;
	esac
	if ! DIFFREPORT_FORMAT="$LINT_DELTA_FORMAT" DIFFREPORT_FAIL_ON_NEW="$LINT_DELTA_FAIL_ON_NEW" \
		"$GO_LUA_DIR/scripts/wippy_diag_delta.sh" "$LINT_DELTA_BASELINE" "$DIAGNOSTICS_JSONL" "$DELTA_REPORT"; then
		delta_status=1
		status=1
	fi
	if [[ -s "$DELTA_REPORT" ]]; then
		cat "$DELTA_REPORT"
	else
		printf 'no delta report written: %s\n' "$DELTA_REPORT"
	fi
	printf '%s\n' "$DELTA_REPORT"
fi

exit "$status"
