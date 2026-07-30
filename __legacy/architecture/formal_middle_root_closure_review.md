# Formal Middle-root closure: adversarial review

Status: design correction required, 2026-07-18. Read-only audit; no production
code was changed.

## Verdict

The neutral `formal.Middle` vocabulary is the right way to represent
invocation-local registers, and it is sufficient for every registered axis.
The proposal is **not complete as phrased**, however, in two precise ways:

1. a Middle root must identify one stable lexical register, not one producing
   formal-region cell; the region cell already identifies the program point
   whose tuple contains that register's current payload; and
2. that cell payload must be a guarded, correlated whole-product tuple with
   ordered effects, not an unguarded scalar product/path result.

With those corrections, N4 root assignments, external calls, generic-for,
expression phi registers and nested Apply all use one law. There is no
axis-specific interprocedural mechanism and no concrete `State` boundary.

The publication consequence is also precise: `InputBoundaryClosed` means **no
free Middle**, not “no Middle.” A canonical relation may retain body-owned MID
coordinates as existential variables when their declaration and complete
defining/reaching equation closure are in the same artifact.

## What exists today

`Arena` has exactly three invocation-local register namespaces:

- symbol registers from `bindEnvironmentSymbol` / `environmentValue`
  (`terms.go:321-332`, `387-395`);
- point-and-slot call-result registers from `bindCallResult` /
  `callResultValue` (`terms.go:335-349`, `375-385`); and
- expression registers from `bindExpressionValue` / `expressionValue`
  (`terms.go:351-373`).

Path-addressable locals are the same symbol register plus suffix:
`EnvironmentPath` is interned only from `symbol.ID` and segments
(`terms.go:403-410`). It is not a fourth register namespace.

The current closure already documents why these values are real registers:

- expression values retain every ordered write instead of substituting one
  predecessor (`relation_code_closure.go:165-176`);
- N4 retains its exact post-transaction register because contracts, overlays,
  tuple adjustment and nil fill can differ from every source term
  (`relation_code_closure.go:183-195`);
- generic-for and external calls bind the exact transaction-written target
  (`relation_code_closure.go:377-420`); and
- nested Apply binds local targets to the shared `valueFrameResult` selector
  (`relation_code_closure.go:508-534`).

`formalRelationCell` separately identifies node, step and outcome equations by
`(relationVar, root, step/outcome, kind)`
(`formal_relation_region_inventory.go:22-47`). This is already the program
point dimension of the dataflow lattice.

## Counterexample 1: producer-cell-owned roots split one mutable register

```lua
local t = {x = 0}
if c then
    t = {x = 1}
end
return t.x
```

The path term for `t.x` is keyed by the lexical symbol and suffix, not by the
assignment which most recently wrote `t`. If the declaration and guarded
assignment mint different Middle root identities, the post-join read needs a
path-valued phi selecting between them. The existing term language has no path
select: `pathNode` is one root/environment plus segments, and the current
environment merge only constructs a scalar `SelectValue`
(`relation_code_closure.go:468-481`).

Adding a second path-phi language would be the wrong repair. The canonical
dataflow representation already supplies the missing dimension: use one
stable `MID(Symbol(t))` register, and let each formal-region cell carry that
register's current guarded value/path/heap factors. Entry and assignment
equations update the payload; root identity does not change.

This also avoids renaming every heap/path/keyspace coordinate on every CFG
edge. A per-cell root namespace would multiply root count by cell count and
turn ordinary flow into boundary transport—the performance defect being
removed.

## Counterexample 2: an unguarded product loses correlation

```lua
local x
if c then
    x = 1
else
    x = nil
end
if x then
    return c
end
return false
```

Joining the two writes into only the product value `1 | nil` makes the second
condition unknown and loses the fact that `x`'s truthiness is correlated with
`c`. A later concrete Apply with `c=true` can no longer recover the exact
branch.

The current closure preserves this information by building
`SelectValue(nextGuard, other, value)` at a join
(`relation_code_closure.go:468-479`), and the formal inventory preserves
distinct choice-true/choice-false influences
(`formal_relation_region_inventory.go:49-68`). The formal cell payload must
therefore retain guarded alternatives over the complete registered tuple.
Joining/widening only the leaf factors is sound; erasing the guard partition is
not.

The same issue appears in short-circuit expression phi registers. The explicit
comment at `relation_code_closure.go:165-176` says substituting one guarded
`FrameResult` would resurrect an untaken call. A single MID expression register
is correct only when its incoming writes remain guarded contributions.

## Counterexample 3: a root result alone does not define read-before-write

