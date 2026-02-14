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
| `FunctionFacts` | `Facts.FunctionFacts` | pre-flow + post-flow via `returns.MergeFunctionFactIntoFacts` / `returns.MergeFunctionFactsIntoFacts` | declared phase, narrowing phase, sibling snapshots, callsite typing | widened in `returns.WidenFacts` via canonical `ReconcileFunctionFact` |
| `ReturnSummaries` | `Facts.ReturnSummaries` | compatibility mirror derived from canonical writes | legacy read surfaces expecting summary maps | rewritten from `FunctionFacts` in `returns.NormalizeFunctionFactChannels` |
| `NarrowReturns` | `Facts.NarrowReturns` | compatibility mirror derived from canonical writes | legacy narrowing env map surfaces | rewritten from `FunctionFacts` in `returns.NormalizeFunctionFactChannels` |
| `FuncTypes` | `Facts.FuncTypes` | compatibility mirror derived from canonical writes | legacy sibling/type lookup surfaces | rewritten from `FunctionFacts` in `returns.NormalizeFunctionFactChannels` |
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

Primary root-cause class was split policy across `ReturnSummaries` / `NarrowReturns` /
`FuncTypes`. That path has been canonicalized around `FunctionFacts`.

Remaining risk is migration debt: compatibility mirrors still exist and must stay
strictly derived from canonical facts to avoid drift. Any new producer must write
through `returns` reconciliation helpers, never directly to mirrors.

## Non-Negotiable Invariants

1. One canonical merge policy per channel family:
   - return vectors
   - function signatures
   - param hints
2. No read-time corrective merge that hides write-time inconsistency.
3. Compatibility mirrors are derived views only; canonical source is `FunctionFacts`.
4. Pre-flow inference and post-flow inference must declare which one is authoritative for each consumer.

## Canonical Refactor Direction

1. Keep `returns.ReconcileFunctionFact` as the only policy entry point for function facts.
2. Keep `interproc.StoreFactsFromResult` and pre-flow return inference writing via reconciliation helpers.
3. Keep compatibility mirrors generated from canonical facts only (`NormalizeFunctionFactChannels`).
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
