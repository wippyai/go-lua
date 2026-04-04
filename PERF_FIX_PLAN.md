# Lint Deadlock Fix: Route Through Salsa Query Layer

## Problem

`wippy lint` deadlocks on large projects (300-900 files). Root cause: expensive type operations called thousands of times without caching, bypassing the existing Salsa-style query system (`types/db/query.go`).

### Hot paths (from profiling keeper, docker-demo, be-common-components)

1. **`PruneSoftUnionMembers`** — 49% CPU. Called from `isSubtype` on every subtype check. Walks entire type tree looking for soft union members. Per-call memo discarded after each invocation.

2. **`ExpandInstantiated`** — was 42% CPU before per-call memo fix. 16 direct calls bypass `Engine.ExpandInstantiated` query.

3. **`applyInferSubst`** — was 75% CPU in constraint solver. No Salsa analog; internal memo added.

4. **`walkType` (`occursIn`, `containsTypeVar`)** — was 68% CPU. No Salsa analog; seen-set added.

5. **`typeContainsNever`** — caused 148GB allocations in docker-demo. No Salsa analog; seen-set added.

## Architecture

The Salsa query layer already exists:

```
types/query/core/Engine
  .IsSubtype(ctx, sub, super)     -> db.Query "IsSubtype"
  .ExpandInstantiated(ctx, t)     -> db.Query "ExpandInstantiated"  
  .Widen(ctx, t)                  -> db.Query "Widen"
  .Field/Method/Index(ctx, ...)   -> db.Query with memoization
```

Interface: `types/query/core/TypeOps` — accepted by synthesizer, hooks, etc.

Problem: 80+ call sites use raw `subtype.IsSubtype()` instead of `Engine.IsSubtype()` because they lack `TypeOps` or `*db.QueryContext`.

## Plan

### Phase 1: Thread TypeOps through hot subsystems

The 80 direct `subtype.IsSubtype` calls cluster in these files:

| File | Count | Has TypeOps? |
|------|-------|-------------|
| returns/join.go | 10 | No — needs threading from pipeline |
| synth/ops/check.go | 6 | Yes — has `synth.Engine` with TypeOps |
| returns/widen.go | 5 | No — needs threading from pipeline |
| flow/query.go | 4 | Has QueryContext |
| constraint/solver.go | 4 | No context — leaf-level |
| flowbuild/assign/error_return_policy.go | 4 | Has scope with TypeOps |
| narrow/narrow.go | 3 | No context — leaf-level |
| flow/transfer.go | 3 | Has QueryContext |
| constraint/infer.go | 3 | No context — leaf-level |
| hooks/assign_check.go | 3 | Yes — hooks receive TypeOps |
| Others (2 each) | ~20 | Mixed |

Strategy per group:

**A. Already have TypeOps (synth/ops, hooks, flowbuild):** Change `subtype.IsSubtype(a, b)` to `ops.IsSubtype(ctx, a, b)`. Mechanical replacement.

**B. Pipeline subsystems (returns/join.go, returns/widen.go):** Thread `TypeOps` from `pipeline.Runner` into return inference. The `Runner` already has `types core.TypeOps`. Pass it down to return join/widen functions.

**C. Flow system (flow/query.go, flow/transfer.go):** Already have `*db.QueryContext`. Need access to `Engine` or expose `IsSubtype` as a query they can call.

**D. Leaf-level (constraint/solver.go, narrow/narrow.go, constraint/infer.go):** These are deep in the type system with no infrastructure access. Options:
  - Accept: these calls are on small types (constraint variables, narrowed results) and less hot
  - Or: pass a subtype checker function as a parameter

### Phase 2: Make PruneSoftUnionMembers a derived query

Current: `isSubtype()` calls `PruneSoftUnionMembers(sub)` and `PruneSoftUnionMembers(super)` every time.

Option A — **Prune-on-construction**: When creating Union types (`typ.NewUnion`), prune soft members immediately. Then `PruneSoftUnionMembers` becomes a no-op. Risk: changes Union construction semantics, may break inference that needs soft members temporarily.

