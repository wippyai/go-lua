#!/usr/bin/env bash
# Gate: every engine fail-closed diagnostic family observed in a harness
# diagnostics.jsonl must have a matching family_pattern row in
# analysis/architecture/external_failure_census.csv. A gap means a new
# fail-closed family appeared externally: add a red census test first,
# then a matrix row.
#
# usage: scripts/external_failure_census_check.sh <diagnostics.jsonl> [census.csv]
set -euo pipefail

usage() {
	echo "usage: $0 <diagnostics.jsonl> [census.csv]" >&2
	exit 2
}

[[ $# -ge 1 && $# -le 2 ]] || usage

DIAGNOSTICS="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CENSUS_CSV="${2:-$SCRIPT_DIR/../analysis/architecture/external_failure_census.csv}"

[[ -f "$DIAGNOSTICS" ]] || { echo "diagnostics file not found: $DIAGNOSTICS" >&2; exit 2; }
[[ -f "$CENSUS_CSV" ]] || { echo "census matrix not found: $CENSUS_CSV" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 2; }

# Engine fail-closed diagnostics are W0000 records whose message carries an
# internal engine prefix. Normalization strips 64-hex body hashes and digit
# runs (expression/point/source indices) so instances collapse into families.
families="$(
	jq -r '
		select(.code == "W0000")
		| select((.message | startswith("transformer:")) or (.message | startswith("prepare lexical forest:")))
		| [(.message | gsub("[0-9a-f]{64}"; "<hash>") | gsub("[0-9]+"; "N")), .entry_id]
		| @tsv
	' "$DIAGNOSTICS" \
		| awk -F'\t' '
			{ count[$1]++; if (!($1 in rep)) rep[$1] = $2 }
			END { for (key in count) printf "%d\t%s\t%s\n", count[key], rep[key], key }
		' \
		| sort -t "$(printf '\t')" -k1,1nr -k3,3
)"

if [[ -z "$families" ]]; then
	echo "external failure census: no engine fail-closed diagnostics in $DIAGNOSTICS"
	exit 0
fi

patterns="$(tail -n +2 "$CENSUS_CSV" | cut -d, -f1)"

gaps="$(awk -F'\t' -v patterns="$patterns" '
	BEGIN { n = split(patterns, pats, "\n") }
	{
		for (i = 1; i <= n; i++) {
			if (pats[i] != "" && index($3, pats[i]) > 0) next
		}
		print
	}
' <<<"$families")"

total="$(wc -l <<<"$families")"
if [[ -z "$gaps" ]]; then
	echo "external failure census: all $total engine fail-closed families covered by $CENSUS_CSV"
	exit 0
fi

echo "external failure census gaps (count, representative entry, normalized family):" >&2
echo "$gaps" >&2
echo "each gap is a new engine fail-closed family: add a red census test in analysis/check/fixpoint/program, then a family_pattern row to $CENSUS_CSV" >&2
exit 1