```lua
local x = 0
x = x + 1
return x
```

If the assignment's Middle root is interpreted as both the source and
destination of its own producing cell, the equation becomes the circular
`x' = x' + 1`. The correct transfer reads `MID(Symbol(x))` from the predecessor
cell payload and atomically writes the same register in the step cell payload.

Thus cells own versions of the tuple; they do not own register identity.
Ordered `relationNodeSequence` step cells provide the before/after boundary.
This rule also handles repeated writes and loop-carried writes without SSA
renaming: ordinary flow reads the predecessor tuple, and loop feedback joins at
the WTO head.

## Scalar values and guards

Complete under the corrected rule:

- Every `valueEnvironment(slot)` becomes
  `valueRoot(MID(register(slot)))`.
- A guard reading that value evaluates the root against its predecessor cell's
  guarded tuple. Guard nodes remain the existing target-owned ROBDD syntax.
- A choice contributes the complete tuple under the exact guard/complement;
  it does not independently join each scalar before the choice.
- At a WTO head, leaf factors use registered Join/Widen/Narrow while the finite
  guard partition retains correlation. There is no depth/context root and no
  cap.

The register schema is a closed typed sum, not a raw integer convention:

```text
SymbolRegister(symbol.ID)
CallResultRegister(cfg.Point, result ordinal)
ExpressionRegister(factflow.ExprRef)
```

It is sealed once in canonical kind/field order from `Arena.environment` and
the plan. `formal.Root{owner, ordinal, Middle}` is its durable identity; dense
indices are acceleration only.

## Path-addressable values

Complete only when a symbol's path and scalar are the same MID register.
`EnvironmentPath(symbol, suffix)` becomes
`Path(MID(SymbolRegister(symbol)), suffix)`. The registered tuple at each cell
then supplies Values, path evidence, heap identity, dynamic index, membership,
user lattices and every other enabled factor for that root.

Boundary symbols need an entry equation:

```text
MID(SymbolRegister(p)) := IN(Param(p))
```

before any local write. This matters for a rebound parameter:

```lua
local function f(p)
    p = {}
    p.x = 1
end
```

After N4, `p.x` must address the local current register, not the caller's IN
path. A stable MID register initialized from IN and then updated by N4 gives
the exact semantics without a path-phi operator. Captures/globals/ambients use
the same entry binding plus their existing mutable output/effect ownership.

The current closure's split maps (`environment` for scalars, `paths` for
boundary paths) are not the final contract: `relation_environment_closure.go`
maps every boundary symbol path directly to its IN root at lines 98-135 while
local paths are retained separately at lines 137-150. The formal evaluator
must instead read scalar and path facets from the same registered MID
coordinate.

## N4 root assignment

A MID value is only the address of the target register. N4 remains one atomic
registered operation, because it also owns declared contracts, overlays,
tuple adjustment/nil fill, object construction, equality/presence publication,
descendant invalidation and cross-axis completion.

The formal step must apply the existing factor-native N4 transactions to the
predecessor tuple and commit the entire target tuple once. Reconstructing the
post-N4 scalar from source terms would repeat the concrete/symbolic divergence
already identified. The root does not replace the operation semantics; it
makes their target independent of concrete `statekey.Value`.

## External calls and nested Apply

Scalar local targets are not the complete call transaction. The canonical
`CallResultTransaction` also owns postcondition refinements, path relations and
return-presence publication (`factapply/call_result_transaction.go`,
`PlanCallResultTransaction`, `HasPostconditionSteps`,
`HasPublicationSteps`). External call outcomes may additionally carry effects,
diagnostics, suspension and typestate.

Therefore a call step:

1. evaluates its external artifact or lexical callee outcome;
2. applies the complete existing ordered call transaction to the guarded
   predecessor tuple; and
3. writes call-result/expression/symbol MID registers in that same atomic
   output tuple.

For lexical calls, `valueFrameResult` must **not** itself be replaced by a
Middle root. It is the existing correlated reference to one callee OUT slot
under a shared `callFrameTerm` (`terms.go:652-660`). The Apply step resolves
that target-owned reference and writes the caller's MID call-result/local
register. Converting the frame selector into an unbound MID before Apply would
lose the caller-to-callee binding and create a second call mechanism.

## Generic-for

`ApplyGenericFor` is explicitly a clear/write/membership transaction, not a
scalar assignment (`factapply/generic_for_transaction.go`,
`PlanGenericForTransaction`, `ApplyGenericFor`). A MID symbol root handles the
loop-variable address, but the formal step must still invoke the registered
membership operation and carry all resulting factors. A scalar/path-only cell
payload is incomplete.

