# Fixpoint Kernel Design

Purpose: define one canonical abstraction for local-function facts so policy is centralized, deletions are explicit, and regressions map to one ownership boundary.

## Problem Statement

Current regressions come from two coupled design gaps:

1. Function facts are split across multiple channels (`ReturnSummaries`, `NarrowReturns`, `FuncTypes`) and reconciled in multiple places (writer-time, widen-time, read-time).
2. Assignment-call typing can observe post-assignment target states at the same CFG point, causing RHS contamination (`entries = filter_tests(entries, args)` class).

Both create policy drift and fragile local fixes.

## Canonical Abstractions

## A) Function Fact Kernel (FFK)

Owner: `compiler/check/returns`

FFK is the only place allowed to reconcile local function semantics.

Input per symbol:
- previous fact (`FuncTypes`, `ReturnSummaries`, `NarrowReturns`)
- pre-flow summary candidate
- post-flow narrow summary candidate
- post-flow function-type candidate

Output per symbol:
- canonical function type
- canonical return summary
- canonical narrow summary

Invariants:
1. Shape-preserving merge for same-signature functions.
2. Return merge policy is centralized and reused everywhere.
3. No read-time alignment/fixups.
4. Widening happens only at fixpoint boundary.

## B) Pre-Assignment RHS Overlay (PARO)

Owner: `compiler/check/flowbuild/assign`

PARO computes RHS symbol types from predecessor points for symbols being assigned at point `p`.

Invariant:
- RHS synthesis for `x = expr` must evaluate `x` using pre-assignment state, never post-assignment state at `p`.

## Deletion Targets

When FFK is fully wired, remove:
1. Read-time local-function alignment from `store.GetLocalFuncTypesSnapshot`.
2. Any ad-hoc summary->function alignment in consumer paths.
3. Duplicate function merge branches outside `returns/`.

When PARO is wired, remove:
1. ad-hoc assignment-point workarounds that compensate for RHS contamination.

## Wiring Plan

1. Centralize merge policy in `returns/` (FFK APIs).
2. Route pre-flow return inferencer and post-flow interproc writer through FFK.
3. Keep `WidenFacts` as boundary-only widening layer.
4. Remove read-time reconciliation from `store`.
5. Apply PARO in both assignment extraction and SCC local inference paths.
6. Delete redundant wrappers/duplicate merges after parity is proven.

## Regression Classes (must stay green)

1. Suite key precision (`sorted_keys/group_by_suite`).
2. Filter reassignment any-poisoning (`entries = filter_tests(entries, args)`).
3. Channel select helper return narrowing.
4. Dynamic contract open return (`any` + `nil` branch).

## Validation Gate

1. `go test ./compiler/check/returns ./compiler/check/store ./compiler/check/infer/interproc ./compiler/check/infer/return -count=1`
2. `go test ./compiler/check/tests/regression -run "TestLinterFalsePositive_TestRunner|TestWippyRunner_|TestChannelSelectHelperReturnNarrowing|TestContractOpen_DynamicReturnNotCollapsedToNil" -count=1`
3. `go test ./compiler/check/... ./types/... -count=1`
4. `scripts/local_safety_report.sh`

No claim of fix until all four pass (or explicit external dependency skip is recorded).
