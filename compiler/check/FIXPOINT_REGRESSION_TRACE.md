# Fixpoint Regression Trace

Snapshot taken from current branch state via:

- `go test ./compiler/check/... -count=1`
- focused repro scripts under `/tmp` using `compiler/check/tests/testutil`

## Current Failing Tests

From `/tmp/go_check_regressions.log`:

1. `TestDebugTypedEntriesFlow`
2. `TestLinterFalsePositive_TestRunnerPattern`
3. `TestLinterFalsePositive_TestRunnerExact`
4. `TestLinterFalsePositive_TestRunnerWithTypedEntries`
5. `TestLinterFalsePositive_TestRunnerWithFalseMeta`
6. `TestLinterFalsePositive_WippyTestRunner_WithRegistryFind`
7. `TestWippyRunner_SortedKeysWithFilterBranch`
8. `TestWippyRunner_NearLiteralTestRunnerFlow`

## Regression Clusters

## Cluster A: `sorted_keys`/suite-key precision collapse

Impacted tests:

- `TestLinterFalsePositive_TestRunnerPattern`
- `TestLinterFalsePositive_TestRunnerExact`
- `TestLinterFalsePositive_TestRunnerWithTypedEntries`
- `TestLinterFalsePositive_TestRunnerWithFalseMeta`
- `TestDebugTypedEntriesFlow`
- `TestLinterFalsePositive_WippyTestRunner_WithRegistryFind`

Observed state (repro traces):

- Pattern case:
  - `sorted_keys`: `fun(t: {[unknown]: unknown[]}?) -> unknown[]`
  - diagnostic: `argument 1: expected string, got unknown`
- Typed entries case:
  - `sorted_keys`: `fun(t: {...}?) -> string[]`
  - param hints for `sorted_keys` exist and are map-shaped:
    - `[{[string]: Entry[]}]`
  - diagnostic: `argument 1: expected {...}, got {[string]: Entry[]}`
- Registry case:
  - `sorted_keys`: `fun(t: {[any]: Entry[]}?) -> any[]`
  - diagnostic: `argument 1: expected string, got any`

Interpretation:

- Param-hint signal exists for `sorted_keys` but final stored function type can still degrade to coarse/placeholder param domains (`{...}` or unknown-key map).
- This points to a reconciliation gap between hint transport and final `FuncTypes` persistence, not absence of hint generation alone.

## Cluster B: `filter_tests` any-poisoning in branch reassignment

Impacted tests:

- `TestWippyRunner_SortedKeysWithFilterBranch`
- contributes to `TestWippyRunner_NearLiteralTestRunnerFlow`

Observed state:

- `filter_tests` stored local type:
  - `fun(entries: any?, patterns: any?) -> any`
- At call in root:
  - `filter_tests(entries, args)` seen as `a0=any`, `a1=any`
- Follow-on:
  - `group_by_suite(entries)` argument becomes `any`
  - diagnostic: `expected {id: string, meta: {suite: string?}?}[]?, got any`

Interpretation:

- The reassignment path `entries = filter_tests(entries, args)` is losing pre-call argument precision.
- This is likely a symbol-state ordering/versioning issue at call-in-assignment points (pre-state vs post-state contamination), amplified by split function-fact channels.

## Cluster C: cross-channel drift in function semantics

Symptoms across A/B:

- `ReturnSummaries`, `NarrowReturns`, and `FuncTypes` can disagree on parameter/return precision.
- read-time patching/alignment tries to repair this late, but regressions still leak.

Relevant code seams:

- pre-flow write: `compiler/check/pipeline/driver.go`
- post-flow write/alignment: `compiler/check/infer/interproc/postflow.go`
- read-time alignment: `compiler/check/store/store.go`
- iteration widening/refinement: `compiler/check/returns/widen.go`

## Root Issue Hypothesis (Current)

Primary issue is not one isolated checker rule; it is inconsistent ownership of local-function semantics across channels plus weak pre/post state separation at some call-assignment sites.

As long as:

1. function semantics are split across multiple channels, and
2. reconciliation happens in multiple phases (write-time + boundary + read-time),

the same failure class reappears in new forms.

## Minimum Guardrail Suite for This Class

Run together for every refactor touching fixpoint/interproc/return inference:

1. `go test ./compiler/check/returns ./compiler/check/store ./compiler/check/infer/interproc ./compiler/check/infer/return -count=1`
2. `go test ./compiler/check/tests/flow -run TestFixpointUnification -count=1`
3. `go test ./compiler/check/tests/regression -run 'TestLinterFalsePositive_TestRunner|TestWippyRunner_|TestChannelSelectHelperReturnNarrowing|TestContractOpen_DynamicReturnNotCollapsedToNil' -count=1`
4. `go test ./compiler/check/... ./types/... -count=1`
5. `scripts/local_safety_report.sh` (integration/lint harness)