Loop recurrence is handled by the existing formal WTO influences. The target
MID register is stable across iterations; feedback contributes the full
guarded tuple to the loop head, where registered factors widen/narrow.

## All axes, including future axes

No axis switch belongs in Middle-root closure. `ProductDomain` already exposes
the closed registered seams:

- `VisitValueDependencies` for whole products;
- `VisitLaneValueDependencies` for opaque lane factors; and
- `VisitCoordinateValueDependencies` for coordinate families.

Their dependency is the closed sum `statekey.ValueDependency`, which already
accepts a typed `formal.Root`. The cell payload and operation transactions use
the registered product/factor laws. Adding or removing an axis changes its
registration and operation contract, not register identity, call composition,
WTO scheduling or closure.

## The current SlotSpace is not yet a Middle schema

`SlotSpace` currently derives ordinals only from boundary `Shape` kinds and
validates every vocabulary against the same total boundary-root count
(`formal_slot_space.go:70-119`). Its tests even construct a `RootResult` as
`formal.Input` and a `RootCapture` as `formal.Middle`
(`formal_slot_space_test.go:52-66`). That proves vocabulary distinction and
width, but not semantic schema membership.

The corrected schema needs exact vocabulary-specific inventories:

- IN: parameter/capture/global/ambient boundary roots;
- MID: the sealed typed register inventory above;
- OUT: result, heap/effect and observation roots declared for publication.

`SlotAt` must validate an ordinal against the selected vocabulary's exact
inventory. It must not accept a boundary Result as IN or manufacture MID from
a boundary `RootKind`. Full `uint64` ordinals remain correct; no width cap is
needed.

## Exact closure/publication law

A stabilized lexical artifact is input-boundary closed iff the transitive
dependency graph of every published OUT is closed over:

- declared IN roots;
- declared OUT roots;
- constants/concrete identities and explicit external artifacts; and
- same-body MID roots for which the artifact contains exactly one declaration
  and the complete defining/reaching formal-region equations.

MID variables are existentially quantified inside the relation artifact. They
are included in canonical bytes/fingerprint. They never appear in a caller
binding cursor, external dependency inventory, route key, or concrete entry
state. A foreign, undeclared, multiply declared, or equation-orphaned MID is a
free variable and fails the whole seal transaction.

The output need not inline every MID into a giant expression DAG. It may refer
to a bound MID through the artifact's equation graph; “no unresolved MID” means
the binder/equation closure is present, not that all internal variables were
textually eliminated.

## Minimal corrected implementation design

1. Seal one vocabulary-specific Middle register schema per lexical body from
   the three existing environment slot kinds.
2. Rewrite every `valueEnvironment` and environment `PathTerm` reference to the
   corresponding stable MID root. Seed boundary-symbol MID registers from IN
   roots at the entry equation.
3. Make each existing `formalRelationCell` payload a guarded correlated tuple
   over the complete registered product plus ordered existing term/effect
   references. Terms read predecessor payloads; operations commit successor
   payloads atomically.
4. Keep `valueFrameResult` as the sole lexical-call result reference. Apply
   writes resolved results and the complete call transaction into caller MID
   registers.
5. Let flow/choice/WTO influences compose the full tuple. Join/Widen/Narrow use
   registered factor laws inside guard partitions; no route, context, depth or
   budget is introduced.
6. At stabilization, seal one artifact whose binder graph includes every
   reachable MID declaration/equation. Mint `InputBoundaryClosed` only after a
   transitive no-free-MID check over the actual stabilized payload.

This is the smallest correction because it reuses the existing term language,
formal cells, WTO, registered factor operations and dependency visitors. It
adds one lexical register schema and one guarded tuple payload; it does not add
SSA path syntax, a second engine, or axis-specific composition.

## Required focused proofs before wiring

1. Mutable path local across a guarded rebind preserves input correlation and
   writes the selected object only.
2. Rebound parameter path no longer addresses the caller's IN root.
3. Short-circuit expression phi with effectful calls executes only the selected
   frame and preserves its result correlation.
4. `x = x + 1` reads predecessor MID and writes successor MID without a
   self-referential term equation.
5. External multi-result call preserves scalar results, postcondition path
   equality/refinement, presence, effects, diagnostics and typestate atomically.
6. Generic-for preserves scalar projection and membership across a loop WTO.
7. Same helper under 1/10/100 callers has identical MID schema, formal cells,
   target evaluations and retained artifact; only frame binding/Apply work
   scales.
8. A same-body bound MID seals; foreign/orphan/multiply bound MID fails closed
   and publishes nothing.
