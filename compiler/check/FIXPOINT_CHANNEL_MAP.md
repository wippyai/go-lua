# Fixpoint Channel Map

This document is the canonical map of fixpoint data transfer in `compiler/check`.
Use it before changing return inference, interproc facts, or widening.

## Scope

- Root loop: `compiler/check/pipeline/driver.go`
- Snapshot state and swap semantics: `compiler/check/store/store.go`
- Pre-flow producer: `compiler/check/infer/return/infer.go`
- Post-flow producers: `compiler/check/infer/interproc/postflow.go`, `compiler/check/infer/nested/processor.go`
- Consumers: `compiler/check/pipeline/runner.go`, `compiler/check/infer/*`, `compiler/check/phase/*`

## Channels

| Channel | Stored In | Main Producer(s) | Main Consumer(s) | Merge/Swap Policy |
|---|---|---|---|---|
| `Effects` | `InterprocPrev/Next.Effects` | `driver.storeFunctionEffect` | effect lookup in flow extraction and termination checks | replace snapshot each iteration (`FixpointSwap`) |
| `ReturnSummaries` | `Facts.ReturnSummaries` | pre-flow return inferencer (`driver.checkFunctionFixpoint` + `infer/return`) | scope/flow declared phase, sibling seeding, postflow alignment | widened in `returns.WidenFacts` via `WidenReturnSummaries` |
| `NarrowReturns` | `Facts.NarrowReturns` | `interproc.StoreFactsFromResult` | narrowing env in runner | widened in `returns.WidenFacts`; also used to refine summaries |
| `FuncTypes` | `Facts.FuncTypes` | `interproc.StoreFactsFromResult` | `runner` sibling lookup, nested self/type enrichment | widened via `WidenFuncTypes` |
| `ParamHints` | `Facts.ParamHints` | `interproc.CollectParamHintsFromResult` | `infer/return`, `infer/paramhints` | widened via `WidenParamHints` |
| `LiteralSigs` | `Facts.LiteralSigs` + scratch | `interproc.StoreFactsFromResult`, scratch from runner | literal signature provider in runner | widened via `WidenLiteralSigs`; scratch cleared each iteration |
| `CapturedTypes` | `Facts.CapturedTypes` | `infer/nested` | `runner` declared types merge, return infer captured overlay | widened via `WidenCapturedTypes` |
| `CapturedFields` | `Facts.CapturedFields` | `interproc.StoreFactsFromResult` | return infer nested field mutation overlay | widened via `WidenCapturedFieldAssigns` |
| `CapturedContainers` | `Facts.CapturedContainers` | `interproc.StoreFactsFromResult` | runner flow extraction extra container mutations | widened via `WidenCapturedContainerMutations` |
| `ConstructorFields` | `InterprocPrev/Next.ConstructorFields` | `infer/nested` constructor detection | self-type enrichment in nested processing | replace snapshot each iteration (`FixpointSwap`) |

## Iteration Boundary

`SessionStore.FixpointSwap()` does:

1. Compare `Effects`, then `Prev <- Next`, reset `Next`.
2. Compute `mergedFacts := widenInterprocFacts(Prev.Facts, Next.Facts)` then store to `Prev`.
3. Compare/replace `ConstructorFields`, reset `Next`.
4. Clear scratch (`LiteralSigsByGraphID`).
5. Record changed channels for non-convergence diagnostics.

## Current Architectural Risk (Root Cause Class)

Precision for local function typing is currently split across multiple channels:

- `ReturnSummaries` (declared/pre-flow)
- `NarrowReturns` (post-flow)
- `FuncTypes` (post-flow function signatures)

These are combined in multiple places with different policies:

- widening (`returns.WidenFacts` + `refineReturnSummariesWithNarrow`)
- read-time alignment (`store.GetLocalFuncTypesSnapshot`)
- write-time alignment (`interproc.StoreFactsFromResult`)

This creates policy drift: a behavior can be "fixed" in one merge path and still regress through another.

## Non-Negotiable Invariants

1. One canonical merge policy per channel family:
   - return vectors
   - function signatures
   - param hints
2. No read-time corrective merge that hides write-time inconsistency.
3. `FuncTypes` and return channels must agree by construction, not by ad-hoc alignment in multiple layers.
4. Pre-flow inference and post-flow inference must declare which one is authoritative for each consumer.

## Canonical Refactor Direction

1. Define a single "function fact reconciliation" entry point in `returns/`:
   - inputs: previous fact, new pre-flow summary, new post-flow summary, new function type
   - output: normalized `{ReturnSummaries, NarrowReturns, FuncTypes}` for one symbol
2. Make `interproc.StoreFactsFromResult` and pre-flow return inference both call that entry point.
3. Remove duplicate local aligners from:
   - `store.GetLocalFuncTypesSnapshot` (read-time patching)
   - any ad-hoc postflow alignment branches
4. Keep `WidenFacts` as the only iteration-boundary widening location.
5. Add regression tests at three layers:
   - unit tests in `returns/` for reconciliation invariants
   - checker regression tests (`compiler/check/tests/regression`)
   - Wippy harness validation in target dirs (`tests/app`, framework `actor/relay/bootloader/migration`)

## Operational Checklist (Before Any Fixpoint Change)

1. Update this map if adding/removing a producer or consumer.
2. Run:
   - `go test ./compiler/check/... -count=1`
   - `go test ./types/... -count=1`
3. Rebuild local wippy binary from current go-lua.
4. Run lint targets with `--cache-reset` in all integration dirs we track.
5. Only then claim convergence/regression status.