Option B — **Lazy flag**: Add a `hasSoft` bit to Union type. `PruneSoftUnionMembers` checks the flag and returns immediately if false. Set the flag during `NewUnion` construction by checking members. Cost: one bool per Union, one check per member at construction.

Option C — **Query wrapper**: Add `PruneSoft` to `Engine` as a memoized query. Callers going through `Engine.IsSubtype` already benefit. Direct `subtype.IsSubtype` calls still pay the cost.

**Recommendation: Option B (lazy flag)**. It's the most canonical — the type knows about itself, no external caching needed.

### Phase 3: Remove ad-hoc memos that duplicate Salsa

After Phase 1-2, review and remove:

- `subst.go` expand memo pool — **keep**: prevents exponential blowup *within* a single expansion. Salsa caches the final result but not intermediate recursion. This is algorithmic, not caching.
- `constraint/infer.go` `applyInferSubst` memo — **keep**: internal to constraint solver, no Salsa analog. Converts O(n^2) cycle detection to O(n) with result memo.
- `constraint/infer.go` `walkTypeMemo` seen set — **keep**: prevents re-walking same pointer in `occursIn`/`containsTypeVar`. No Salsa analog.
- `returns/join.go` `typeContainsNeverMemo` seen set — **keep**: prevents unbounded recursion. No Salsa analog.

These are not "non-canonical memoization" — they're algorithmic fixes within compute functions. Salsa memoizes query results; these prevent pathological behavior during a single query computation.

### Phase 4: Audit new code for direct bypasses

Add a lint/review rule: new code should use `TypeOps.IsSubtype` not `subtype.IsSubtype` when `TypeOps` is available. The raw function is for the engine's compute function and leaf-level code without infrastructure.

## Files to modify

### Phase 1A — Mechanical (TypeOps already available)
- `compiler/check/synth/ops/check.go` (6 calls)
- `compiler/check/hooks/assign_check.go` (3 calls)
- `compiler/check/hooks/return_check.go` (2 calls)  
- `compiler/check/hooks/field_check.go` (2 calls)
- `compiler/check/synth/ops/call.go` (2 calls)
- `compiler/check/synth/ops/typecheck.go` (2 calls)
- `compiler/check/synth/ops/generic.go` (1 call)
- `compiler/check/synth/phase/extract/*.go` (3 calls)
- `compiler/check/synth/phase/resolve/resolver.go` (2 calls)
- `compiler/check/flowbuild/assign/error_return_policy.go` (4 calls)
- `compiler/check/flowbuild/assign/precision.go` (2 calls)

### Phase 1B — Thread TypeOps into return inference
- `compiler/check/returns/join.go` (10 calls)
- `compiler/check/returns/widen.go` (5 calls)
- `compiler/check/pipeline/runner.go` (pass TypeOps to return inference)

### Phase 1C — Flow system
- `types/flow/query.go` (4 calls)
- `types/flow/transfer.go` (3 calls)
- `types/flow/type_facts.go` (1 call)

### Phase 1D — Leaf-level (defer or pass function)
- `types/constraint/solver.go` (4 calls)
- `types/constraint/infer.go` (3 calls)
- `types/narrow/narrow.go` (3 calls)
- `types/query/core/field.go` (2 calls)
- `types/query/core/index.go` (2 calls)
- `types/query/core/instantiate.go` (1 call)
- `ltype.go` (1 call)

### Phase 2 — Union soft flag
- `types/typ/union.go` — add `hasSoft bool` field, set in constructor
- `types/typ/soft.go` — check flag in `PruneSoftUnionMembers`
- `types/typ/rebuild.go` — ensure flag propagation in NewUnion

## Execution order

1. Phase 2 first (Union soft flag) — biggest impact, self-contained
2. Phase 1A — mechanical, low risk
3. Phase 1B — moderate refactor, high impact (returns/join is hottest)
4. Phase 1C — flow system routing
5. Phase 1D — evaluate if needed after 1-3
