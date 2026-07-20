# Formal fiber inventory map

Status: implementation map, 2026-07-18. This document freezes the descriptor
universe consumed by `formal_fiber_directory.go`. It does not introduce an
expression IR, an executable formal engine, or a second product implementation.

## Freeze seam and ownership

Freeze one forest-owned catalog with one descriptor span and one
`formalFiberDirectoryArena` **per `relationVar`**. Do not use one universal
directory arena for the whole forest: a body owns its `ProductDomain`,
`KeySpace`, registered lane/family capabilities, term/effect arenas and formal
schema. Their opaque payloads cannot lawfully be compared or zipped with a
different body's payloads. A callee tuple is composed into the caller by exact
formal substitution/import; its directory root is never zipped directly with a
caller root.

The freeze point is:

1. after bodies have their stable `relationVar` order
   (`relation_program.go:648-650,680-701`);
2. after `relationCode`, `Arena` and `EffectArena` are sealed
   (`relation_program.go:978-1007`);
3. after the vocabulary-specific `SlotSpace` and all static coordinate
   inventories are sealed; and
4. before formal cells are evaluated by the WTO. The cell/influence universe is
   already frozen from code at `formal_relation_region_inventory.go:84-143`.

The directory provides only immutable structural addressing. Ref zero is the
canonical all-default subtree and every update is path copying
(`formal_fiber_directory.go:64-87,107-110,182-199`). A descriptor therefore owns
the **typed interpretation** of zero for its fiber. Zero never means a universal
Go/lattice Bottom.

A descriptor needs only private structural data:

```text
owner relationVar
role  closed role tag
key   typed role identity (never a name or hash)
group contiguous joint-group span, if any
default typed terminal/default resolver
ops   concrete registered/symbolic operation table
deps  concrete registered dependency visitor
```

The descriptor array order is semantic identity, not insertion order:

```text
body relationVar order
  care
  MID value bindings by formal root ordinal
  MID path bindings by formal root ordinal
  effect occurrences in stable storage order by
      (relationRootRef, one-based step ordinal)
  outcome occurrences by boundaryOutcomeRef
  ordinary lanes by ProductLane.Ordinal
  coordinate groups by (ProductLane.Ordinal, CoordinateFamily.Ordinal):
      skeleton, then scalar slots by ProductDomain.CoordinateSlotLess
  ground Values topology, then slots by (Vocabulary, formal ordinal)
```

The body order is already full lexical-body byte order, and `SlotSpace` derives
the finite Input/Middle/Output widths from `Shape`, the sealed Middle schema and
heap templates (`formal_slot_space.go:36-51,74-84`). `formal.Root` retains the
full body ID, `uint64` ordinal and vocabulary; no dense index or digest is
semantic identity (`domain/formal/root.go:5-29`).

## Descriptor roles

### 1. Care / reachability

- **Identity/order:** exactly one fiber at ordinal zero of each body span. Cell
  identity remains the external `formalRelationCell`; it is not duplicated in
  the descriptor.
- **Default:** Boolean false/unreachable.
- **Operations:** the shared decision kernel owns Boolean union/intersection.
  Join and Widen use `OR`; Meet uses `AND`; Narrow retains the stabilized
  ascending care and restricts the descending payload to it. This is guard
  correlation, not a State lane.
- **Dependencies:** none in the leaf. A guarded transfer inventories its guard
  atoms from the existing `Guard`/`ValueTerm` DAG; it does not make guards into
  product axes.
- **Producer:** `relationCode` control nodes and the frozen influence edges.
  `relationNodeChoice`, sequence, loop and outcome syntax are in
  `reduced_relation.go:70-130`; exact flow/choice/loop/callee influences are
  linked at `formal_relation_region_inventory.go:158-243`.

### 2. MID symbolic value bindings

- **Identity/order:** one fiber per `FormalSlot` in `formal.Middle`, by its
  one-based vocabulary-local ordinal. The sealed Middle register inventory is
  sorted and numbered once at `middle_register.go:89-116`.
- **Payload/default:** an arena-qualified existing `ValueTerm` reference plus
  its DD enable region; default is absent. A reference is `(relationVar,
  ValueTerm)`, not a copied term or a `product.Value`.
- **Operations:** identical references reuse exactly; guard-disjoint references
  remain DD alternatives. A same-valuation merge must name an already-sealed
  `valueJoin`, `valueSelect` or lexical mu term. If syntax has no such term, the
  relation is incomplete and freeze/evaluation rejects; Join/Widen/Narrow may
  not intern a runtime term or replace it with Top. Canonical joins/selections
  already belong to `Arena.JoinValue`/`SelectValue`
  (`terms.go:521-565`).
