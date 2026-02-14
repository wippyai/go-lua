# Canonical Function Facts Design

## Goal

Eliminate parallel implementations of function typing state and make one canonical path for:

- pre-flow return evidence
- post-flow return evidence
- callable function type used by all consumers

This design targets correctness, determinism, and maintainability. It is intended to remove whole classes of divergence bugs (for example, one channel saying `proxy:nil` while another says `proxy?:Proxy`).

## Current Design Flaw

Today, the checker carries multiple partially-overlapping channels for the same function symbol:

- `ReturnSummaries` (pre-flow)
- `NarrowReturns` (post-flow)
- `FuncTypes` (callable signature)

These channels:

- are produced at different points in the iteration
- are consumed by different phases
- can be merged by different code paths

As a result, they can diverge and produce phase-dependent behavior. This increases false positives and makes fixes fragile.

## Canonical Abstraction

Introduce a single canonical lattice value per function symbol:

`FunctionFact`

Fields:

- `SignatureBase *typ.Function`
  - Parameter shape and non-return metadata (type params, params, variadic, spec/effects metadata ownership policy).
- `ReturnsPre []typ.Type`
  - Pre-flow inferred return vector.
- `ReturnsNarrow []typ.Type`
  - Post-flow refined return vector.
- `SignatureEffective *typ.Function`
  - The only function type consumed by phases and export logic.

Only one kernel may update this value:

- `returns.ReconcileFunctionFact(...)`

All other code becomes producer/consumer of the canonical fact, not owner of merge policy.

## Invariants (Hard Requirements)

For each function symbol `s` in a graph snapshot:

1. Single-writer policy
   - `FunctionFact` is updated only through kernel APIs.
2. Signature consistency
   - `SignatureEffective` returns must equal canonical merge of `ReturnsPre` + `ReturnsNarrow` + existing signature returns policy.
3. Monotone convergence
   - successive updates do not lose previously-proven return evidence.
4. Phase-stable read model
   - scope/narrow/export phases read the same canonical fact object (phase may choose view, not different storage).
5. Deterministic snapshots
   - no map iteration nondeterminism in merge/output ordering.

If any invariant fails in tests, build must fail.

## Canonical Merge Policy

Kernel policy is centralized and shared:

- normalize vectors (`nil -> typ.Nil`, prune soft members)
- prefer directional refinement where valid
- preserve nil-slot semantics for Lua multi-return arity
- for higher-order recursion risk, use monotone join
- compute one `SignatureEffective` from the reconciled return evidence

No consumer may apply local return merge rules.

## Storage Model Changes

Replace triplicated maps in `api.Facts`:

- remove independent ownership of:
  - `ReturnSummaries`
  - `NarrowReturns`
  - `FuncTypes`
- add:
  - `FunctionFacts map[cfg.SymbolID]returns.FunctionFact`

Compatibility adapters may exist temporarily, but must be read-only derived views and deleted after migration.

## Pipeline Consumption Rules

1. Return inference
   - emits candidate updates to `FunctionFact` only.
2. Scope phase
   - function symbols resolved from `SignatureEffective`.
3. Narrow phase
   - contributes `ReturnsNarrow` candidates via kernel.
4. Export path
   - never re-derives function signatures independently.
   - uses canonical `SignatureEffective` for exported functions.
5. Hooks/synth/call typing
   - use canonical function fact lookup path only.

## Migration Plan

### Phase 0: Guardrails (no behavior change)

- Add invariant tests that compare existing channels for equivalence where expected.
- Add deterministic snapshot diff tests.

### Phase 1: Canonical fact introduction

- Add `FunctionFact` type and kernel-based storage.
- Dual-write old + new channels.
- Add assertions that old channels are derivable from new facts.

### Phase 2: Reader migration

- Move scope/phase/synth/export readers to `FunctionFact`.
- Keep old maps only as derived compatibility views.

### Phase 3: Deletion

- Remove old parallel maps and helper duplication.
- Remove dead wrappers and duplicate merge utilities.

## Test Strategy

### Kernel Law Tests

- idempotence: `merge(x, x) == x`
- commutativity where required
- monotonicity across iterations
- nil-slot arity behavior
- higher-order widening behavior

### Integration Regression Tests

Add/keep regression suites reproducing real failures:

- views class (`page.proxy` and related field narrowing/indexing)
- docker class (`any row`, optional string narrowing, structural table returns)
- session/agent/llm class where previously observed phase/channel drift existed

### Harness Gates

`scripts/verify-suite.sh` remains required and must include:

- go-lua checker tests
- wippy targets per-directory (not whole repo glob)
- explicit fail on non-zero errors

### Determinism Gate

Run selected lint targets multiple times from fresh binary; counts must be stable.

## Deletion Targets (Complexity Reduction)

After migration, delete:

- duplicated return merge call sites outside kernel
- phase-local function return reconciliation helpers that bypass kernel
- compatibility wrappers once all readers are canonical

This is where maintainability gains come from: fewer paths, fewer policies, fewer surprises.

## Success Criteria

1. One canonical function fact path across phases.
2. Existing false-positive classes reduced without broad regressions.
3. Harness counts stable across reruns.
4. Less code and fewer policy forks than current design.

