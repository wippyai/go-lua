# Analysis Baselines

These files are Phase 0 migration oracles for the judgment-IR cleanup.

## Local Fixtures

Generate the normalized fixture baseline with:

```sh
env GOCACHE=/tmp/go-build-cache \
  FIXTURE_BASELINE_OUT=testdata/baselines/fixture_diagnostics.jsonl \
  go test -run TestWriteFixtureDiagnosticBaseline -count=1 -v
```

Current snapshot:

- `fixture_diagnostics.jsonl`
- 574 suites total
- 556 suites pass curated expectations
- 18 suites fail curated expectations
- 0 suites skipped

This baseline records suite status, diagnostics, evidence, labels, and curated
missing/unexpected rows. A producer migration may only change it when the delta
is explicitly classified as an accepted engine precision change or an accepted
old-diagnostic bug.

## External Harness

Generate the Wippy harness baseline with:

```sh
env GOCACHE=/tmp/go-build-cache-wippy-golua \
  GOMODCACHE=/tmp/go-mod-cache-wippy-golua \
  OUT_ROOT=/tmp/wippy-golua-lint-harness \
  RUN_ID=phase0-baseline \
  RUN_GOLUA_TESTS=0 \
  STRICT=0 \
  scripts/wippy_lint_harness.sh
cp /tmp/wippy-golua-lint-harness/phase0-baseline/summary.tsv \
  testdata/baselines/external_harness_summary.tsv
cp /tmp/wippy-golua-lint-harness/phase0-baseline/codes.tsv \
  testdata/baselines/external_harness_codes.tsv
cp /tmp/wippy-golua-lint-harness/phase0-baseline/families.tsv \
  testdata/baselines/external_harness_families.tsv
tail -n +2 /tmp/wippy-golua-lint-harness/phase0-baseline/summary.tsv |
  while IFS="$(printf '\t')" read -r target errors warnings hints status json; do
    jq -c --arg target "$target" \
      '.diagnostics[]? | {target:$target, entry_id, code, severity, line, column, message}' \
      "$json"
  done > testdata/baselines/external_harness_diagnostics.jsonl
```

Current snapshot:

- `external_harness_summary.tsv`
- `external_harness_codes.tsv`
- `external_harness_families.tsv`
- `external_harness_diagnostics.jsonl`
- 77 errors
- 2 warnings
- 0 hints

`external_harness_diagnostics.jsonl` is a normalized per-diagnostic shadow-diff
oracle with target, entry id, code, severity, line, column, and message. The raw
per-target JSON logs remain under `/tmp/wippy-golua-lint-harness`.