- **Dependencies:** walk the existing value/guard DAG, resolving every
  `valueRoot` through this body's `SlotSpace`; frame-result references retain
  their sealed `callFrameTerm`. The existing closed term-reference census is
  the source of truth (`relation_code_closure.go:614-675`), not a new parser.
- **Producer:** Middle writes are existing step/value syntax and sealed lexical
  registers. `boundaryStep` owns its value-bearing syntax at
  `reduced_relation.go:47-68`.

Callable results are not a second writable fiber vocabulary. Production
`FreezeRelationProgram` requires `Shape.Results == 0`; result values and their
correlation already live in the canonical `boundaryOutcomeTuple.operations`
and `returnTransaction.sources`. The outcome-occurrence fiber below enables
that existing syntax directly.

Input roots are not writable fibers. They remain leaves in `ValueTerm` and are
bound by the dense immutable `callFrameTerm` recipe (`terms.go:94-121,628-667`).

### 3. MID symbolic path bindings

- **Identity/order:** the same Middle `FormalSlot` order as value bindings.
  Unused roots remain physically default.
- **Payload/default:** arena-qualified existing `PathTerm`; default absent.
- **Operations:** exact reference reuse and guard partitioning only. `PathTerm`
  has no independent lattice join. Two distinct paths demanded on one live
  valuation must already be represented by the owning relation operation or the
  carrier rejects; it may not invent a path union.
- **Dependencies:** the path root resolves to one neutral `formal.Root`; its
  immutable segment suffix remains in the existing `pathNode`
  (`terms.go:173-177`). Coordinate consumers use the registered coordinate
  dependency visitor for the resulting structural key, never payload casts.
- **Producer:** all path syntax is already retained in the term/effect/outcome
  arenas. `relationCodeTermRefs` enumerates outcome paths and every effect path
  at `relation_code_closure.go:643-675,696-742`.

Formal path keys must use `KeySpace.InternFormalRoot(formal.Root)` plus the
existing immutable segments. The old `structuralPathKey` converts roots back to
concrete symbol slots (`coordinate_selection_contract.go:59-82`); that is a
route-runtime adapter and must not be used by the formal inventory.

### 4. Ordered effect occurrence

- **Identity/order:** one descriptor for every lexical
  `boundaryStepEffect`, keyed by `(relationVar, relationRootRef, stepIndex+1)`.
  The occurrence, not merely `EffectTerm`, is identity; the same immutable term
  may execute at two distinct lexical positions.
- **Payload/default:** immutable `(relationVar, EffectTerm)` plus a Boolean
  enable DD; default disabled/no effect.
- **Operations:** occurrence payloads must be identical. Join/Widen union enable
  regions; Meet intersects them; Narrow restricts the predecessor's enable
  region. Distinct payloads claiming one occurrence are malformed. Descriptor
  order is only canonical occurrence identity/storage order. Actual program
  order remains the target `relationCode` control/path order under guards,
  especially across branches; the inventory must not invent one global effect
  execution order or join a mutable effect slice.
- **Dependencies:** use the existing effect visitor, including targets, values,
  source paths, expected values and object members
  (`relation_code_closure.go:696-742`). Registered lane/family dependencies are
  selected from that same term/path census.
- **Producer:** reduction copies each lexical `instructionEffect` to one
  `boundaryStepEffect` at `reduced_relation.go:235-243`. The five closed effect
  kinds and immutable `EffectTerm` authority are in
  `effect_terms.go:14-29,188-205`.

### 5. Correlated outcome occurrence

- **Identity/order:** one descriptor per nonzero `boundaryOutcomeRef`, in its
  sealed table order. Identity is `(relationVar, boundaryOutcomeRef)`.
- **Payload/default:** immutable reference to the existing
  `boundaryOutcomeTuple` plus enable DD; default absent/no normal continuation.
- **Operations:** the immutable payload cannot change. Join/Widen union enable
  regions, Meet intersects, and Narrow restricts the ascending occurrence.
  Diagnostics, obligations, observations, return correlations and typestate
  stay inside the existing tuple; they are not parallel product fibers.
- **Dependencies:** `relationCodeTermRefs.outcome` is the complete ValueTerm,
  PathTerm and Guard visitor (`relation_code_closure.go:643-675`).
