# Fixpoint Regression Trace

This file tracks the current regression state for the fixpoint kernel work.

## Current Snapshot

Validated on branch `feature/unified-fixpoint-kernel` with:

- `go test ./compiler/check/returns ./compiler/check/store ./compiler/check/infer/interproc ./compiler/check/infer/return -count=1`
- `go test ./compiler/check/tests/flow -run TestFixpointUnification -count=1`
- `go test ./compiler/check/tests/regression -run 'TestLinterFalsePositive_TestRunner|TestWippyRunner_|TestChannelSelectHelperReturnNarrowing|TestContractOpen_DynamicReturnNotCollapsedToNil' -count=1`

Result: all green.

## Notes

- The earlier runner/sorted-keys/filter-tests fixpoint regressions are currently not reproducing under the guardrail suite above.
- Integration harness (`scripts/verify-suite.sh`) still reports non-zero lint diagnostics outside go-lua in:
  - `~/wippy/framework/src/llm/src` and `~/wippy/framework/src/llm/test`
  - `~/wippy/docker-demo/src`
- Those remaining diagnostics are tracked as cross-repo integration follow-up, not unresolved go-lua core-suite failures.

## Required Validation Before Claiming Another Fixpoint Change

1. `go test ./compiler/check/returns ./compiler/check/store ./compiler/check/infer/interproc ./compiler/check/infer/return -count=1`
2. `go test ./compiler/check/tests/flow -run TestFixpointUnification -count=1`
3. `go test ./compiler/check/tests/regression -run 'TestLinterFalsePositive_TestRunner|TestWippyRunner_|TestChannelSelectHelperReturnNarrowing|TestContractOpen_DynamicReturnNotCollapsedToNil' -count=1`
4. `go test ./compiler/check/... ./types/... -count=1`
5. `scripts/verify-suite.sh`
