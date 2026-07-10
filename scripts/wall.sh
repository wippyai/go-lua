#!/usr/bin/env bash
# Full repository verification wall. Run from anywhere with: bash scripts/wall.sh
# PROMPTMAP is optional; when unset, its external meta-audit is visibly skipped.
set -uo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Sandboxed and CI shells may inherit a read-only home cache. Keep the wall
# runnable from a fresh clone while allowing callers to supply shared caches.
export GOCACHE="${GOCACHE:-/tmp/go-build-cache-golua-wall}"
export GOMODCACHE="${GOMODCACHE:-/tmp/go-mod-cache-golua-wall}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

declare -a NAMES=()
declare -a RESULTS=()
failed=0

run() {
	local name="$1"
	shift
	printf '\n== %s ==\n' "$name"
	if "$@"; then
		NAMES+=("$name")
		RESULTS+=("PASS")
	else
		NAMES+=("$name")
		RESULTS+=("FAIL")
		failed=1
	fi
}

skip() {
	local name="$1"
	local reason="$2"
	printf '\n== %s ==\nSKIPPED: %s\n' "$name" "$reason"
	NAMES+=("$name")
	RESULTS+=("SKIPPED")
}

run "build" go build ./...
run "vet" go vet ./...
run "full tests" go test ./...
run "fixtures" go test . -run '^TestFixtures$' -count=1
run "fixture meta-gates" go test . -run '^(TestCuratedGate|TestFixtureDiagnosticsRequireEvidenceRenderAndLabels|TestDiagnosticExpectation.*|TestDiagnosticFileMatchingUsesExactFixtureAliases|TestFixtureDiagnostic.*|TestDiagnosticRenderPolicy.*|TestStructuredDiagnosticsCanRequireCompleteList|TestFrameLocalFixtureQualificationStats|TestDecomposableFixtureQualificationStats)$' -count=1
run "curated oracle" go test . -run '^TestCuratedOracle$' -count=1
run "soundness probes" go test . -run '^TestSoundness' -count=1
run "lattice laws" go test ./analysis/engine/factapply -run '^TestCoreAbstractInterpretationLaws$' -count=1
run "architecture/import audits" go test ./analysis/architecture -count=1

if [[ -n "${PROMPTMAP:-}" ]]; then
	run "promptmap meta-audit" env OUTDIR="${PROMPTMAP_OUTDIR:-/tmp/go-lua-promptmap-meta}" bash scripts/promptmap_meta_audit.sh
else
	skip "promptmap meta-audit" "PROMPTMAP is unset"
fi

# `go test -run '^Fuzz'` executes each fuzz target's seed corpus once; it does
# not enter the long-running fuzzing mode selected by `-fuzz`.
run "fuzz seed corpus" go test ./... -run '^Fuzz' -count=1

printf '\n%-30s %s\n' "CHECK" "RESULT"
printf '%-30s %s\n' "-----" "------"
for i in "${!NAMES[@]}"; do
	printf '%-30s %s\n' "${NAMES[i]}" "${RESULTS[i]}"
done

if (( failed )); then
	exit 1
fi