- **Producer:** each terminal return appends exactly one tuple and retains its
  ref at `reduced_relation.go:278-303`; the formal region declares one outcome
  cell per table entry at `formal_relation_region_inventory.go:117-124`.

### 6. Ordinary registered residual lane

- **Identity/order:** one descriptor for each enabled non-Values lane for which
  `ProductDomain.CoordinateFamilies(lane)` is empty, ordered by
  `ProductLane.Ordinal`. A lane represented by coordinate families gets no
  duplicate ordinary descriptor.
- **Default:** `ProductDomain.LaneBottom`.
- **Operations:** solely `LaneSame`, `LaneJoin`, `LaneMeet`, `LaneWiden` and
  `LaneNarrow` (`product_lane_factor.go:380-463`). Representation-equal reuse is
  allowed; “first lattice-equal operand wins” is not canonicalization.
- **Dependencies:** `VisitLaneValueDependencies` and
  `VisitLaneIdentityTerms` (`product_lane_factor.go:507-588`). Exact identity
  images use the one complete-tuple `IdentitySubstitutionPlan`, never a
  lane-local inverse fiber (`identity_substitution.go:24-41,115-217`).
- **Producer:** the body's sealed ProductDomain registration. Lane order and
  descriptors come only from `LaneInventory`/`NonValuesLaneInventory`
  (`product_lane_factor.go:75-130`); step semantics produce opaque factors
  through those registered laws.

### 7. Coordinate family skeleton and scalar fibers

- **Identity/order:** one contiguous joint group for every registered family,
  ordered by `(lane ordinal, family ordinal)`. The skeleton is first, followed
  by the body's closed `CoordinateFactorInventory` slots in
  `CoordinateSlotLess` order (`coordinate_family.go:492-512`).
- **Default:** skeleton zero means `CoordinateSkeletonBottom`. Scalar zero is a
  **group-relative** default resolved by `CoordinateDefault(currentSkeleton,
  slot)`, not universal absence. `Required` coordinates are emitted explicitly
  when materializing even if their physical directory token is zero;
  Optional/Forbidden omission follows `CoordinateScalarSupport`
  (`coordinate_family.go:451-469,688-727`).
- **Operations:** the skeleton uses `CoordinateSkeletonJoin/Meet/Widen/Narrow`;
  scalars use `CoordinateScalarJoin/Meet/Widen/Narrow`
  (`coordinate_family.go:540-573,623-654`). Restriction and any skeleton change
  operate on the whole joint group. When a skeleton change can alter support or
  defaults, materialize/recombine **only that family** through the exact
  registered lane operation; never combine scalar fibers independently.
- **Dependencies:** each scalar key uses
  `VisitCoordinateValueDependencies` (`coordinate_family.go:527-537`). Identity
  support and substitution remain group/tuple operations: recompose the family
  lane, then use the lane's registered identity visitor and the complete-tuple
  substitution plan. There is no lawful skeleton-only inverse fiber.
- **Producer:** ProductDomain registration supplies the family inventory and
  complete algebra (`product_domain.go:84-129`; `coordinate_family.go:284-299`).
  Static scalar keys are sealed/sorted/deduplicated with
  `SealCoordinateFactorInventory`; family dependency closure is only the
  registered `CloseCoordinateFactorInventory`
  (`coordinate_factor_inventory.go:58-104,204-230`).

The frozen body inventory is the union of immutable prepared seeds and every
syntax-owned producer coordinate, then registered closure. The current route
implementation demonstrates legitimate producers—initial seeds, linked result
equality, return-presence and presence-implication plans—at
`branch_factor_topology.go:286-391`. Formal freezing must express their paths as
typed formal roots and remove linked-route/frame identities; it must not copy
that route adapter.

### 8. Ground formal Values

This is the registered residual Values product only when a fact is genuinely a
ground `product.Value`. It never replaces a symbolic binding such as
`OUT = IN`.

- **Identity/order:** one group-level Top Boolean followed by one scalar fiber
  per admitted `FormalSlot`, ordered by vocabulary then ordinal. The Top bit is
  mandatory: a finite map whose missing key means Bottom cannot otherwise
  represent `ValueFactor.Top` (`value_lane_factor.go:11-18`).
- **Default:** Top=false and every scalar=`product.Bottom`.
- **Operations:** exactly `ValueFactorLattice[FormalSlot]`, including its
  registered Same/Join/Meet/Widen/Narrow behavior
  (`value_lane_factor.go:24-90`). A sparse directory may dispatch a changed
  scalar through the same underlying product-value operation, but group results
  must equal this lattice exactly.
