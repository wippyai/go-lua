# Canonical Abstract Interpreter Super Design

Status: design lock for the canonical rewrite. The design target is a single
canonical product fixed point. Query decomposition is allowed only as an
implementation strategy that preserves the same equations and dependencies.

Goal: remove driver-owned precision mechanisms by making every analysis fact live
in exactly one of five places:

1. immutable input facts derived before solving;
2. an abstract-domain component of the product state;
3. a monotone transfer over that product state;
4. a summary transformer/projection computed by the same fixed point;
5. a read-only observation/projection boundary from the solved product into
   diagnostics, manifests, or public API shapes.

Anything outside those buckets is architectural debt. A post-solve driver pass, a
mutable transfer callback installed after solving, a hard iteration cap, or a
source-name map used as semantic state means the abstraction is missing.

## 0. Layer contract

"Stages" in the canonical engine are architectural layers, not independent
precision passes:

1. **Extraction** lowers source syntax to immutable graph/fact inputs. It may
   name symbols, refs, points, slots, and source contracts. It may not infer a
   solved runtime value.
2. **Product domains** define abstract carriers, orders, joins, widenings, and
   deterministic keys. If a runtime precision fact is not representable here, the
   design is incomplete.
3. **Transfer** is a monotone equation over the product state. It consumes only
   immutable inputs and current cell values.
4. **Solver** runs the single Kildall/Cousot fixed point. Query decomposition is
   an evaluation strategy for this one system.
5. **Summary** projects caller-visible facts from solved states and feeds them
   back only as dependency-tracked cells in the same system.
6. **Observation/diagnostics** are pure projections from solved product facts
   into legacy/public type shapes. They may lose precision only by documented
   projection, never by recomputing or rediscovering facts in the driver.

The direction is one-way except for summary cells that are part of the fixed
point. If a later layer needs precision that an earlier layer did not represent,
the fix is to add the missing carrier/transfer/projection, not to patch the later
layer with a source-case walk.

## 1. Paper constraints

This design follows the standard abstract-interpretation shape:

- Kildall: solve a finite system of monotone data-flow equations over cells by a
  deterministic worklist.
- Cousot-Cousot: soundness is by concretization; infinite ascending chains are
  cut by domain widening at feedback vertices, not by pass caps.
- Sharir-Pnueli: interprocedural precision requires valid call/return flow by
  summary transformers or an equivalent supergraph, not function bodies patched
  after a local solve.
- Astree-style products: precision comes from cooperating domains and explicit
  reductions, not from the driver recognizing individual fixture patterns.
- Salsa/query friendliness: inputs are immutable, keys are stable identities,
  carriers are canonical deterministic values, and cached queries do not depend
  on mutable closures injected after construction.

The semantic object being computed is one product fixed point, or its sound
widened post-fixpoint when a component has infinite ascending chains:

```text
lfp(F) over Cell -> ProductCellValue
```

`Cell` ranges over program-point state, parameter demand, context entry evidence,
and summary projection cells. `F` is monotone and deterministic. Infinite-height
cycles are widened at declared feedback vertices, with delayed widening allowed
only as a precision policy: the first strict updates use exact join, then the
domain widening enforces convergence if the chain keeps growing. An
implementation may memoize parts of `F` as db queries, but those queries are
views/evaluation strategy only: they must record the same dependencies, use the
same bottom/widening discipline, and may not introduce a second convergence
criterion, post-solve mutation, or driver-owned precision pass.

Primary references used for this lock:

- https://cs.nyu.edu/~pcousot/COUSOTpapers/POPL77.shtml
- https://doi.org/10.1145/512927.512945
- https://cds.cern.ch/record/120118
- https://www.astree.ens.fr/
- https://www.di.ens.fr/~feret/publications/asian2006.shtml
- https://salsa-rs.github.io/salsa/reference/algorithm.html

## 2. Global invariant

The driver may build the module, construct immutable facts, start the solver, and
bridge final results to the legacy diagnostics. It may not improve precision.

Forbidden in the final canonical system:

- `moduleCaptures` as a solved semantic map;
- `SetCaptureResolver` or any mutable transfer callback installed after solving;
- `captureRefinePasses` or any pass-count convergence bound;
- closure-method flow-back after summaries have been projected;
- fixture-specific graph walks that rewrite types after the product solve;
- `map[string]typ.Type` as internal semantic state. String maps are allowed only
  at external boundaries such as specs/manifests and must be converted once to
  symbol/ref keyed facts.
- `map[string]product.AbstractValue` as internal flow state. Encoded strings may
  exist only at parser/API boundaries; the carrier is keyed by typed identities.
- AST-pointer registries in production call resolution. Function literals remain
  diagnostic/bridge keys, but solver-facing identity must be `FuncRef`/symbol
  facts so query keys are stable and cache-friendly.

Required replacement shape:

```text
Program inputs
  -> immutable facts
  -> one monotone product equation system
  -> lfp over Point/ParamDemand/Entry/Summary cells
  -> projections from the fixed point
  -> diagnostic/result bridge
```

## 3. Current architecture

The current engine already has the right core pieces:

- `types/lattice/solver` is the deterministic Kildall/Cousot worklist.
- `compiler/check/canonical/equation` solves one function as
  `PointCells x ParamContractCells`. It may not run transfer in a fake discovery
  pass. Point loop/FVS cells and parameter-demand cells are normal solver cells;
  the solver exact-joins a widening cell's pre-visit fan-in, then delayed
  widening protects the first post-visit joins before Cousot widening
  accelerates continuing cycles.
- `flow.PointState` now includes value env, condition, numeric state, relations,
  capture cells/effects, closure/function refs, static member facts, and key
  presence facts. The value env is keyed by typed
  `flow.ValueKey`, not anonymous strings; the encoded string form is only the
  deterministic cache representation behind constructors such as
  `flow.SymbolValueKey`.
- `summary.Summary` now includes returns, parameter contracts, return relations,
  capture effects, capture value exports, capture function-identity exports,
  capture closure-value exports, returned-function identity tuples,
  returned-closure tuples, prototype self, caller-to-callee entry value evidence,
  and body-proven parameter narrowing effects.
- `summary.Queries` now has one recursive summary solve per context.
  `Summarize` drives the interprocedural fixed point; `Intra` is an exact local
  Kildall observer over the converged Summary dependencies, not a second
  memoized interprocedural fixed point. This preserves point precision under the
  current revision-granular db engine while avoiding the old stale `IntraQ`
  cache.

This is good, but not yet the complete supergraph. Each remaining precision gap
must now be closed by adding an immutable fact, a product-state component, a
transfer, or a summary projection. A new `PointState` axis is allowed only when
the carried fact has its own lattice and interprocedural meaning; boundary facts
that already denote ordinary values must enter the existing value/cell product.

## 4. Canonical cells

The final equation universe is indexed by a stable summary context:

```text
Ctx = (ref, entryCellsKey, entryFunctionRefsKey, entryClosureRefsKey,
       entryValuesKey)
```

For each reachable context, the cells are:

```text
Point(Ctx, point)                         -> flow.PointState
ParamDemand(Ctx, slot)                    -> paramevidence.ParamContract
EntryCells(Ctx)                           -> flow.CaptureCells
EntryFunctionRefs(Ctx)                    -> flow.FunctionRefs
EntryClosureRefs(Ctx)                     -> flow.ClosureRefs
EntryValues(Ctx, slot)                    -> product.AbstractValue
CallEntryValues(Ctx, callee, slot)        -> product.AbstractValue
ParamNarrows(ref)                         -> []paramevidence.ParamNarrow
Summary(Ctx)                              -> summary.Summary
```

These are one product equation system. Splitting them into a per-function
builder plus summary query is correct only if the split is observationally
equivalent to solving these cells together: reads are registered as dependencies,
recursive cycles start at bottom, widening is owned by the relevant domain, and
no driver code feeds a solved result back into a later solve.

`Point(Ctx, point)` is the normal CFG out-state for one function/context.

`cfg.Point` itself is not an analysis dimension. It is CFG identity only. If a
new dynamic precision fact is needed at a point, it must be added to
`flow.PointState` as a product-domain component with its own carrier, order,
join/meet/widen behavior, transfer, and projection/query boundary. If the fact is
static source/oracle/config data, it belongs in immutable facts or transfer
configuration before solving. Adding semantic axes directly to `cfg.Point` is a
driver workaround and is forbidden.

`ParamDemand(ref, slot)` is backward demand from body uses to entry.
Contract cells are read only by the entry equation. A changed demand cell
re-runs entry, and any changed entry out-state then propagates through ordinary
CFG predecessor dependencies. Non-entry points receive no direct contract side
channel; this keeps the dependency graph contract -> entry -> CFG rather than
contract -> every point, and avoids using broad contract reads as a precision
mechanism.

`CallEntryValues(caller, callee, slot)` is observed caller-to-callee value
evidence for ordinary parameters. It is value-state, not a separate precision
domain and not a new `PointState` axis: the caller transfer evaluates the actual
argument in solved `PointState`, `summary.CallEntryValueProjection` projects it
into the caller's `Summary`, and the summary solve query folds caller summaries
into callee `equation.Builder.WithEntryValues`. Direct call-site entry keys use
`summary.DirectCallEntryValues`. The canonical driver only supplies graph/type
callbacks such as callee resolution, source-slot mapping, annotation status, and
argument evaluation; it does not own the entry-value join, fixed-key fallback
rule, or projection algebra. The callee entry transfer writes those values into
the same Env/Cells locations declared parameters and contracts use. The old
transitional `facts.InferredParamSlots` pre-solve lane has been removed.

`summary.AggregateEntryValues` must be dependency-lazy: it may read caller
summaries only when the callee has an inferred slot, and only for actual static
callers of that callee. Context-sensitive direct and higher-order calls pass
exact product entry values through `EntryValuesKey`; they are not recovered by
polluting the fallback graph with every function summary. The aggregation may
read prototype self publisher summaries only when an undeclared receiver slot can
consume them, and only for publishers indexed under that receiver's prototype
symbol. Eagerly touching every summary, or every prototype publisher regardless
of receiver prototype, is a cache/fixpoint pollution bug because it adds spurious
summary-solve dependencies and can change widening precision.

Provider APIs may not accept whole `Summary` callbacks for these folds. The
query layer owns summary cells; driver providers receive only typed component
readers such as `CallEntryValues(dep, callee)`, `PrototypeSelf(dep)`,
`CaptureExports(dep)`, and `CaptureFunctionRefs(dep)`. This keeps the equation
surface honest: adding a dependency on one summary component cannot accidentally
inspect or couple to unrelated axes.

