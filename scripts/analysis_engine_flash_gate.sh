#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

engine_pattern='\b(ActionGraph|CompileActions|QueryCone|RuleAction|CommitAction|GroupRef|GroupAt|groupEpoch|runtimeGroup|runtimeAction|boundRuleAction|boundSupportAction|runtimeCommittedInput|runtimeEquationGroup|bindRuleAction|bindSupportAction|EquationCache)\b'
old_import_pattern='github\.com/wippyai/go-lua/analysis/engine/(state|factflow|transfer|visibility|callboundary|callpayload|dynamicindex|operationplan|sourcevalue|cancellation)(["/]|$)'

if ! command -v rg >/dev/null 2>&1; then
	echo 'analysis engine flash gate: ripgrep (rg) is unavailable, so no pattern can be searched' >&2
	echo 'analysis engine flash gate: install ripgrep; a missing search tool is a gate failure, not a clean tree' >&2
	exit 2
fi

failed=0

# rg exits 0 on a match, 1 on no match, and 2 or above on a search error. Only
# 1 means this gate looked and found nothing; every other nonzero status is a
# gate that did not run.
scan() {
	local description=$1
	local pattern=$2
	shift 2
	local status=0
	rg -n --glob '*.go' --glob '!**/__legacy/**' --glob '!**/_reference/**' "$pattern" "$@" || status=$?
	case $status in
	0)
		echo "analysis engine flash gate: $description" >&2
		failed=1
		;;
	1) ;;
	*)
		echo "analysis engine flash gate: search for $description failed with status $status" >&2
		failed=1
		;;
	esac
}

scan 'obsolete execution-spine vocabulary remains' "$engine_pattern" analysis/engine
scan 'obsolete engine package import remains' "$old_import_pattern" .

if (( failed != 0 )); then
	exit 1
fi

echo 'analysis engine flash gate: clean'
