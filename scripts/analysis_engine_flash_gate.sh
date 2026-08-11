#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

engine_pattern='\b(ActionGraph|CompileActions|QueryCone|RuleAction|CommitAction|GroupRef|GroupAt|groupEpoch|runtimeGroup|runtimeAction|boundRuleAction|boundSupportAction|runtimeCommittedInput|runtimeEquationGroup|bindRuleAction|bindSupportAction|EquationCache)\b'
old_import_pattern='github\.com/wippyai/go-lua/analysis/engine/(state|factflow|transfer|visibility|callboundary|callpayload|dynamicindex|operationplan|sourcevalue|cancellation)(["/]|$)'

failed=0

if rg -n --glob '*.go' --glob '!**/__legacy/**' --glob '!**/_reference/**' "$engine_pattern" analysis/engine; then
	echo 'analysis engine flash gate: obsolete execution-spine vocabulary remains' >&2
	failed=1
fi

if rg -n --glob '*.go' --glob '!**/__legacy/**' --glob '!**/_reference/**' "$old_import_pattern" .; then
	echo 'analysis engine flash gate: obsolete engine package import remains' >&2
	failed=1
fi

if (( failed != 0 )); then
	exit 1
fi

echo 'analysis engine flash gate: clean'