Direct call sites may also pass exact caller-projected values directly in the
summary key through `summary.EntryValuesKey`. In that case aggregate
`CallEntryValues` is fallback evidence only for slots not fixed by the explicit
key. This keeps context-sensitive summaries Salsa/cache friendly: the semantic key
is an interned product-value map, not `map[string]typ.Type` and not a mutable
driver side table.

Entry evidence and body obligations have different provenance:

- entry evidence describes runtime values already observed at call boundaries;
- `ParamDemand` describes obligations the body imposes on acceptable entries.

If precise entry evidence is disjoint from a body obligation, the contradiction is
body-local and must not also be published as a caller contract. The bridge uses
`paramevidence.EntryContradictsBodyContract` against the solved summary entry
evidence for this provenance rule, not against a driver-side type guess.

The parameter-obligation algebra lives in `compiler/check/domain/paramevidence`,
not in the canonical driver. That package owns source-slot projection, forwarded
typed-callee obligation extraction, entry-vs-body contradiction handling, and
signature rewriting. The driver may provide module services such as "resolve this
callee under this expression observer"; it may not implement a second parameter
analysis in place.

`EntryCells(ref)` is the join of all capture-cell stores that may enter this
function:

- lexical parent exports for closure creation;
- actual caller cell stores at call sites;
- static boundary facts such as imported module aliases. These aliases are stored
  as sorted `topology.ModuleAlias{Symbol, Type}` facts indexed by
  `canonical/facts`, not as a driver-owned `map[SymbolID]Type` cache.

`EntryFunctionRefs(ref)` is the corresponding function-identity path state for
captured values. A function-valued path (`M.run`, `M.dep.get`) is not a driver
side table: it is a point-state axis with a finite may-set lattice. When such a
path crosses a function boundary through a captured cell, it must be part of the
summary context key alongside `EntryCells`; otherwise the callee body sees only
the structural `fun() -> any` value and loses the summary-sensitive return.
Lexical parent exports feed this entry state through `Summary.CaptureFunctionRefs`;
call-site captures feed it through the `FunctionRefs` key carried by
`summary.NewKeyWithRefs`.

Closure values need one more distinction than bare function identities. A nested
function value is not just `FuncRef`; it is `(FuncRef, captured-cell store,
captured-function-ref store, captured-closure-ref store)` at the creation point.
A returned/stored closure
must carry that environment with the value, otherwise a later call can resolve
the body identity but cannot reconstruct the lexical cells the body reads. The
canonical shape is a point-state product axis, not a driver fallback:

```text
ClosureRefs(Ctx, path) -> finite set of
  { ref: FuncRef,
    entryCells: CaptureCellsKey,
    entryRefs: FunctionRefsKey,
    entryClosures: ClosureRefsKey }
```

Transfer creates a closure ref for a function literal from the current
`PointState.Cells.Project(capturedSymbols(ref))` and
`PointState.FunctionRefs.Project(capturedSymbols(ref))`, and
`PointState.ClosureRefs.Project(capturedSymbols(ref))`. Assignment/table-field
flow rebases it the same way `FunctionRefs` is rebased today. Summary projection
exports returned closure refs slotwise; call transfer consumes them to call
the summary solve under the closure's captured entry keys. `FunctionRefs` remains the
coarse identity fallback for declarations/imported signatures and for values
whose closure environment is unknown. This is the required replacement for any
bottom-context lexical-parent summary read that tries to infer a closure's
environment after the value has escaped.

Callable observation is also a product projection, not a driver typing rule:

```text
CallableType(Ctx, path):
  if ClosureRefs(Ctx, path) has finite closures:
    join summaries under each closure's captured entry context
  else if FunctionRefs(Ctx, path) has one body ref:
    summarize under the ambient point-state entry product
  else:
    unknown
```

This projection lives behind `canonical.callableProjector`. Diagnostics may read
the resulting `typ.Function`, but they may not re-query summaries through a
type-only `FunctionRef` path when a `ClosureRef` exists.

Ordinary path observation is likewise a product projection:

```text
PathValue(Ctx, path):
  if path is root:
    project Env/Cells value, preserving gradual-top provenance
  else:
    value := walk product members from the root value, consulting StaticMembers
      at each prefix and numeric length proofs only for final sequence-index
      refinement
    callable := CallableType(Ctx, path)
    if callable is known and value is known:
      refine the callable arm of value while preserving nilability/presence
    else if callable is known:
      project callable signature
    else:
      project value
```

This projection lives behind `canonical.pathProjector`. It may read solved
`PointState` product axes, but it may not inspect source syntax, consult driver
side maps, or add branch-specific cases. If a path loses precision here, the fix
is an upstream product carrier/transfer/summary projection, not another
observation fallback.

Callable identity is not a presence proof. `FunctionRefs` / `ClosureRefs` may
sharpen a value already known to be callable, but they must not erase ordinary
path-read nilability. A declared map read like `handlers["missing"]` remains
`Handler?` unless the product state carries a must-present static member/key
proof for that exact read; a finite function identity at the same path refines
only the non-nil callable arm to the solved signature.

Key-presence observation is a must-fact query over solved product state:

```text
KeyPresence(Ctx, table, key):
  read PointState.KeyPresence[(table, key)]
KeyValueOrigin(Ctx, table, key, value):
  read PointState.KeyPresence[(table, key, value)]
```

`PointState.KeyPresence` is a finite must-set of structural table/key path pairs,
table/key/value origin triples, and key-array provenance facts
`keys(array) -> table`. Generic-for transfer seeds direct presence for
`pairs(table)`, seeds value-origin when the paired value is exactly `table[key]`,
and seeds indexed presence for `ipairs(array)` only when live product state still
proves the array currently contains keys of a table. Assignment to the table, any
overlapping table member, key, value, or key-array path kills the dependent facts;
`table.insert(array, ...)` kills key-array provenance too. Join and widen
intersect the sets, so a proof survives only when every predecessor proves the
same pair/triple/array relation. Exact dynamic-index transfer and diagnostics
consume this product axis. They may not scan `Cond` disjuncts or the CFG for
keyed-iteration provenance, and canonical transfer may not duplicate the same
runtime provenance into `Cond` as compatibility storage. Those would be
driver-owned precision policy instead of components of the product fixed point.

Index-write admission is a point-local proof event, not durable store state:

```text
IndexWriteAdmission(Ctx, point, target, key, value):
  read PointState.Points[point].IndexWrites
  match target/key/value by normalized symbol/path keys and product key value
  return the admitted product value projected to typ.Type
```

`PointState.IndexWrites` is a finite must-set of dynamic-index replacement writes
the canonical transfer admitted through the value-domain write law. Transfer seeds
it only in the write reducer after a final dynamic index write (`t[k] = v`) passes
`product.IndexWriteAdmits` (or the self-derived write law) and the declared target
prefix is not sealed by a non-refinable annotation. It does not seed for static
member/index writes, nested mutations (`t[k].f = v`), unresolved keys, or
unknown/`any` proof values. Because the fact is an event, `Transfer` clears the
axis before interpreting each node; only writes performed at point `p` can appear
in `out[p]`, and the proof cannot leak to successors as store state. Join and
widen intersect the set, so an admission survives only when every incoming path
proves the same write identity.

Observation consumes this through `flow.IndexWriteFacts`. Legacy `flow.Solution`
implements the same interface from `MapMutatorAssignment` rows; canonical facts
implement it from `PointState.IndexWrites`. Observation must not call a concrete
producer (`Solution`) directly and must not rebuild write legality from syntax.

Value provenance is a separate finite must-set axis, not another table-key case:

```text
ValueOrigin(Ctx, value_path):
  read PointState.ValueOrigins origins whose value path covers value_path
  lift the remaining suffix through the origin projection
```

`PointState.ValueOrigins` records derived locals such as iterator targets:
`entry <- ipairs(tests)[1]`, `key <- pairs(table)[0]`,
`value <- pairs(table)[1]`. Backward demand from a typed use of a derived value
uses prefix lookup, so `entry.id` is handled by the same origin as `entry`; the
suffix `.id` becomes structural evidence before `IndexedIteratorEvidence` or
`KeyedIteratorEvidence` lifts it back to the source parameter. Writes to either
the derived value path or the source path kill the origin. This is the generalized
replacement for sorted-key/local-helper cases; no driver code may special-case a
function name, fixture shape, or source line.

The required tests for this component are family tests, not oracle examples:
`ValueOriginFacts` must pass lattice laws, canonicalize deterministically, keep
all covering prefix origins for a consumed path, lift obligations through nested
source paths, and be invalidated by the generic `WriteEffect` reducer. Unknown
or `any` keyed-iteration evidence must not fabricate `{[any]: any}` as a mutable
map contract; it stays absent until the read-only iterable carrier below exists.

Entry seeding is likewise a reducer:

```text
EntrySeed(slot):
  compose declared annotation, exact caller entry value, and body-demand contract
```

Structural annotations with dynamic interiors (`{any}`, `any[]`, maps whose
key/value slots are refinable) are contracts, not precision erasers. Exact caller
entry values may refine those interiors through `EntrySeedEffect`; body contracts
must be applied with body-entry contract composition, not ordinary type/product
LUB, because ordinary LUB lets `any` absorb required path evidence.

Returned function identities are also a Summary component:
`Summary.ReturnFunctionRefs` is a slotwise tuple over `flow.FunctionRefs`.
Combining several possible callee summaries at one call site must use
`summary.JoinReturnFunctionRefs`, not a driver-local tuple lattice. The tuple
canonicalization rule is the same as return values: absent trailing slots denote
bottom and are trimmed.

Return-relation extraction is owned by the `flow.ReturnRelations` domain. A
function contract/type projects to a caller-visible relation through
`flow.ReturnRelationsFromSpec` / `flow.ReturnRelationsFromFunctionType`; the
driver must not parse `effect.ErrorReturn` labels or union members directly.
Union projection is a must-fact operation: nil alternatives are ignored, every
non-nil function alternative must prove the same finite relation, and any
non-function alternative falls back to `Top`.

`Summary(ref)` is the caller-visible abstraction of the solved function.

Parameter narrowing effects belong here. A wrapper such as
`function expect(x) if not x then error() end end` proves a fact about its
argument on every normal return; a caller consumes that fact at the call transfer.
That is interprocedural function behavior, not point-local runtime state. The
effect is therefore projected into `Summary.ParamNarrows`, and transitive wrapper
inheritance is computed by the `ParamNarrows(ref)` product cell as part of the
same dependency graph as returns, relations, cells, and entry evidence.
`ParamNarrows(ref)` is keyed only by `FuncRef` because the exported effect may
mention only parameter placeholders and every-normal-return proof obligations;
it cannot depend on caller entry values or capture cells. Transfers that need
call-site narrowing read this typed cell directly, not a full bottom-context
`Summary`. The old `facts.CloseParamNarrows` side closure is forbidden: it was a
second interprocedural fixed point hidden in facts construction.