- **Dependencies:** product identities are visited once when sealing the
  complete tuple substitution plan; image application is the generic
  `ApplyValueFactorIdentitySubstitution`
  (`identity_substitution.go:266-311`). Cross-axis dependencies from registered
  residual coordinates point to these slots through `ValueDependency`, not by
  inspecting Values maps.
- **Producer:** exact ground entry contracts/constants and registered residual
  transactions only. `valueRoot`, `valueSelect`, `valueFrameResult`, dynamic
  reads and value-bearing effects remain existing symbolic terms.

## Finiteness theorem

For one body let:

```text
M = sealed Middle roots including heap templates
E = lexical boundaryStepEffect occurrences
Q = nonzero boundaryOutcomeRef count
L = ordinary registered lane count
F = registered coordinate family count
C = size of the union-closed static CoordinateFactorInventory
S = admitted ground FormalSlot count
```

Then its fixed directory width is bounded exactly by:

```text
1 care + 2M + E + Q + L + F + C + 1 ValuesTop + S
```

All terms are finite before solve:

- bodies and `relationVar`s are frozen from a finite sorted unit list;
- Middle roots are bounded by the sealed schema; callable outputs are bounded
  by the sealed outcome table and are not duplicated as writable fibers;
- nodes, steps, effects and outcomes are sealed slices in `relationCode`;
- lanes/families are a sealed ProductDomain registration; and
- coordinate scalar keys are a finite, sealed, registered-closure inventory.

No cell, transfer, guard valuation, caller, route or abstract value is part of
that count. Evaluation may only resolve a descriptor ordinal or reject.

## Dynamic keys: the one current solve-time discovery defect

Literal/static suffixes and every segment occurring in a sealed `PathTerm` or
`EffectTerm` are freeze-time keys. Unknown dynamic writes remain one symbolic
effect plus registered DynamicIndex/heap residue; they do not enumerate an
infinite member universe. Caller-owned arbitrary heap members stay in the
caller-bound registered factor and never become target-WTO fibers.

The current concrete dynamic-read protocol violates the formal inventory rule
if used inside the target solve. It initially seeds demands from evaluated
`TableValue` identity (`dynamic_read_factor.go:1039-1048`), then can add path
coordinates from evaluated exact keys (`dynamic_read_factor.go:1082-1097`) and
heap root/member coordinates by inspecting a skeleton, projected values and its
`staticKeys` (`dynamic_read_factor.go:1225-1321`). `addSlot` mutates the plan's
slot list (`dynamic_read_factor.go:1344-1367`), and `dynamicReadAdvance` repeats
until that list stops growing (`dynamic_read_factor.go:771-812`). This is finite
for one concrete query, but caller-dependent and therefore not a legal target
fiber inventory.

The foundational fix is not a budget or a larger cache:

1. `valueDynamicRead`/`valueDynamicTableRead` remain the existing sealed
   `ValueTerm` syntax throughout formal WTO solving; their table/key/path/range
   term dependencies are frozen structurally.
2. The reusable target relation never evaluates them to discover a coordinate
   and never changes its descriptor array.
3. At final caller binding, the existing ProductDomain dynamic-read demand
   transaction may resolve the term against the already-bound caller factors.
   Its temporary concrete demands are specialization scratch, not target tuple
   fibers, target WTO cells or retained target state.
4. Any dynamic-read-derived fact that must survive symbolic composition must be
   represented by an existing symbolic term/effect or a registered formal
   residual keyed by a typed formal root. If neither vocabulary can express it,
   extend the foundational relation operation/registration; never create a
   solve-time fiber or fall back to concrete State.

The same rule applies to the old `structuralPathKey` adapter and route-linked
call-result existentials: formal inventory uses full `formal.Root` identity and
lexical occurrence syntax, never a concrete slot, invocation or route.

## Implementation acceptance

Before wiring formal evaluation, freeze must prove:

1. every descriptor identity is unique and ordinals reproduce the canonical
   order above;
2. every formal cell resolves only descriptors in its owner's span;
3. no solve operation changes descriptor, KeySpace, term, effect or coordinate
   inventory cardinality;
4. every coordinate group is complete under its registered static closure;
5. per-role operations equal the registered whole-factor oracle, including
   skeleton-dependent scalar defaults and complete-tuple identity quotienting;
6. directory root zero materializes each role's typed default exactly; and
7. 1/10/100 callers leave target descriptor count, tuple roots, term/effect
   counts, WTO cells/evaluations and retained target memory unchanged.
