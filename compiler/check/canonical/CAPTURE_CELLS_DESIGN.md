# Capture Cells Design

Status: component design lock under `CANONICAL_SUPER_DESIGN.md`.

Goal: remove the driver-owned capture/upvalue re-solve loop by making captured
lexical cells first-class abstract state in the canonical fixed point.

Current cut: `SetCaptureResolver`, captured-container driver flow-back,
`moduleCaptures`, return-method write caches, closure-method flow-back,
`captureRefinePasses`, and receiver self reseeding are removed from live Go code.
Caller-sensitive capture stores, function identities, and exact entry values flow
through keyed summary entries; entry stores are immutable equation-builder inputs
rather than mutable transfer configuration. The curated oracle is green at
422/422. The component close gate is now a removal audit over the remaining
fallback helpers, not another driver precision pass.

## 1. Current deviation

The old driver handled captured values outside the abstract interpreter:

1. Solve every function.
2. Project a module-wide `moduleCaptures` map from solved point states.
3. Previously: mutate transfers with `SetCaptureResolver` (deleted in the current
   cut).
4. Previously: re-solve up to `captureRefinePasses` for remaining receiver self
   seeds (deleted by the PrototypeSelf migration).
5. Previously: run extra flow-back for closure-installed methods and re-solve
   again.

That shape was a second driver-level iteration with a hard cap and post-solve
transfer mutation. It could preserve useful precision, but the precision lived in
the wrong layer. The current design has moved those facts into product state and
summary keys; remaining work is to delete or justify any compatibility helper
that is still reachable.

## 2. Concrete semantics

Lua closures capture mutable locations, not value snapshots.

For a lexical binding `x`, the concrete runtime has a location `loc(x)`.
Every closure that captures `x` reads and writes the same location. Therefore
capture precision is heap/cell state:

```text
read x     reads Store[loc(x)]
x = e      writes Store[loc(x)] = eval(e)
x.f = e    writes a field inside Store[loc(x)]
call f()   applies f's return effects and cell effects
```

Any abstract design that treats captured values as a post-solve lookup map is
compensating for a missing cell domain.

## 3. Abstract domain

The canonical per-function state should become:

```text
FunctionState = Points x Contracts x CaptureCells
```

or, equivalently, if the cells are point-sensitive:

```text
PointState = Env x Cond x Num x Rel x ReturnRel x CaptureCells
FunctionState = Points x Contracts
```

The preferred integration is point-sensitive cells inside `PointState`: a write
at one CFG point changes the outgoing state of that point, branch joins use the
ordinary point-state join, and loop widening fires at the same feedback vertex
set as the rest of the point state.

The carrier is already introduced as `flow.CaptureCells`:

```text
CellId       = stable CFG symbol id of the lexical binding
CaptureCell  = CellId x product.AbstractValue
CaptureCells = finite deterministic map CellId -> AbstractValue
```

Order and operations:

```text
absent cell = product.Bottom()
c1 <= c2    iff forall id. c1[id] <= c2[id]
join        = pointwise product.Join
widen       = pointwise product.Widen
top         = explicit top sentinel
```

This is the finite-map lifting of the value-product lattice. Since
`product.AbstractValue` has monotone join and ACC-under-widen, the pointwise
cell map inherits those properties.

After the call-effect correction, the implemented point state is:

```text
PointState = Env x Cond x Num x Rel x ReturnRel x CaptureCells x CaptureEffects
```

`CaptureCells` is the current store. `CaptureEffects` is the accumulated
caller-visible transformer used for summaries.

## 4. Transfer rules

Capture read:

```text
evalIdent(x):
  if x is local/param of this function:
    read PointState.Env[x]
  else if x is a captured free variable:
    read PointState.Cells[x]
  else:
    unresolved
```

Capture assignment:

```text
x = e, where x is a captured free variable:
  PointState.Cells[x] = eval(e)
```

Captured container write:

```text
x.f = e, where x is a captured free variable:
  PointState.Cells[x] = product.WithField(PointState.Cells[x], "f", eval(e))
```

Call:

```text
call f(args):
  result values = Summary(f).Returns
  point cells   = apply Summary(f).CellEffects to caller cells
```

The existing closure-method flow-back pass is a special case of the captured
container write plus call-effect rule. It should disappear after call effects are
in the summary.

## 5. Summary boundary

`summary.Summary` should gain caller-visible cell effects:

```text
Summary = Returns x Params x Relations x CellEffects
```

`CellEffects` is a separate `flow.CaptureEffects` carrier, not a raw
`flow.CaptureCells` store snapshot. The distinction matters: a captured cell may
be present in the entry store without being written by the callee. Publishing the
final store as an effect would mistake unchanged cells for writes once entry
capture seeding is complete.

The effect carrier is a deterministic finite map:

```text
CellId -> { must-write(value) | may-write(value) }
absent = identity effect
```

Branch join turns a write present on only some paths into `may-write`. Applying
`must-write(v)` strongly updates the caller cell to `v`; applying
`may-write(v)` joins the caller's old cell value with `v`. Sequential
composition preserves later must-writes as strong updates and composes callee
effects into the caller function's own accumulated effect.

For module-local callees, call typing reads the callee's current summary through
the existing `SummaryQ` cycle. Recursive clusters therefore converge through the
same db query fixed point and `SummaryWiden`.

The entry-cell source is now also summary-owned:

```text
Summary = Returns x Params x Relations x CellEffects x CaptureExports
```

`CaptureExports` is a `flow.CaptureCells` store snapshot projected from normal
return-boundary states. It is intentionally distinct from `CellEffects`:

- `CaptureExports` answers "what values can a directly nested closure initially
  capture from this lexical parent?"
- `CellEffects` answers "what captured cells does a call to this function write
  in its caller?"

`SummaryQ` seeds a child transfer's `PointState.Cells` from its lexical parent
chain's `CaptureExports` before solving the child. That records lexical capture
dependencies in the same summary fixed point instead of a driver post-solve
mutation.

The summary dependency graph must include lexical capture edges in addition to
call edges where a function's entry capture cells are initialized from an
enclosing function's summary. This replaces `moduleCaptures`.

Function identity and exact entry values are now part of the same summary
boundary:

```text
SummaryKey = FuncRef x CaptureCellsKey x FunctionRefsKey x EntryValuesKey
```

`FunctionRefsKey` carries path-sensitive function-valued fields such as
`M.run` or `$0.with_options`. `EntryValuesKey` carries direct call-site argument
values as interned product-domain facts. Aggregate `Summary.CallEntryValues`
remains fallback evidence only for slots not fixed by the explicit summary key.

## 6. Determinism and cache shape

Semantic state must not be represented by source-name maps.

Rules:

- Internal capture identities are `cfg.SymbolID`, not strings.
- Carriers are canonical finite entries sorted by symbol.
- Boundary specs may still use names, but they must be converted once at the
  admission boundary.
- Equality and widening are structural over the canonical carrier.
- No transfer mutator may be required after the query is built.

## 7. Migration steps

1. Land `flow.CaptureCells` and law tests. Done.
2. Add `Cells flow.CaptureCells` to `PointState` and include it in
   `PointStateDomain`. Done.
3. Make transfer read/write captured free variables through `PointState.Cells`
   instead of driver callbacks. Done: transfer reads cells and writes captured
   identifier/field/index assignments into cells; the old mutable resolver
   callback path is gone.
4. Add `CellEffects flow.CaptureEffects` to `summary.Summary` and project it from
   solved return-boundary states. Done.
5. Make call transfer apply callee cell effects and compose them into the
   caller's own accumulated effect. Done.
5a. Add `Summary.CaptureExports` and seed child capture entries from parent
    exports through `SummaryQ`. Done as a transitional canonical source; the
    old resolver fallback is gone.
5b. Move captured container writes into transfer:
    - `table.insert(captured.path, v)` updates the captured cell and emits
      `CellEffects`;
    - `captured.path[k] = v` where `captured.path` is a growable inferred
      container updates the captured cell and emits `CellEffects`.
    Done. This restored the curated oracle from 420/422 to 422/422 without a new
    driver patch. The old driver post-solve captured-container enrichment
    (`table.insert` and indexed-write flow-back over `moduleCaptures`) has now
    been deleted; scoped canonical tests and the curated oracle stayed green.
5c. Make owner-captured locals/params cell-backed in their owning function.
    `input.ScopeFacts.CellSymbols` is the sorted immutable fact set derived from
    direct nested closures' captured symbols. Transfer seeds, reads, writes,
    narrows, and returns those symbols through `PointState.Cells`; owner writes do
    not emit caller-visible `CellEffects`, while free-var writes still do.
    Summary projection now reads returned identifiers from Cells and only exports
    Env symbols explicitly captured by nested closures. Done as the owner-side
    store unification cut.
6. Remove compatibility paths. Done for `moduleCaptures`, closure-method
   flow-back re-solve, return-method write caches, `SetCaptureResolver`,
   `seedCaptureResolvers`, `seedCaptureNarrowBases`, `captureRefinePasses`, and
   receiver self reseeding. Current audit target: aggregate `CallEntryValues`
   fallback and body-contract derivation helpers must remain query-owned summary
   projections or be deleted.

Critical remaining design gap before closing step 6:

- function entry cells now include actual caller-provided capture stores via the
  summary key and lexical parent exports as immutable equation-builder input.
- function values such as `M.run` now have a `FunctionRefs` product/summary lane,
  including return-slot projection and rebasing for returned method tables. The
  remaining question is not "add a driver lookup"; it is whether the remaining
  call-entry fallback helpers are still needed after exact `EntryValuesKey`
  specialization.
- receiver/prototype paths no longer scan or patch `moduleCaptures`; they are
  handled by immutable metatable/prototype facts plus the product-state
  `PrototypeSelf` carrier recorded in `PROTOTYPE_RECEIVER_DESIGN.md`.

## 8. Gates

No global fixtures for this component.

Required scoped gates:

```text
go test -count=1 ./types/flow -run 'Test(CaptureCells|PointStateDomain_Laws)' -timeout 120s
go test -count=1 ./compiler/check/canonical/state -run 'TestFunctionStateDomain_Laws' -timeout 120s
go test -count=1 ./compiler/check/canonical/summary -run 'TestSummary_' -timeout 120s
go test -count=1 ./compiler/check/canonical/... -timeout 120s
go test -count=1 -v -run 'TestCanonicalCurated(Oracle|Gate)' . -timeout 240s
```

Add focused component tests for:

- captured read uses cell value;
- captured assignment updates cell value;
- captured field/index write updates the cell value;
- callee summary cell effects update caller cells;
- recursive capture effects converge by widening, not by a driver pass cap.