The parameter-effect export boundary is a projection, not a source-name map.
`flow.ParamInfo.Symbol` is the canonical parameter identity. Root-name fallback
is accepted only for unresolved paths with `Symbol=0`, so a callee local with the
same spelling as a parameter cannot be exported as that parameter. Exported
constraints may mention only parameter placeholders (`$N`) and explicit return
paths (`ret[N]`); any path-pair constraint that would leak a callee-local or
callee-global path is projected away. This keeps summaries portable, deterministic,
and suitable as Salsa/cache values.

`domain/paramevidence` owns that export/import projection. The driver may read a
callee summary or imported signature, but it must not translate between
`ParamNarrow` and `FunctionRefinement.OnReturn` itself.

The existing per-function builder handles point and demand cells for one context.
The complete interprocedural migration must either extend it to a program-level
equation system, or keep an equivalent summary-query evaluation whose value
includes every entry/context input and whose dependencies are immutable query
reads, not driver mutation. If a precision fact needs a second pass over solved
states, the design is wrong: the fact is missing from immutable inputs, product
state, transfer, or summary projection.

## 5. Transfer contract

Transfer is the only place source semantics may change product state.

Identifier read:

```text
if symbol is cell-backed:
  read PointState.Cells[symbol]
else:
  read PointState.Env[symbol]
```

Assignment:

```text
if target symbol is cell-backed:
  Cells[symbol] = eval(source)
  CellEffects = CellEffects.Then(must-write(symbol, value))
else:
  Env[symbol] = eval(source)
```

Field/index/container mutation:

```text
base = cell-or-env root value
updated = product container transfer(base, key/path, value)
write updated back to the same root store
emit CellEffects when the root is cell-backed
```

Call:

```text
actuals, current cells -> callee EntryCells/ParamDemand contributions
callee Summary -> caller returns, relations, and cell effects
caller Cells = Summary.CellEffects.Apply(caller Cells)
```

When one call site can trigger multiple module-local callees or callback effects
and their relative execution order is unknown, the join is
`flow.CooccurringCaptureEffects`: `join(a.Then(b), b.Then(a))`. This algebra
belongs to `flow.CaptureEffects`, not the canonical driver. The call-site fold
over direct callee summaries and callback summaries is
`summary.AggregateCellEffects`; it owns deterministic callback-parameter
ordering, method-call callback argument indexing, and callback-cardinality
weakening (`CardExactlyOnce` keeps must-write; every other cardinality becomes a
may-write). The driver may resolve concrete callee/callback refs, but it must not
inline this aggregation policy.

Generic-for iterator classification and loop-variable projection live in
`domain/iteration`. The driver may resolve the iterator callee and source
argument expression, but it must not own Iterator-effect extraction, `pairs` /
`ipairs` fallback, dynamic-any policy, keyed-container soundness, or present-entry
key/value projection. The same owner handles key-presence provenance: keyed
`pairs` source extraction, indexed `ipairs` over a keys-collector result,
single-assignment gating, keys-collector return-slot matching, method receiver
runtime-argument mapping, and static container path projection. Keyed iteration
declines a closed field-only record because there is no single map-like
present-entry relation for every yielded slot.
Builtin iterator fallback is admitted only from normalized binding identity:
`domain/iteration.BuiltinName` requires a recorded `SymbolGlobal` for exactly
`pairs` or `ipairs` with matching recorded name. A local, parameter, upvalue, or
unbound raw identifier with that spelling is not a builtin; it must expose an
Iterator contract to affect the single product fixpoint.

Runtime-argument-sensitive return effects live in `domain/callreturn`. The
canonical driver, observation projector, and legacy synth extraction may provide
the completed call tuple plus callee/receiver/runtime argument facts, but they
must not each rebuild effect argument vectors and walk return slots locally. That
owner applies contract/error-return/flow-into/selector transforms copy-on-write
and keeps method receiver shifting deterministic across all call surfaces.

No call transfer may ask the driver for a solved module-wide type. If a callee
needs caller context, that context must be a cell in the equation system.

## 6. Capture/upvalue component status

Done:

- `flow.CaptureCells` finite-map lattice.
- `flow.CaptureEffects` transformer lattice with `Apply`, `Then`, and
  `CooccurringCaptureEffects`.
- `PointState.Cells` and `PointState.CellEffects`.
- `Summary.CellEffects` and `Summary.CaptureExports`.
- The summary solve query seeds child entries from lexical parent CaptureExports.
- Captured identifier/field/index writes update cells/effects.
- Captured `table.insert(root.path, v)` and captured nested dynamic-index writes
  now update cells/effects in transfer, not driver enrichment.
- The old driver post-solve captured-container enrichment for those writes has
  been removed.
- Owner-captured locals/params are now `CellSymbols` facts and live in
  `PointState.Cells` in their owning function. Transfer and narrowing read/write
  them through Cells, return projection consults Cells, and observation sees Cells
  before legacy capture fallback.

Verified after the latest cut:

```text
go test -count=1 ./compiler/check/canonical/... -timeout 120s
go test -count=1 -v -run 'TestCanonicalCurated(Oracle|Gate)' . -timeout 240s
```

The curated oracle is back to 422/422.

Still not final:

- child entry cells come from lexical exports, but complete call-context
  propagation belongs in `EntryCells(ref)`;
- `EntryCells(ref)` cannot be a static initializer snapshot. A deletion experiment
  proved that static bootstrap regresses method-table/prototype precision by
  joining initial nil fields into summaries. The current cut replaced
  `SetCaptureResolver` with caller-store keyed summaries and made entry seeding
  immutable equation-builder input.
- `moduleCaptures`, `SetCaptureResolver`, `captureRefinePasses`, receiver self
  reseeding, prototype enrichment, return-method write caches, and closure-method
  flow-back are removed from live Go code. Remaining mentions are documentation
  or comments used to describe the migration history.
- Done: static prototype method identities live in `canonical/facts` alongside
  metatable indexes, method receivers, and setmetatable sites. The driver no
  no longer builds a separate joined method-surface table; transfer receives
  deterministic `PrototypeMethod` topology through `transfer.Config` and
  publishes runtime callable availability into the `FunctionRefs` product axis.

## 7. Migration order

One component at a time. A component is closed only when its legacy driver path is
removed and the scoped oracle remains green.

1. Capture/upvalue cells.
   - Finish cell-backed owner locals.
   - Done: add call-site capture entry propagation and delete
     `SetCaptureResolver`.
   - Done: move entry seeding out of transfer mutation.
   - Done: delete `captureRefinePasses`.
   - Done: delete `moduleCaptures`, return-method write caches, and
     closure-method flow-back from live Go code.
   - Done: audit the aggregate `CallEntryValues` fallback and body-contract
     derivation helpers. Entry-value projection/fallback now lives in
     `summary/entry_value_projection.go`; body-contract algebra lives in
     `domain/paramevidence/body_contract.go`. Neither is allowed to become a
     second solver, and the driver only supplies immutable facts and resolver
     callbacks.

2. Prototype/metatable receiver semantics.
   - Done: design and implementation recorded in
     `PROTOTYPE_RECEIVER_DESIGN.md`.
   - Done: move static `__index`/method-receiver/setmetatable-site evidence into
     immutable facts keyed by `cfg.SymbolID`, `ref.FuncRef`, `cfg.Point`, and
     slots.
   - Done: add a product-state carrier for `PrototypeSelf`: prototype symbol to the
     runtime instance `self` value, pointwise over `product.Domain`; summaries
     carry/project it interprocedurally.
   - Done: update `PrototypeSelf` in transfer for `setmetatable(instance, mt)` and
     `self.field = value`; summary projection must not rediscover those source
     constructs after solving.
   - Done: type method slot 0 through immutable equation-builder entry values
     derived from receiver summary dependencies, not `SetInferredParams`.
   - Done: delete `seedMethodSelfFromCaptures`, `enrichPrototypeReceivers`,
     `joinSelfFieldWrites`, and the receiver reason for the old driver re-solve
     loop.

3. Callback/env overlays.
   - Done: keep string-keyed spec overlays only at the contract boundary.
   - Done: callback environment overlay inference lives in
     `compiler/check/domain/callbackenv`. Canonical facts, synthesis, and
     function-fact solved-signature projection import the domain owner directly
     instead of reaching through legacy function-fact compatibility wrappers.
   - Done: replace the anonymous `map[int]map[string]typ.Type` overlay API with
     deterministic `callbackenv.Overlays` / `callbackenv.Overlay` carriers.
     `callbackenv.GlobalName` aliases `domain/globalenv.Name`, the shared
     source-global identity carrier used before canonical symbol lowering. It is
     explicitly documented as an external contract/API name, not a solver
     identity.
   - Done: `domain/globalenv` owns the source-global overlay algebras.
     `TypeOverlay` carries external source-name type bindings;
     `ValueOverlay` carries analysis-context abstract values. `MergeTypeOverlay`
     joins duplicate evidence facts, `OverrideTypeOverlay` models
     context-local/global-env rebinding at API and legacy phase boundaries, and
     `MergeValueOverlay` converges callback/context value evidence with
     `product.CarryForward`. Callers no longer hand-roll string-map merge,
     projection, equality, lookup, or hashing loops.
   - Done: convert overlays to symbol/ref keyed `CallbackEnvEntry` facts before
     solving, including deterministic nested callback propagation.
   - Done: lower callback overlay names immediately at the callback target graph;
     canonical propagation now uses `FuncRef -> SymbolID -> Type` staging and
     joins duplicate symbol facts before producing sorted `CallbackEnvEntry`
     values. The only remaining string maps are external contract/global-type
     boundary shapes; analysis-facing callback overlay state is typed and sorted.
   - Done: seed those facts through `EntrySymbolValues` at the callee entry
     point, so ordinary product-state transfer owns the values. This is not a
     separate `PointState` axis because callback env entries are runtime symbol
     values, not an independent abstract domain.
   - Done: `EntrySymbolValues` is a static provider API only. It receives no
     `summaryOf` callback, so immutable callback/global entry seeding cannot
     create bottom-context summary dependencies or smuggle precision through the
     interprocedural driver.

4. Caller argument entry evidence.
   - Done: add `Summary.CallEntryValues` as a deterministic finite-map summary
     component keyed by callee ref and graph slot.
   - Done: project actual argument `product.AbstractValue`s from solved caller
     `FunctionState.InPoints`.
   - Done: fold caller summaries into callee `EntryValues` and seed entry through
     the existing value product.
   - Done: remove `facts.CloseInferredParams`, `InferredParam*`, and the
     `signatureForRefWithInferred`/`bodyParamExprTypeWithInferred` side lane.
   - Done: keep entry-vs-body provenance by reading solved summary entry evidence,
     not a pre-solve driver guess.
   - Done: add `summary.EntryValuesKey` so exact call-site parameter values are
     part of the summary context key (`Ref x CaptureCellsKey x FunctionRefsKey x
     EntryValuesKey`). Direct call-return/cell-effect/relation lookups use
     `SummarizeWithEntryValues`, and aggregate `CallEntryValues` fills only
     slots the explicit key did not provide.
   - Done: move body-contract projection/application out of `driver.go` into
     `domain/paramevidence/body_contract.go`; `driver.go` now only supplies graph
     and callee-resolution services.
   - Done: move call-argument obligation vector algebra into
     `domain/paramevidence/body_contract.go`. Signature-owned obligations,
     body-demand obligations, hard-contract joining, informative-slot filtering,
     and all-empty vector normalization share one domain implementation; the
     driver no longer owns local `paramContractVector` / demand-vector helpers.
   - Done: post-convergence call diagnostics now request
     `transfer.ProductCallContext` from the transfer package instead of rebuilding
     product argument values in the driver. The expression evaluator and
     product-call context shape are single-sourced in transfer; the driver only
     selects the solved point state and stores the projected call-edge evidence.
     Product/type vector conversion is single-sourced too:
     `product.FromTypes`, `product.ProjectValuesOrUnknown`, and
     `transfer.ProductCallContext.ArgTypes` own the type-only boundary.
   - Done: move source-parameter annotation checks and source-to-runtime-slot
     mapping into `domain/paramevidence`. The driver no longer owns colon-call
     receiver shifting or annotation policy for entry-value projection and
     signature contract application.
   - Done: move call-entry-value projection and merge rules out of `driver.go`
     into `summary/entry_value_projection.go`; `driver.go` now only supplies
     callee/slot/annotation/evaluation callbacks.
   - Done: move aggregate caller-entry fallback and prototype-self receiver entry
     seeding policy into `summary.AggregateEntryValues`. The driver supplies lazy
     summary iterators and static fact projections only; owner tests pin both the
     merge/filter semantics and the no-spurious-dependency rule.
   - Done: move generic-for iterator classification and loop-variable projection
     (`iteratorKind`, keyed-container check, present-entry key/value typing) out
     of `driver.go` into `domain/iteration`. The driver now supplies only callee
     and source-expression resolution. Builtin fallback is binding-normalized:
     `pairs`/`ipairs` are recognized only when the callee identifier resolves to
     a recorded global symbol with the same name, so local shadowing cannot leak
     builtin iterator semantics.
   - Done: move iteration key-presence provenance out of `driver.go`.
     `domain/iteration` owns iterator source classification and container-path
     projection; `canonical/facts` owns only immutable keys-collector function
     recognition; `PointState.KeyPresence` owns the live array-to-container
     relation. The driver performs callee/fact lookup only.
   - Done: collapse the keys-collector carrier to `domain/keyscoll.KeysCollector`.
     `canonical/facts` now only indexes that domain-owned carrier by `FuncRef`,
     and `domain/iteration` consumes the same type directly; the duplicate
     `facts.KeysCollectorEntry` / `iteration.KeysCollector` adapter shapes and
     the exported `facts.CollectKeysCollectors` helper are gone.
   - Done: collapse prototype/metatable topology carriers to `domain/metatable`
     (`Index`, `MethodReceiver`, `PrototypeMethod`, `SetMetatableSite`).
     `canonical/facts` now keeps only unexported `FuncRef` index rows for
     receiver/site lookup and returns domain-owned topology to transfer/driver;
     the exported `facts.*Entry` topology carrier surface is gone.
   - Done: collapse callback environment symbol bindings to
     `domain/callbackenv.GlobalBinding`. `canonical/facts` still lowers
     external `EnvOverlay` names to graph-local symbols and keeps unexported
     `FuncRef` rows for lookup, but consumers now receive the callbackenv-owned
     carrier and the exported `facts.CallbackEnvEntry` surface is gone.
   - Done: move parameter-narrowing carrier and FunctionRefinement
     export/import projection out of `driver.go` and `canonical/facts` into
     `domain/paramevidence`. Canonical summary, transfer, and driver consume the
     paramevidence carrier and deterministic finite-set helpers directly; the
     old `canonical/facts` compatibility alias and helper surface is gone.
   - Done: move transitive delegated-call provenance for parameter narrowing
     (`DelegatedCall`) out of `canonical/facts` into `domain/paramevidence`.
     Summary still resolves the call graph, but the carrier belongs to the
     parameter-evidence domain.
   - Done: parameter-narrowing effect set identity is owned by
     `domain/paramevidence`. Canonical transfer now emits raw structural
     `ParamNarrow` facts and delegates clone/sort/compact to
     `paramevidence.SortParamNarrows`; it no longer fabricates encoded string keys
     for semantic deduplication.
   - Done: move `Summary.Returns` concrete tuple projection and function-signature
     return replacement out of `driver.go` into `canonical/summary`. The driver and
     observation bridge now ask the summary owner for caller-visible return types
     instead of inspecting the return abstract-value tuple locally.
   - Done: move runtime-effect return transforms out of `driver.go`,
     `observation/expr.go`, and synth extraction into `domain/callreturn`. The
     call surfaces now pass completed call facts to one owner that handles
     method receiver argument shifting and copy-on-write return-slot transforms.
   - Done: move `ops.CallResult` return-vector projection into
     `domain/callreturn.ResultTypes`. Canonical call transfer and observation no
     longer duplicate the packed-tuple/adjusted-return interpretation locally.
   - Done: canonical call typing applies the compiler-owned `contract.ReturnSpec`
     override after the call pipeline and runtime-effect transforms, matching the
     legacy synthesizer's two-tier spec handling (AST inline literal match first,
     type/product argument evidence match second). This keeps conditional
     manifest returns such as `process.listen(..., {message=true})` inside the
     single product flow instead of falling back to raw/default returns.
   - Done: move type-level and product-level call-return policy out of
     `driver.go` into `canonical/call`. `canonical/call.ReturnInput` owns the
     deterministic type-return sequence (intercepts, summary returns, callee/
     receiver normalization, call pipeline, effect transforms, spec overrides);
     `canonical/call.ReturnValueInput` owns the product-return sequence
     (type intercepts, summary values, gradual dynamic calls, type fallback).
     The driver now supplies only context callbacks and explicit availability
     bits: base scope, manifests, bindings, type lookup, summary projection,
     the canonical `TypeResolver`, and type-arg resolution. No second pass or driver-owned
     precision compensation is introduced.
   - Done: move callable type-resolution precedence out of `driver.go` into
     `canonical/call.TypeResolver`. It owns the ordered policy for solved
     expression types, symbol-bound function signatures, graph-local declared
     globals, external global names, one-segment local field-function fallbacks,
     imported module-alias member fallbacks, and method receiver resolution. The
     driver supplies only callbacks for graph-local signatures, imported base
     types, global declarations, and external global lookup. `TargetResolver`
     remains the identity/closure `FuncRef` resolver; `TypeResolver` is the
     caller-visible `typ.Type` resolver. Tests pin one-segment static fallback,
     local-field-before-imported fallback, global-symbol-before-name lookup, and
     method static fallback order.
   - Done: move argument-demand function-shape policy out of `driver.go` into
     `canonical/call.FunctionForDemand`. Summary-known module targets win; the
     fallback resolves plain callees through `TypeResolver`, method calls through
     receiver member lookup, rejects gradual `any`/unknown/non-functions, and
     expands instantiated callables before projecting signature-owned argument
     obligations. The driver supplies only the optional summary-signature
     callback and the already-canonical `TypeResolver` value; it does not
     re-wrap separate callee/receiver callbacks for demand.
   - Done: move call-argument demand source precedence out of `driver.go` into
     `canonical/call.CallArgDemandsForCall`. Selected summary-demand projection
     is authoritative when present, even when it proves no concrete obligations;
     only a missing summary source falls back to signature-owned argument
     contracts. This prevents driver-local fallback from reintroducing broader
     manifest/signature demands after the product fixed point selected a callee
     summary.
   - Done: move call-summary argument-demand projection shape into
     `canonical/call.DemandsForCallTargets`. Driver still adapts program internals
     into `paramevidence.CallArgDemandTarget` rows, but the call-boundary
     projection from selected targets to concrete source argument demands is no
     longer assembled in `driver.go`.
   - Done: callback-spec lookup for call-site capture effects consumes the same
     canonical `TypeResolver` value. Summary-known module signatures still win;
     fallback callee/receiver resolution is no longer re-wrapped as separate
     driver callbacks for callback specs.
   - Done: caller-visible return-relation fallback consumes the same canonical
     `TypeResolver` value. Summary relations still win and closure-authoritative
     misses still block fallback; when fallback is legal, `canonical/call`
     resolves the live callee relation first and then the immutable static callee
     relation through `TypeResolver.ResolveStaticCallee`.
   - Done: imported parameter-narrow fallback and static callback-overlay
     extraction also consume `TypeResolver`. The call owner decides when to read
     imported member signatures or global callback-overlay signatures; driver no
     longer passes one-off imported/global resolver callbacks for these paths.

5. Declared annotation facts.
   - Done: transfer consumes declared symbol types from `input.ScopeFacts` instead
     of a separate constructor precision map.
   - Done: local/global/parameter declared types remain immutable pre-solve facts;
     transfer reads them as entry/narrowing authority, not as inferred state.
   - Done: the canonical program now retains one `functionFacts` carrier for
     declared annotations/globals. The duplicate `program.declaredTypes` map was
     removed; transfer input, capture entry seeding, caller-entry filtering, and
     diagnostics read the same immutable fact carrier.

6. Observation bridge cleanup.
   - Bridge final product facts to diagnostics.
   - Do not let observation feed precision back into the solver.
   - Done: retain each function's immutable annotation/global facts in the program
     input carrier and reuse them when projecting diagnostics. The bridge may add
     observation-only function signatures to a clone, but it no longer rebuilds
     declared facts by rescanning graph evidence after the solve.
   - Done: remove the post-solve `functionFields` table and dominator scan.
     `FunctionPathAt` now projects function-valued paths from the solved
     `FunctionState.InPoints` product value and the `FunctionRefs` axis.
     Summary-sensitive function values are resolved through the retained summary
     query using `(CaptureCells, FunctionRefs)` context. A field rebinding is
     represented by ordinary transfer, strong subtree invalidation, join, and the
     summary context key, not by bridge-side invalidation policy.
   - Done: root function definitions (`function f()`) write both the callable
     product value and the function identity into `PointState.FunctionRefs`.
     Identifier reads and named call resolution now consult the solved point-state
     product before falling back to immutable module binding facts. The old
     `funcSignatures` observation override no longer feeds precision into
     `RefinedAt`.
   - Done: conversion between canonical function identities (`ref.FuncRef`) and
     the flow-domain mirror (`flow.FunctionRef`) is owned by
     `compiler/check/canonical/ref`; driver and observation no longer carry local
     conversion/path-lookup helpers.
   - Done: field-path call resolution consults the solved product value before
     imported module-alias field fallback. A local rebinding of `M.f` therefore
     flows through `Env` plus `FunctionRefs`, not through the imported manifest's
     stale field signature.
   - Done: static field-call fallback is single-sourced: local field-function
     topology `(base symbol, field segment) -> FuncRef` lives in
     `canonical/topology` and is indexed by `canonical/facts`. It wins before
     imported module-alias member facts. The imported path is only a boundary
     fallback for require-alias exports after product state and module-local
     field identity fail.
   - Done: imported module aliases are immutable topology facts
     (`topology.ModuleAlias`) indexed by the canonical fact carrier.
     Capture-entry seeding and captured field-call fallback read
     `facts.ModuleAliasType(sym)`; the driver no longer owns a `moduleAliasTypes`
     precision cache.
   - Done: summary call-graph edges are derived through the canonical callee-ref
     resolver. The driver-owned `byName` map was removed; name strings remain only
     as parser/contract boundary vocabulary, while the fixed point records
     dependencies in `FuncRef` space.
   - Done: method-call module-summary lookup no longer uses a bare method-name
     fallback. `B:get()` resolves through receiver symbol + field-function identity,
     so another receiver's `A:get()` cannot donate returns or effects by name.
     Inside a method body, `self:get()` resolves through the current method's
     `MethodReceiver` prototype symbol plus field-function identity; this preserves
     the deadlock-dataflow-node `self:inputs()` case without reintroducing the
     bare-name shortcut.
   - Done: production symbol/function registries now lower to `FuncRef` after CFG
     discovery. Identifier and field-path callee fallback reads
     `cfg.SymbolID -> FuncRef` and `(cfg.SymbolID, constraint.Segment) -> FuncRef`;
     AST function pointers remain only at the diagnostic bridge and temporary
     discovery boundary.
   - Done: source-symbol function bindings are immutable
     `canonical/topology.FunctionBinding` values indexed by `canonical/facts`.
     The canonical driver no longer owns a `program.funcRefs` map or rescans
     function-definition CFG nodes when projecting function signatures; call
     resolution and signature fallback read the same topology carrier.
     Observation binding-type projection is also facts-owned:
     `facts.Module.FunctionBindingTypes` iterates the normalized binding carrier,
     while the driver supplies only the `FuncRef -> typ.Type` signature callback.
   - Done: `Summary.CaptureFunctionRefs` exports function identities at normal
     boundaries, and the summary solve query joins those lexical exports into child
     `EntryFunctionRefs`. Nested closures now receive both captured values and
     captured function identities through the same summary fixed point.
   - Done: callable observation is owned by `canonical.callableProjector`.
     Diagnostics resolve a function-valued runtime path through `ClosureRefs`
     first, preserving the closure's captured entry cells/function refs/closure
     refs, and only then fall back to ambient `FunctionRefs`. The bridge no
     longer projects stored closures through a type-only function identity that
     collapses same-body closures to `fun() -> any`. Callable projection now
     reads callee summaries through the `summary.Reader` live-or-converged
     boundary and applies `summary.FunctionSignatureWithProjectedReturns`; it no
     longer calls back into driver-owned function-value summary fallback logic.
   - Done: call-target resolution is owned by `canonical/call.TargetResolver`.
     It computes direct and closure target axes from solved `FunctionRefs` /
     `ClosureRefs`, applies immutable symbol/field/self-method topology fallback
     only when the solved axis is absent, and returns a normalized `TargetSet`.
     The driver now supplies only graph bindings and static topology callbacks;
     it no longer owns separate identifier, method, closure, and callback-target
     resolution branches.
   - Done: call-site summary application is owned by
     `canonical/call.SummaryProjectionForTargets`. The driver resolves
     normalized targets and supplies target-local entry-context readers, but the
     repeated "select targets -> build entry context -> read summary -> expose
     `summary.CallSummaryProjection`" shape is no longer duplicated across
     return values, return refs, relations, cell effects, and parameter-demand
     evidence. Summary algebra remains in `canonical/summary`.
   - Done: call-site no-return pruning is selected-target policy, not a
     driver-local single-ref lookup. `canonical/call.SelectionNeverReturns`
     prunes only when the normalized product/static target selection has at
     least one concrete target and every selected target is proven no-return.
     Mixed targets, empty selections, and closure-authoritative misses keep the
     continuation reachable.
   - Done: path observation is owned by `canonical.pathProjector`.
     `RefinedPathAt`, `RefinedPathValueAt`, gradual-top provenance, exact
     `StaticMembers` reads, product member traversal, and sequence-index
     length refinement now sit behind one read-only projection boundary. The
     bridge exposes gradual-top provenance only through `flow.ProductFacts` /
     `flow.ProductPathFacts`; the duplicate `flow.GradualTopFacts` surface was
     removed so product evidence is the single semantic proof. The bridge still
     implements the legacy type/path/length fact interfaces, but path precision
     is no longer spread through `observe.go`.
   - Done: keyed-iteration presence is owned by `PointState.KeyPresence`.
     Exact dynamic-index transfer and `HasKeyOf` observation now read the
     finite must-set product axis instead of scanning `Cond` disjuncts with
     bespoke any-disjunct logic.
   - Done: solved-flow observation is producer-neutral. Diagnostic hooks ask
     `api.FuncResult.SolvedFlow()` for the `api.FlowOps` surface. A result that
     actually materializes `flow.Solution` can expose that; canonical product
     results expose `canonicalFacts`. The bridge must not fabricate a
     `flow.Solution` just to satisfy hook reachability/numeric/path/exclusion
     queries; `observation.Projector` receives the same `api.FlowOps` surface.
     Condition-only proof selection and assignment/index-write product
     evaluation are separate owner-specific projections; they must not be folded
     into `FlowOps` until represented by their own normalized proof/evaluation
     carrier. This prevents `FlowOps` from becoming a catch-all escape hatch.
   - Done: assignment-source RHS evaluation has that separate carrier.
     `flow.AssignmentSourceFacts` is the producer-neutral interface for
     source-owned RHS values, and `compiler/check/domain/assignsource` owns the
     AST-free evaluator over solved-flow proofs. Observation no longer calls
     `flow.Solution` directly for assignment-source products. Old flow continues
     to satisfy the interface through its existing `AssignmentSourceValueAt`;
     canonical facts implement it by composing `assignsource.Query` with the
     product projection, path projection, length/key proofs, call-return slot
     projection, and operator algebra.
   - Done: dynamic index-write admission has its own point-local proof axis.
     `flow.IndexWriteFacts` is the producer-neutral interface for admitted store
     writes. Canonical transfer seeds `PointState.IndexWrites` in `WriteEffect`
     only when the value-domain admission law accepts the write and clears the
     axis at each node so the event cannot flow to successors. Observation now
     asks the interface and no longer names `flow.Solution` for this seam.
   - Done: condition-only proof projection has its own producer-neutral surface.
     `flow.ConditionProofFacts` exposes `ConditionAt`, `ConditionTypeAt`,
     `ConditionedTypeAt`, `ConditionedSeedTypeAt`, and `ProvesTypeAt` as pure
     projections from the converged point condition and a finite static/seed
     type environment. Canonical facts implement it over `PointState.Cond` and
     normalized structural path keys; legacy `flow.Solution` still satisfies the
     same interface. Observation no longer calls `flow.Solution` directly for
     condition-proof refinement. The product-domain path parser accepts both
     historical `symN` roots and canonical `sN` roots so descendant projection
     works over normalized PointState keys without reintroducing solver-version
     strings.
   - Done: constant-value facts are symbol-keyed at the flow boundary.
     `flow.ConstFacts` exposes `ConstValueAtSym`; source names are resolved to
     `SymbolID` only at the observation/path-lowering boundary through the CFG.
     `flow.Inputs` and legacy `flow.Solution` both satisfy the same interface,
     and observation no longer calls the name-keyed `Solution.ConstValueAt`
     path. This keeps constant lookup compatible with canonical graph identity
     and Salsa-style cache keys.
   - Done: high-level path observation now has a core policy owner.
     The existing `canonical.pathProjector` is the low-level product/path
     projection primitive; it does not by itself own the full observation policy
     that used to be scattered through expression and assignment-source path
     reads. `flow.PathObservationFacts` / `flow.PathObservationQuery` now own
     the AST-free policy core: pre/current/post phase, strict pre-phase for
     self-referential assignment sources, direct path facts, declared
     reconciliation, condition-proof enablement, authoritative `Never`, proof
     preservation, selected observation source metadata, and normalized
     expression-local index-read proof context (`flow.PathObservationIndexRead`).
     `api.FlowOps` is still only a primitive solved-flow query surface and must
     not become a catch-all escape hatch. Canonical facts implement
     `PathObservationFacts` directly over `FunctionState.InPoints` /
     `FunctionState.Points`; the observation-side implementation is now the
     compatibility adapter for producers that have not moved to the canonical
     surface. The shared selection law lives in
     `flow.SelectPathObservationResult`: producers compute candidates
     (`declared`, `direct`, `solved`, `proof`) from their own solved carrier, and
     flow owns deterministic candidate ordering, source metadata, strict
     pre-phase fallback, authoritative bottom, declared fallback, and optional
     admission. Call-boundary argument observation uses the same
     `PathObservationFacts` surface with `PathReadPre` and proof preservation,
     while explicitly ignoring declared-only fallback so parameter-entry
     evidence is never invented from annotations alone. Validation also locked
     the nilable-read law: explicit `nil` is a valid path observation of a `T?`
     declared read and must not be collapsed to "no fact" during
     same-expression reconciliation.

7. Transfer construction seams.
   - Done: pure transfer configuration now enters through `transfer.Config`.
     Cast annotation resolution and type-name-as-value resolution are constructor
     inputs, not mutable `SetCastResolver` / `SetTypeNameValueResolver` calls.
   - Done: prototype receiver topology and predicate guard facts are pre-transfer
     facts. Predicate function/result carriers live in `domain/guard`; the fact
     module only computes deterministic lookup rows before transfer construction
     and injects them through `transfer.Config`, not exported setters.
   - Done: `T:is(x)` assignment guard binds are immutable pre-transfer facts.
     `domain/guard` owns the checked-value/argument symbol projection and
     `TypeCheckBind` carrier; `canonical/facts` only indexes those bindings by
     `FuncRef` with deterministic sorting and defensive copies. The driver
     supplies module type-name resolution but no longer derives type-check
     narrowing facts.
   - Done: observation facts read solver-derived `FunctionState.InPoints`
     directly. The unused edge-narrower parameter and driver-local
     `edgeNarrower` adapter were deleted, so branch-edge refinement remains owned
     by the equation solve and is not re-derived by the diagnostic bridge.
   - Rule: only monotone runtime state belongs in `PointState`; immutable graph,
     signature, annotation, and guard facts stay in input/fact/config carriers.
   - Done: canonical `PointState.Env` now uses typed `flow.ValueKey` keys. Raw
     strings are no longer the domain boundary for point values; source/API names
     remain strings only at their real boundary.
   - Done: symbol value key construction/parsing is owned only by `types/flow`
     (`SymbolValueKey`, `ParseSymbolValueKey`). Driver, summary projection, and
     transfer code no longer carry local `symKey` mirrors of the cache encoding.
   - Done: predicate-call result links in legacy flow inputs use typed
     `flow.PredicateLinkKey{Symbol, DefPoint}` keys. The old `name@defpoint`
     string encoding and prefix/`Atoi` lookup were removed, so predicate
     narrowing evidence is keyed by graph-local symbol identity before
     interpretation.
   - Done: exhaustiveness select-case correlation uses `constraint.PathKey`
     for result/channel paths. The hook-local `#sym@version...` string key and
     path-format parsing are gone; channel-select domains are keyed by the same
     normalized path identity used by flow.
   - Done: exhaustiveness closed-domain matching consumes
     `observation.DeclaredPathType` over the admitted `flow.DeclaredTypes`
     carrier for the discriminant object path before falling back to solved
     expression observation. A declared local initialized with one concrete
     variant therefore still checks exhaustiveness against its closed annotation
     domain, while normal expression reads keep using narrowed product facts.
   - Done: symbol value projection from a converged `flow.PointState` lives in
     `flow.SymbolValue`. The `Cells`-over-`Env` precedence is point-state algebra,
     not a canonical-driver helper.
   - Done: lexical symbol storage policy is centralized in
     `canonical/transfer.symbolStoragePolicy`. `types/flow.PointWriter` and
     `flow.SymbolValue` remain primitive mechanics only: write/read Env or Cells
     when instructed, without deciding whether a lexical symbol is Env-backed,
     owner-cell-backed, or captured-cell-backed. Transfer symbol reads/writes route
     through `t.symbolValue` / `t.writeSymbolValue`; closure creation first
     projects already-live captured cells, then falls back through the same policy
     for missing entry cells.
   - Done: zero-safe projection of `product.AbstractValue` to public `typ.Type`
     lives in `product.ProjectValueOrUnknown`. The diagnostic/summary boundary no
     longer owns the value-domain rule that an unestablished slot projects to
     `unknown`.
   - Done: structural field/member lookup is called directly through
     `types/query/core.Field`. The canonical driver no longer carries a local
     `fieldMemberType` wrapper for callback specs, imported field callees, or
     forwarded-parameter body-contract evidence.
   - Done: graph symbol ownership/free-symbol classification lives in
     `compiler/cfg.Graph` (`OwnsSymbol`, `IsFreeSymbol`). Capture-entry seeding,
     capture export projection, and cell-backed transfer setup no longer duplicate
     local/param/upvalue classification in driver or summary code.
   - Done: owner-captured cell discovery lives in `compiler/cfg.Graph`
     (`CellBackedSymbols`). The canonical driver no longer walks nested-function
     captures to decide which owner locals/params are stored in `PointState.Cells`.
   - Done: method-self alias peeling uses the canonical type-layer helper
     `types/typ/unwrap.Alias`. The driver no longer carries a local
     `unwrapSelfAlias` variant for synthesized implicit `self` signatures.
   - Done: callable signature construction is owned by
     `compiler/check/canonical/signature`, not `driver.go`. That package owns
     generic type-parameter scope extension, source parameter lowering
     (unannotated parameters are optional gradual `any`), declared-return
     lowering, inferred-return splicing for unannotated functions/methods,
     ref-signature fallback mode for unresolved declared returns, and method
     implicit-`self` selection. The driver supplies only base scope, type
     resolver, AST/function refs, and inferred-return providers.
   - Done: predeclared-global seed normalization lives in `compiler/bind`
     (`PredeclaredGlobalNames`). Canonical and legacy pipeline drivers no longer
     carry local `collectGlobalNames` copies for the names passed to `bind.Bind`.
   - Done: direct static field-callee fallback uses binder-resolved
     `constraint.Path` identity through `callTyper.exprPath` and
     `Path.DirectFieldName`. The canonical driver no longer carries a local
     `staticFieldPath` AST parser for `base.field` callees.
   - Done: the legacy flow solver's field-overlay index is now a derived cache
     over `product.AbstractValue`, not a `map[string]typ.Type` semantic store.
     Overlay/cache roots are typed `constraint.PathKey`; suffix strings remain
     only the deterministic segment-tail encoding. The underlying legacy value
     store and point-mutable stores are now keyed by `constraint.PathKey`, not
     raw strings. The cache is excluded from `SolutionEqual`; projection
     to `typ.Type` happens only at the query boundary that builds field lists.
   - Done: flow alias-congruence state now stores canonical
     `constraint.PathKey -> constraint.PathKey` edges, not `map[string]string`.
     String formatting remains the deterministic encoding of a path key, but the
     semantic relation is typed as path identity.
   - Done: the table-key presence carrier (`pathPresence`) now stores stable
     `constraint.PathKey` keys internally for both function-wide and point-local
     presence maps. String conversion remains only at parser/API boundaries.
   - Done: the flow worklist dependency scheduler no longer encodes path and
     symbol dependencies in one string namespace (`"$sym:"` sentinels). It uses a
     typed dependency key whose alternatives are canonical `constraint.PathKey`
     and `cfg.SymbolID`. The changed-key propagation list feeding that scheduler
     is also `[]constraint.PathKey`, not `[]string`.
   - Done: flow suffix indexes now store canonical root identity as
     `constraint.PathKey` (`valueSuffixIndex` and `pointRootKey.root`). Suffix
     strings remain only the deterministic segment-tail encoding.
   - Done: flow field-overlay lookup cache now uses structural
     `constraint.Segment` keys below each `constraint.PathKey` root while
     retaining the product value carrier. Raw suffix strings are parsed at
     `PathKey` admission only and are no longer the internal field identity of
     `fieldOverlayIndex`.
   - Done: predeclared global value types are lowered once from external
     source-name config into entry symbol values keyed by `cfg.SymbolID`.
     `canonical.NewDriver` admits the external map once into
     `domain/globalenv.TypeOverlay`; the binder consumes deterministic overlay
     names, and canonical transfer consults the overlay only at source-name
     intercept boundaries. Observation `Projector` consumes only the normalized
     `GlobalTypeOverlay` carrier before falling back from graph symbols to
     source globals.
   - Done: `pipeline.Driver` and `pipeline.Runner` also admit configured global
     maps once into `domain/globalenv.TypeOverlay`; phase environments and public
     results receive projected maps only at external projection boundaries.
     Runtime-effect global lookup (`effects.ResolveRefinementBySym`) consumes the
     carrier directly.
   - Done: phase/extraction shared contexts no longer re-expand admitted globals
     into internal maps. `phase.PhaseEnv.GlobalTypes` and
     `abstract/core.FlowContext.Globals` carry `domain/globalenv.TypeOverlay`;
     resolve/scope/extract helpers consume the carrier directly.
   - Done: dynamic analysis-context globals use typed `api.GlobalName` /
     `api.GlobalOverlay` rather than `map[string]product.AbstractValue`.
     `api.GlobalName` is the same `domain/globalenv.Name` carrier used by
     callback overlays, and `api.GlobalOverlay` aliases
     `domain/globalenv.ValueOverlay`, so callback/global context values share one
     deterministic carrier family before lowering to graph symbols. The carrier
     owns normalization, duplicate-name convergence, projection to type overlays,
     equality, lookup, and parent-hash ordering. Contract/env source-name maps are
     lifted at admission; `api.envBase` stores `domain/globalenv.TypeOverlay`
     internally and exposes only `GlobalTypeOverlay` /
     `WithGlobalTypeOverlay` on `api.BaseEnv` for in-process consumers.
     Source-name maps remain only at constructor/configuration admission and
     external result projections.
   - Done: `api.FuncResult` and `api.FuncAnalysisView` now carry
     `GlobalTypeBindings` plus `GlobalTypeOverlay()` so solved-state observation
     consumes the normalized `domain/globalenv.TypeOverlay` result carrier.
     `GlobalTypes map[string]typ.Type` remains only as an external projection
     for result readers; `observation.Config` no longer accepts a raw global
     type map.
   - Done: function-literal signatures now expose `api.LiteralSignatureLookup`
     as the normalized in-process carrier. `phase.LiteralSigsProvider`,
     solved-state observation, nested-function context derivation, post-flow
     interproc fact writing, query dependency equality, pipeline results, and
     canonical results consume that lookup surface. AST-keyed literal-signature
     maps are admitted into the lookup at producer boundaries and remain only as
     external projections; `observation.Config` no longer accepts a raw literal
     signature map.
   - Done: interprocedural captured-field and constructor-field product facts use
     typed `constraint.Segment` field keys in `api.Facts` and the `interproc`
     lattice. `CapturedFieldAssignsDelta` and `ConstructorFieldsDelta` now accept
     typed `interproc.FieldValues`; `InterprocFactProduct.ConstructorFields` and
     constructor self-type enrichment also consume typed field values. Constructor
     field collection returns typed `FieldValues` directly; source field-name maps
     remain only at the legacy assignment collector and final `typ.Record`
     projection boundaries. Self-type enrichment's internal existing-field set
     is keyed by `interproc.FieldKey`, not field-name strings.
   - Done: `overlaymut` field-assignment products now use typed
     `overlaymut.FieldAssignments` (`SymbolID -> interproc.FieldValues`) and
     `MergeFieldsIntoType` / `MergeRequiredFieldsIntoType` accept typed
     `FieldValues`. `overlaymut` no longer projects to an intermediate
     `map[string]typ.Type`; field names are projected one key at a time only
     when writing the final `typ.Record` fields.
   - Done: captured-field promotion and constructor self-type enrichment stay on
     typed `interproc.FieldValues` / `captured.PromotedFields`; required-vs-
     optional captured-field merging no longer uses raw field-name sets.
   - Done: captured-container mutation facts use typed interproc mutation
     identity for product merge/equality: operator kind, mutation mode, and
     structural path key. The string `api.ContainerMutationKey` remains only a
     boundary/debug rendering, not the product de-duplication carrier.
   - Done: module export populated-map recovery uses typed field keys internally
     for exported record overlays and recovered literal map keys. Public
     `ExportTypes` / manifest type-definition maps remain source-name boundary
     APIs; the recovery pipeline no longer threads `map[string]typ.Type` through
     intermediate helpers.
   - Done: structural field identity is factored into `domain/fieldkey` so
     components below `interproc` can share one typed key without package cycles.
     `functionfact` export projection now carries projected export members by
     field key internally and only lowers/raises source field names at the
     record/interface boundary. `functionfact.ClassFamilyJoin` also de-duplicates
     and orders class-family record fields by structural field key before writing
     the final `typ.Record`. `functionfact` environment-return normalization now
     merges and orders specs with typed structural identity instead of an encoded
     string key.
   - Done: stable prototype receiver collection in synthesis now uses the same
     field-key carrier while merging literal-table, assignment, function-def,
     and existing record fields. Source strings are admitted only when reading
     AST/binding names and projected only while constructing the final
     `typ.Record`.
   - Done: metatable class-family sealing compares duplicated prototype surfaces
     with structural field keys instead of raw field-name sets; source names
     remain only at the `typ.Record` boundary.
   - Done: parameter-use evidence now carries demanded fields as structural
     `constraint.Segment` keys from trace collection through `api.FlowEvidence`
     and `paramevidence` projection. Raw strings are projected only while reading
     or writing `typ.Record` fields, so scope/pipeline/canonical consumers no
     longer rebuild ad hoc `map[string]struct{}` semantic field sets. The
     conditional body-precondition record merge likewise de-duplicates and orders
     fields by `fieldkey.Key` before projecting into the final `typ.Record`.
   - Done: canonical transfer's nested field-write, container append, and map
     insert helpers carry static paths as `[]constraint.Segment` internally.
     CFG/source string field paths are lifted at the transfer boundary; member
     identity stays structural through `value.MemberKey` and
     `product.MemberOf` / `product.WithMember`. Dot fields project to
     `typ.Record` names only at the current record boundary; static string and
     integer indexes stay indexed reads/writes instead of collapsing into record
     fields.
   - Done: guard-domain truthy/type guard keys now carry an opaque comparable
     structural suffix path built from `constraint.Segment` sequences. Guard
     propagation and guarded table-field narrowing no longer use a raw dotted
     field string as semantic identity; AST table keys lower through
     `pathseg.StaticTableFieldKeySegment` / `fieldkey.Key` and source names are
     projected only at the `typ.Record` boundary.
   - Done: production transfer/observation/value/metatable semantics no longer
     depend on debug/probe environment variables. The old `ZZ*` / `ZNARROW`
     scaffolding was removed from semantic code paths; analysis behavior is
     deterministic from input facts and product state.
   - Done: the legacy flow `Solution` no longer exposes raw string-key version
     dumps or the removed edge-value map through public debug helpers. Tests use
     semantic queries (`TypeAt`, `NarrowedTypeAt`, `ConditionAt`) or package-local
     state checks instead of depending on dead compatibility scaffolding.
   - Done: standalone CFG-dump probe tests were removed from the canonical package.
     Regression tests must assert semantic invariants; CFG topology exploration
     belongs in local investigation notes, not committed always-on test code.

## 8. Current known precision gaps

These are not accepted behavior. They are tracked so cleanup does not convert a
probe into a passing regression for behavior that should be fixed by the product
analysis.

- Closed: `TestNilUnionUnrelatedGuardKeepsOptionalGap` is unskipped. The product
  and query layers preserve partial-record union field optionality (`VR.value`
  projects to `Action?`), and declared return checking now uses
  `value.DeclaredBoundaryCompatible` rather than treating same-expression
  path-fact reconciliation as a general assignability proof. Reconciliation may
  cross a declared boundary only when it preserves explicit nilability, so an
  unrelated guard cannot erase `nil` from `vr.value`. No driver case was added.

- Closed: `TestCapturedOptionsDirectWritePrecisionGap` is unskipped. The caller
  projects the precise direct-call argument into `Summary.CallEntryValues`, and
  the transfer no longer converts a temporarily pending unannotated parameter into
  a top captured-cell effect during the first bottom iteration of the summary
  fixed point. The precise entry value now flows through the captured-cell effect
  path without driver compensation.

- Closed as an oracle correction: the old
  `TestCapturedOptionsAnyNarrowingPrecisionGap` required a mere `not_nil`
  presence proof to justify assigning `captured.retry.max_attempts: any` to
  `number`. That contradicts the declared-any contract already pinned by the
  gradual-typing regression suite: presence/truthiness proves non-nil, not a
  scalar type. The canonical tests now split the invariant:
  `TestCapturedOptionsAnyNarrowingRequiresScalarProof` expects the
  `any -> number` diagnostic after only `not_nil`, while
  `TestCapturedOptionsAnyFieldTypeGuardFeedsConcreteBoundary` proves the valid
  precision case where a positive
  `type(captured.retry.max_attempts) == "number"` edge feeds the concrete
  boundary. No driver or observation relaxation was added.

- Closed: `TestCapturedOptionsIndirectOpenFeedsCapturedCellWithoutGuard` is
  unskipped. Indirect open calls now project caller entry values through the
  summary/product path, feed captured-cell effects, and preserve the concrete
  `captured.retry` shape without a guard. This closes the old indirect captured
  option gap without a post-solve driver rewrite.

- Closed: `TestNestedFieldTypeGuardPreservesArithmeticInTemporalLoopShape`
  stays green under canonical flow. The gap was not loop arithmetic; canonical
  call typing skipped the shared `contract.ReturnSpec` override, so
  `process.listen("increment", {message=true})` returned the raw channel type and
  `msg:from()` degraded to `any`. Canonical `CallReturns` now applies the same
  spec-return transform as the synthesizer after ordinary call/effect projection.

- Closed foundational record/member carrier: `typ.Record` now carries exact
  bracket members separately from dot fields via `StaticMembers`
  (`StaticStringIndex`, `StaticIntIndex`). Product/value code lowers
  `constraint.Segment` to `value.MemberKey`, so `.field`, `["field"]`, and
  `[1]` are distinct structural coordinates with deterministic ordering.
  Subtyping/query/IO/value transfer paths consume that carrier; product-family
  precision and `ComparePrecision` include static members so exact bracket facts
  cannot collapse with dot fields during reduction. Components above this layer
  must continue using structural keys and may project to source-name strings only
  at the legacy record-field boundary.

- Closed foundational table-field syntax carrier: parsed table constructors now
  retain whether a key came from name syntax (`{foo = v}`) or bracket syntax
  (`{["foo"] = v}`, `{[1] = v}`) on `ast.Field.KeySyntax`. `pathseg` owns the
  field-aware lowering (`StaticTableFieldSegment`), preserving legacy/manual AST
  behavior only when syntax is unknown. Table literal synthesis, return-shape
  projection, guard extraction, CFG field-symbol extraction, canonical closure/
  function-ref collection, and mutation-slot collection consume that helper.
  This prevents valid-identifier bracket keys from collapsing into dot fields at
  the source boundary. Runtime soundness is still respected: a foreign
  `["foo"]` write can reach `foo`, so it weakens the dot field while also
  retaining exact bracket-member evidence.

- Closed foundational narrowing carrier: guarded exact-member presence is no
  longer encoded only by rewriting table types with non-optional record fields.
  `flow.PointState.StaticMembers` owns branch-local exact member evidence as
  `flow.StaticMemberFacts`, keyed by
  `flow.SymbolPathKey(root, []constraint.Segment)`.
  - carrier: sorted finite map from structural path key to product-domain value
    fact, with a bottom sentinel for unreachable;
  - order/join: must-fact lattice; join keeps keys proven on every predecessor
    and joins their product values, so branch merges keep only sound common
    member evidence;
  - widen: componentwise product widen over common keys; keys are finite
    program path identities, sorted deterministically;
  - transfer: exact writes install member facts, assignment kills affected path
    facts by structural prefix, guarded narrowing refines/installs facts, and
    reads/observation consult the fact before projecting to diagnostics;
  - projection: local branch facts stay point-local.

  This is allowed by §4 because it is dynamic precision evidence at program
  points with its own lattice. It is not a `cfg.Point` axis, not a driver side
  cache, and not a case table in narrowing. The lower-level `typ.Record` member
  carrier remains the final structural-type fix; `StaticMemberFacts` closes the
  branch-local presence precision gap without waiting for the full record
  representation rewrite.
  Synthetic branch applications (assert continuations and delegated parameter
  condition effects) now apply the complete edge-narrowed `PointState`, not a
  manually copied subset of Env/Cond/Cells. This preserves every product axis the
  normal edge narrower may refine, including `StaticMembers` and numeric length
  facts, and prevents future precision axes from being silently dropped.

- Closed foundational key-presence carrier: `KeyOf(table, key)` provenance is
  no longer recovered by scanning accumulated DNF conditions in transfer or
  observation. It is also no longer recovered by scanning the CFG to rediscover
  `pairs(table)` key/value bindings. `flow.PointState.KeyPresence` owns
  keyed-iteration presence as `flow.KeyPresenceFacts`, keyed by structural
  `PathKey` pairs, triples, and key-array facts.
  - carrier: sorted finite sets of `(table path key, key path key)` and
    `(table path key, key path key, value path key)` must-facts, plus
    `keys(array path key) -> table path key` facts, with bottom for unreachable
    and empty set as top;
  - order/join/widen: must-fact lattice; join and widen keep only table/key
    pairs/triples/array facts proven on every predecessor;
  - transfer: keyed and indexed iterator provenance seeds facts; assignment to a
    dependent or overlapping table/key/value/array path kills them; exact dynamic
    index reads and writes consume facts; self-write classification reads live
    value-origin facts instead of graph scans; canonical transfer no longer conjoins
    `constraint.KeyOf` into `Cond` for keyed-iteration compatibility;
    non-matching containers/keys keep the optional/read uncertainty intact;
  - projection: diagnostics call `HasKeyOf` through the product axis, not through
    a condition scan.

- Closed foundational transfer boundary: container writes are lowered through a
  canonical `transfer.Place` IR before the value product is mutated. A `Place`
  is the semantic location inside a `PointState` (`root`, then static-member or
  dynamic-index steps); it is not a CFG point. `applyContainerWrite` now handles
  direct, nested, static, dynamic, and mixed static/dynamic assignment targets by
  lowering once, then using product operations (`WithMember`, `MutateIndex`,
  `WriteIndexForeign`). The same lowered `Place` projects static paths for
  `StaticMembers`, `FunctionRefs`, and `ClosureRefs` writes, so bracket/dot mixed
  lvalues such as `root["handlers"].make = fn` do not depend on lossy
  `AssignTarget` reconstruction. The old nested-index bypass helpers are
  removed, and tests exercise the actual transfer entrypoint rather than a
  case-specific helper.

- Transfer-effect cleanup started: container writes and identifier writes now
  lower to `transfer.WriteEffect`, which consumes the canonical `Place` and
  applies the coordinated PointState updates for value Env/Cells, static members,
  key presence, numeric length, key-array provenance, prototype-self, function
  refs, closure refs, and point-local dynamic index-write admission proofs.
  Call-return function/closure ref rebases are now resolved into explicit
  reference-write payloads on the same `WriteEffect` and rebased through `Place`;
  static container writes and root writes therefore share one reducer path, while
  dynamic indexes are not misrepresented as exact static paths. Root writes also
  carry point-local relation invalidation (`Rel`
  sibling-nil kills) through `transfer.RelationEffect`, including no-value pending
  writes that must invalidate stale facts without forcing the value axis to top.
  Return-relation seeding for multi-return `(value, err)` correlations also uses
  `RelationEffect`, so the point-local relation axis has one reducer for both
  seeds and kills. Future write precision should add fields or reducer cases to
  `WriteEffect` / `RelationEffect`, not side-channel calls around assignment
  handling. Function/closure identity updates now lower through
  `transfer.ReferenceEffect`, the shared reducer for source-derived identity
  facts, explicit already-rebased call-return refs, return-slot placeholders, and
  dynamic-place root-subtree invalidation. `WriteEffect` and `ReturnEffect`
  construct reference payloads; direct FunctionRefs/ClosureRefs mutation is
  confined to the reducer internals. Captured-cell call effects now use the
  parallel `transfer.CellEffect` reducer: call/provider logic resolves a
  `CaptureEffects` value and optional closure path, while the reducer owns caller
  cell-store mutation, caller-visible `CellEffects` composition, and stored
  closure-environment updates. Table-mutator calls now use a third reducer,
  `transfer.MutatorEffect`, rather than hand-editing Env/Cells in `lenseed.go`:
  direct sequence targets append at an exact `Place`, update the numeric length
  floor when the sequence has a static path key, and invalidate stale key-array
  provenance at that place; dynamic map-element targets append into the map
  value slot through the base `Place` with an explicit abstract key, preserving
  the existing `{[K]: {V}}` shape rather than adding a driver-side
  `table.insert` case. Spec-level `ContainerElementUnion` effects use the same
  reducer family: the call layer resolves only the immutable effect label, while
  transfer lowers runtime argument refs through normalized `CallInfo` arguments,
  evaluates the element in the product domain, and applies the
  `product.ContainerElementUnion` law at the target `Place`. Unresolved element
  values become product Top, so the mutation remains sound instead of silently
  disappearing. Unreachable/non-initialized numeric states keep the old
  invalidation-only behavior, so early bottom iterations cannot pollute the value
  axis with `any`. Non-write narrowing now has its own first reducer,
  `transfer.RefinementEffect`: branch/assert/predicate roots and type-cast
  root/field-path refinements update Env/Cells as meet-like knowledge, without
  assignment invalidation of KeyPresence, StaticMembers, FunctionRefs,
  ClosureRefs, or Rel. Guard-driven static-member cache updates now lower to the
  companion `transfer.StaticMemberRefinementEffect`, so presence/type guards on
  `x.f` seed or narrow `StaticMembers` through the same `Place` vocabulary
  instead of hand-editing that product axis in the branch driver. This is
  intentionally separate from `WriteEffect`, because a guard or cast learns a
  stronger fact about an existing place; it does not overwrite that place. Numeric
  facts now lower to `transfer.NumericEffect`: length guards, index-presence
  length floors, numeric-for bounds, literal/path equality proofs, and numeric
  assignment facts are primitive atoms over canonical `PathKey`s, with one reducer
  owning clone/top/canonicalization behavior for `PointState.Num`. Path-condition
  facts now lower to `transfer.ConditionEffect`, so guard and equality-proof code
  conjoins `PointState.Cond` through a reducer instead of mutating that product
  axis locally. Function entry reachability now lowers through
  `transfer.EntryReachabilityEffect`, so the graph entry lifts bottom numeric and
  capture-effect axes to their reachable identities before parameter seeding
  without hand-initializing product axes in the node driver.
  Return statements now lower their caller-visible boundary state through
  `transfer.ReturnEffect`: the reducer owns return-slot Env values, return-slot
  FunctionRefs/ClosureRefs placeholders, and the return-relation axis consumed by
  summary projection. `applyReturn` constructs one slot-indexed payload from the
  expression tuple; it no longer writes `ReturnRel`, `ReturnSlotKey`, or
  placeholder reference axes directly. The reducer also clears stale return-slot
  values before optionally writing a precise non-identifier value, so identifier
  and unknown-expression returns cannot inherit precision from a previous slot
  state. Split-pattern OOP receiver publication now lowers through
  `transfer.PrototypeSelfEffect`; `WriteEffect.RecordProto` applies it uniformly
  after successful root and nested self writes, while `setmetatable` prototype
  publication uses the same reducer instead of mutating the `PrototypeSelf` axis
  directly.
  Unresolved container writes now still lower their target
  `Place` before source typing; the reducer invalidates stale
  StaticMembers, KeyPresence, FunctionRefs, and ClosureRefs even when the source is
  pending or unknown. When an exact dynamic-index `Place` cannot be lowered
  (`items.byName[k] = v` with an unresolved `k`), transfer emits an
  invalidation-only `WriteEffect` for the largest static write footprint instead
  of skipping the write or fabricating an exact key. Numeric-for and generic-for
  loop targets now also route through the same reducer via iteration-target write
  effects; this kills stale KeyPresence, StaticMembers, FunctionRefs,
  ClosureRefs, and point-local relations before seeding fresh iteration
  provenance. Function-definition targets now also route through `WriteEffect`:
  root and static member definitions share the same value/static/key/ref
  invalidation path, with method definitions carrying explicit function-ref
  payloads when the method ref differs from the function literal ref. The old
  assignment-level static pre-kill, loop-specific KeyPresence kill helper, direct
  `assignSymbolValue`, direct table-mutator root rewrites, and symbol-ref record
  helpers have been removed. Root-container rebuilds now route through the same
  symbol-value writer as root assignments, so Env versus captured Cells is decided
  in one place. The `Place` IR now also owns static path projection, static-prefix
  invalidation footprints, and both cache-key encodings (`constraint.Path.Key`
  for condition/ref-style axes and `flow.SymbolPathKey` for product axes). Numeric
  length facts, static-member facts, key provenance, function/closure reference
  paths, branch guard roots/segments, assertion paths, parameter-narrowing
  arguments, type-cast targets, value-origin demand paths, and literal-equality
  proof paths therefore consume the same lowered access shape rather than
  reconstructing identifier/field/index paths locally. Remaining symbol-value
  reducer callers are entry seeding plus the generic OR-union same-key refinement;
  side-axis updates should continue to move through reducer effects rather than
  driver-local mutations. Static assign-target projection and assign-target
  container projection are also owned by `Place`, so dynamic-key self-write and
  key-provenance logic compare complete static container paths (`items.inner`)
  instead of only root symbols (`items`).

The sorted-keys parameter-evidence regressions are now closed without fixture
branches. The architectural fixes were:

- call-site summary projection and call diagnostics read the call-event
  post-state, not the point-entry state. This captures same-node facts such as
  generic-for target binding before evaluating the call arguments;
- diagnostics traverse exact finite call contexts as `summary.Key` values
  projected from solved call sites. `Summary.CallEntryValues` remains fallback
  aggregate evidence for the summary fixed point; it is not the diagnostic
  context relation;
- broad soft container annotations such as `any[]` are contracts over the slot,
  not hard element facts when the initializer is known. `local xs: any[] = {}`
  preserves the empty value so later mutator effects can refine the element type
  from observed inserts.

Closed in this lane: body-demand contracts are no longer reused as callable
signatures. The canonical driver projects call-argument obligations through
`paramevidence.CallArgContractTypes`: declared parameter contracts plus
`Summary.Params` are consumed at the call edge and routed into the same
`ParamDemand` / entry-seed fixed point. The diagnostic bridge stores the solved
edge obligations in `api.FuncResult.CallContracts`, index-aligned with the
immutable `FlowEvidence.Calls` stream; `CheckCalls` receives that solved carrier
separately and enforces the edge vector without rewriting the callable's public
type. Callable signatures remain the declared source shape plus return summary
only. The projection is source-slot aware, so
method receiver offsets, function-ref callees, and closure-ref callees use the
same parameter-slot mapping as entry-value projection instead of driver-local
indexing. That mapping is runtime-slot first: colon-call syntax skips the
receiver slot in listed arguments, while plain calls into method-defined
functions can still target the implicit self slot. This keeps explicit-self,
implicit-self, dot-call, colon-call, function-ref, and closure-ref call shapes in
one projection family.

Closed in this lane: `pairs(t)` key/value use no longer lowers to a mutable map
obligation (`{[K]: V}`). The type lattice now has a first-class
`ReadonlyMap<K,V>` constructor: mutable `Map`, records, arrays, and tuples may
satisfy it covariantly, but `ReadonlyMap` never satisfies mutable `Map` and does
not expose write/delete slots. `KeyedIteratorEvidence` returns `ReadonlyMap` so
iteration can demand key/value enumeration without claiming the callee may write
arbitrary keys/values. This is intentionally a carrier-level axis wired through
kind/visit/format/hash/IO/subtyping/query/inference/iteration/transfer helpers,
not a driver or oracle special case. Mutable map invariance remains intact.

## 9. Acceptance gates

For each component:

- lattice law tests for every new carrier;
- focused transfer tests for the component's concrete semantics;
- focused summary tests for projection/application;
- `go test -count=1 ./compiler/check/canonical/... -timeout 120s`;
- curated oracle/gate only, not global fixtures:

```text
go test -count=1 -v -run 'TestCanonicalCurated(Oracle|Gate)' . -timeout 240s
```

No component is accepted if a driver workaround remains as the source of the
precision it claims to own.
