# Sparse formal tuple and operand-reuse review

Status: implementation audit, 2026-07-18. This review is deliberately limited
to the formal-region cell payload. It does not change the WTO/SCC repair
tracked in Forge `#1907`, and it does not authorize another executable engine.

## Verdict

The repository already has almost all of the semantic primitives needed for a
physically sparse formal cell, but it does **not** have a reusable formal cell
container. The correct implementation is not `state.State`, a slice with one
entry per axis, or the current `guardedCoordinateFactor`. It is:

1. the existing `relationCode` term/effect references;
2. the existing shared reduced ordered decision DAG for guard correlation;
3. the existing `ProductDomain` factor and coordinate-family lattice laws; and
4. one small immutable, hash-consed, fixed-shape fiber directory whose leaves
   are decision roots and whose updates are sparse deltas.

The directory is structural storage, not a semantic IR. It contains no
expression, effect, call, route, or lattice operation of its own. Its only job
is to make a complete product tuple persistent: identical subtrees are shared,
an affected fiber path is copied in `O(log F)`, and a binary product operation
descends only into unequal subtrees. `F` is the sealed fiber count for the
lexical relation forest.

This is the smallest design that simultaneously preserves:

- the full guarded correlation of the symbolic relation;
- exact registered semantics for every installed axis;
- operand identity when a transfer is a semantic no-op;
- affected-fiber updates instead of cloning a 17-axis `State`; and
- the one-cell-universe rule of `formalRelationRegionInventory`.

There is one honest limitation in the current registration surface. Seven
dependent lane families are transposed to skeleton/scalar fibers today. The
remaining keyed lanes are opaque `LaneFactor`s, so they are sparse at lane
granularity but a genuine write to one of them may still scan or clone that
lane's internal finite map. Also, a changed coordinate-family skeleton may
change defaults/support for many scalar coordinates; without a registered
skeleton-delta affected-cone law, the exact fallback is a scan of that one
sealed family inventory. Neither limitation justifies scanning unrelated axes.

## What can be reused unchanged

### Frozen relation identity and schedule

`formalRelationRegionInventory` already freezes node, step, and outcome cells
from sealed `relationCode`, records every influence, and constructs one WTO
before evaluation (`formal_relation_region_inventory.go:10-143`). That is the
cell universe and scheduling authority. The payload proposed here is indexed
by those cells; it does not discover cells, call targets, loops, or fibers while
solving.

`relationCode`, `ValueTerm`, `PathTerm`, `Guard`, `EffectTerm`,
`boundaryOutcomeTuple`, `relationApplyRef`, and `callFrameTerm` remain the sole
semantic syntax. A formal component terminal refers to those existing sealed
objects by arena owner plus existing ID. It never copies a term DAG and never
converts an input-dependent value to `product.Value` before final Apply.

### Guard decision algebra

`decisionKernel` is already the required persistent guard algebra:

- unique-table reduced ordered nodes;
- terminal interning;
- memoized apply and ITE;
- care-set restriction; and
- `applyUnderCare`, which reuses the only live operand where one input is
  unreachable instead of manufacturing component Bottom masks.

The pure `decisionKernel` can be retained as a private transformer primitive.
Its current placement in `decision_diagram.go` and the `guardedConditionKey`
name in one memo table are historical naming, not semantic route ownership.
The formal implementation needs a new formal terminal authority around the
same kernel; it must not reuse `guardedStateKernel`.

`partitionLeafPairs` and `partitionLeafTuplesUnderCare` are also reusable for
the few genuinely joint queries. They enumerate only correlated reachable
terminal tuples, rather than a Cartesian product of component supports.

### Registered product and factor laws

The following `state.ProductDomain` operations are the semantic TCB and should
be called directly by formal leaf operations:

- `LaneBottom`, `LaneSame`, `LaneEqual`, `LaneLessOrEq`, `LaneJoin`,
  `LaneMeet`, `LaneWiden`, `LaneNarrow`, and `LaneFingerprint`;
- `CoordinateSkeleton{Join,Meet,Widen,Narrow,Equal,RepresentationEqual,Hash}`;
- `CoordinateScalar{Join,Meet,Widen,Narrow,Equal,Hash}`;
- `CoordinateScalarSupport` and `CoordinateDefault`;
- `CoordinateFactorInventory` and its registered closure; and
- `VisitLaneValueDependencies` and identity-substitution policies.

These APIs dispatch through sealed registration ordinals, so adding or
removing an axis changes its descriptor, not the formal solver.

`ValueFactor[K]` is the one generic finite-map lattice and may be instantiated
over `FormalSlot` for genuinely ground registered residual values. It must not
be used to replace an input-dependent `ValueTerm`: the identity/correlated
return counterexample in `formal_carrier_precision_review.md` still applies.

The `LaneSame` hook is especially valuable. It proves physical operand reuse
without an equality scan. `LaneEqual` plus the existing semantic fingerprint
interner is the collision-authority fallback. The formal path must not copy the
old `reuseOrInternLaneResult` rule which chooses the first lattice-equal
operand: that rule is order-sensitive and is already convicted in
`operator_plan_totality_design.md`.

### Coordinate-family representation

The reusable mathematical decomposition is:

```text
coordinate lane = registered family skeletons
                + sorted explicit scalar fibers
                + registered defaults for omitted optional fibers
```

`CoordinateFamilyShape`, `CoordinateScalarSupport`, the family hash/equality
operations, and `CoordinateFactorInventory` provide this decomposition without
exposing family payloads. The current guarded implementation correctly proves
two subtle laws which the formal implementation must keep:

1. skeleton and scalar roots are one dependent family tuple, not separately
   publishable arrays; and
2. omission means `ImplicitDefault`, not scalar Bottom.

The boundary-specific `CoordinateBoundaryFamilyLift` is reusable when an
actual formal effect performs boundary transport: it precomputes exact
affected wires and evaluates one destination fiber. Its
`destinationAffected` law is **not** a generic Join/Widen affected-cone law and
must not be repurposed as one.

### Sparse patch discipline

`ProductPatchPlan`/`ProductPatchBuilder` and `coordinateSparsePlan` demonstrate
the right access discipline: a frozen operator declares reads and writes, a
leaf sees only declared operands, and publication is transactional. The formal
evaluator should consume their sealed access topology and emit fiber IDs plus
decision roots. It should not reuse their concrete-State leaf adapter or their
route-owned locator types.

The fixed-shape replacement algorithm in
`coordinate_contribution_fold.go` is the right implementation pattern for the
fiber directory: immutable inventory, stable leaf ordinal, one root path per
replacement, and no semantic work budget. Its current concrete type is owned
by `guardedWorldArena` and joins route coordinates, so only the algorithm
should be factored into a generic structural arena. Forge `#1907` remains the
owner of the separate incoming-contribution/SCC repair.

## What is route-owned and must not be reused

The following types are evidence about the historical implementation, not
building blocks for the formal one:

- `semanticRoute`, `semanticRouteInventory`, `guardedWorldOwner`,
  `relationInvocationRef`, and invocation namespaces;
- `guardedStateAuthority` and `guardedStateKernel`;
- `guardedStateTerminal`, `guardedComponentTerminal`, and every component key
  containing `authority`, `route`, or invocation identity;
- `guardedOwnedFactor` and `guardedCoordinateFactor` as concrete types;
- `guardedBoundaryOutputTransform` and the transformer-side guarded boundary
  executor;
- `coordinateSparseComponent`, `coordinateComponentLocator`, and
  `coordinateSparseFamilyLaneLocator`, because their ownership includes a
  route and concrete guarded authority;
- `factorCoordinate`, `materializeFactor`, and any path which decomposes or
  recomposes `state.State`; and
- `coordinateContributionFold` as currently typed, because its Join callback
  calls `guardedWorldArena.combineCoordinate`.

The strongest warning is `guardedComponentTerminal.value product.Value`.
Reusing that terminal type for formal output bindings would silently turn
`return p` into a ground abstract value and lose the functional relation.

`guardedBoundaryOutput.go` is similarly unsuitable as the Apply mechanism. It
builds destination/source guarded factors, concrete boundary plans, root
selections, and call-result values. The formal Apply must retain target-owned
terms/effects under the lexical frame binding and use state boundary lifts only
at the final registered effect transaction.

## Concrete payload

The following types are schematic private types, not a proposed public API or
new serialized vocabulary:

```go
type formalFiberID uint32
type formalTupleRoot uint32

type formalTuple struct {
    owner relationVar       // lexical relation owner, never caller/invocation
    root  formalTupleRoot   // immutable product-directory root
}

type formalTupleDelta struct {
    owner  relationVar
    writes []formalFiberWrite // sorted, unique, exact declared write set
}

type formalFiberWrite struct {
    fiber formalFiberID
    value decisionRef        // root in the one shared guard DD
}

type formalFiberTreeNode struct {
    level       uint8
    left, right formalTupleRoot
    // At a leaf, value is a typed component decision root.
    value       decisionRef
}
```

`formalTupleRoot(0)` is the canonical all-default subtree. The descriptor for
each leaf supplies its typed default decision root: unreachable/absent,
registered factor Bottom, omitted optional scalar, or no effect/outcome. Thus
zero does not pretend that all semantic defaults have the same Go value.

The tree arena interns `(level,left,right)` and `(fiber,value)`. A point update
creates at most one node per tree level and returns the original root when the
leaf is unchanged. Internal nodes whose children are both zero reduce to zero.
This is a persistent sparse segment tree, not the dense mutable `nodes` array
inside the current contribution fold.

The sealed `formalFiberDescriptor` inventory has these leaf roles:

```text
Reachability/care                   one Boolean decision root
Symbolic output/register binding   FormalSlot -> arena-qualified ValueTerm
Symbolic path binding              formal root -> arena-qualified PathTerm
Lexical ordered effect occurrence  arena-qualified EffectTerm + enable root
Correlated outcome occurrence      existing boundaryOutcomeRef + enable root
Ordinary registered residual lane  ProductLane -> LaneFactor
Coordinate family skeleton         CoordinateFamily -> skeleton factor
Coordinate scalar fiber            CoordinateSlot -> scalar factor/default tag
Ground formal residual Values      ValueFactor[FormalSlot], only when genuinely ground
```

An arena-qualified term/effect leaf is only `(relationVar, existing ID,
existing lexical frame provenance where required)`. It is not a new term node.
Effects receive fixed lexical occurrence ordinals from `relationCode`; the
fiber inventory therefore preserves source order without storing or joining
mutable effect slices.

Every coordinate family's skeleton and scalar range is contiguous and marked
as one joint group. Ordinary lanes are one leaf each. Values/output slots and
effect/outcome occurrences are sparse individual fibers. The descriptor array
is retained once per frozen forest; cells retain only a tree root.

## Join, widen, meet, and narrow

### Directory traversal

For operation `op(left,right)`:

1. If the tuple roots are identical, return that root. Idempotence covers Join,
   Meet, Widen, and Narrow at equality.
2. Resolve and combine the two care fibers first.
3. Recursively compare the two product directories. Equal subtree IDs are
   reused without inspecting their fibers.
4. A default subtree is handled by the descriptor's algebraic identity rules;
   it is not expanded merely because it is physically absent.
5. At an unequal leaf, combine its two component DD roots with the shared
   care-aware decision operation. Leaf terminals call the descriptor's exact
   registered or symbolic operation.
6. Intern the resulting directory path. If both children are unchanged, reuse
   the operand node; if the complete root is unchanged, publish nothing.

For `k` changed leaves this allocates at most `O(k log F)` directory nodes and
usually fewer due to shared paths and hash-consing. It performs zero work on an
identical subtree containing unrelated axes.

### Guard correlation

Join and Widen use `decisionKernel.applyUnderCare`. Where only one operand is
reachable, its exact decision root is reused; where both are reachable, the
typed leaf operation executes; where neither is reachable, the directory leaf
is default. Meet restricts both operands to the intersection care before the
leaf meet. Narrow uses the ascending predecessor's care and each registered
lane's exact `Narrow` law.

Care restriction of a joint coordinate-family group must choose one cofactor
witness for its skeleton and scalar roots. The vector restriction algorithm in
`guarded_coordinate_family.go` is mathematically reusable after removing its
route/factor ownership. Restricting family fibers independently is forbidden.

### Symbolic term and effect leaves

Symbolic leaves do not call `product.Join`:

- identical arena-qualified references reuse the operand;
- alternatives on disjoint guards remain distinct DD branches;
- a lexical `Choice`, `valueJoin`, or loop-mu result uses the already-frozen
  existing term named by `relationCode`; and
- if two distinct symbolic references must be joined on the same live guard
  valuation and sealed relation syntax provides no existing join/mu term, the
  cell is an incomplete carrier and evaluation rejects. It may not intern a
  new runtime term, store Top, or replay concrete State.

An effect occurrence has one immutable existing `EffectTerm`; its decision
root denotes enabled/absent. Join unions the enable regions for the same
occurrence. Two different effects claiming the same frozen occurrence are a
malformed relation. Outcome occurrences follow the same rule. This keeps
effect/result correlation in the shared decision DAG while preserving lexical
effect order.

### Registered residual leaves

For an ordinary lane factor:

1. test terminal identity;
2. test `LaneSame` and reuse that exact operand;
3. invoke `LaneJoin`, `LaneWiden`, `LaneMeet`, or `LaneNarrow` only if needed;
4. intern by `LaneFingerprint` plus `LaneEqual`; and
5. never choose “first equal operand” as a canonicalization policy.

Scalar Values residuals use `ValueFactorLattice[FormalSlot]` with its existing
`Same` hook. Formal identity substitution uses the one complete-tuple
substitution plan, not a lane-local inverse fiber.

### Coordinate-family groups

If both family skeleton DD roots and a scalar subtree are identical, reuse the
group root. If the skeleton is identical, descend only into unequal explicit
scalar fibers and use `CoordinateScalar*` operations plus the registered
default for omission.

If the skeleton changes, scalar support/default may change even for a
physically omitted slot. The current safe operation is family-local:

1. materialize only that one registered family lane from its sealed skeleton
   and scalar inventory;
2. call the exact whole `Lane*` operation;
3. decompose only that family lane; and
4. patch the resulting family subtree.

This may scan the sealed family inventory, but it never scans another family
or axis. Strict affected-cone behavior for skeleton changes requires one new
registered coordinate-family law returning the exact slots whose
support/default can change. Boundary `destinationAffected` is not that law.

## Transfer and contribution updates

A frozen operator plan resolves each declared write directly to a
`formalFiberID`. The transfer returns `formalTupleDelta`; applying it performs
sorted unique point updates against the input tuple root. Structural carry is
therefore pointer sharing, not copied slices or a scan of all registered lanes.

The outer incoming-contribution fold and the inner product directory are
different structures:

```text
ContributionID segment tree  -- Forge #1907; changes one incoming edge
            leaves contain complete formalTuple roots
formal tuple segment tree    -- this review; changes one semantic fiber
            leaves contain guarded component roots
decision DAG                 -- existing; changes one guard-correlated value
            leaves contain existing terms/effects or registered factors
```

They may share a generic fixed-shape tree implementation, but not ownership or
semantic callbacks. A changed contribution costs `O(log incoming)` outer
recomputations; each tuple combination skips shared product subtrees and
touches only its semantic delta.

## Current allocation and scanning risks

The current route carrier exhibits the costs the formal payload must avoid:

- `guardedCoordinateFactor` owns fresh lane/family/slot slices; conditioning
  and restriction allocate and copy those inventories.
- `factorCoordinateWithReachability` normalizes a complete State, decomposes
  Values, enumerates every coordinate product lane, decomposes every registered
  family, and interns every explicit scalar.
- `materializeFactor` performs the inverse whole-product reconstruction.
- `combineCoordinateFactors` loops all ordinary lanes, all family lanes, and
  the ordered union of every Values slot even when one transfer changed one
  fiber.
- `combineCoordinateFamilyLanesExact` materializes and recomposes an entire
  dependent lane at each correlated terminal pair.
- `factorFingerprint` hashes every stored root in the tuple; a persistent
  directory already has an interned structural identity and does not need this
  whole-vector hash on every update.
- `internProductLane` may serialize/fingerprint a whole factor after a lane
  operation missed operand reuse.
- `lift.Map` performs one or two full pointwise order scans before a novel Join
  and allocates a complete union map. Its excellent identity/Bottom fast paths
  help only when the opaque lane operation is actually reached.
- `CoordinateFactorInventory.Slots` and `FamilySlots` return detached slices;
  use them at freeze time, not per cell evaluation.
- `coordinateSparsePlan` uses maps and rebuilt component slices at execution;
  compile its topology to dense fiber ordinals once instead.
- the current `coordinateContributionFold.clone` copies its dense node array;
  a formal tuple needs immutable path copying/hash-consing, not dense cloning.

The expected formal hot-path cost is consequently:

```text
unchanged transfer        O(1) tuple-root reuse
one fiber write           O(log F) directory nodes + changed component work
k sparse writes           O(k log F), with shared paths reducing the constant
tuple Join/Widen/Narrow    O(unequal product subtrees + changed DD regions)
opaque affected lane      registered lane cost only
changed family skeleton   one-family scan until an affected-cone law exists
```

No operation above has a semantic cap or early termination budget.

## Implementation order and acceptance gates

1. Factor the pure fixed-shape/hash-consed structural directory from the
   `coordinateContributionFold` algorithm. Do not move route types into it.
2. Freeze the formal fiber descriptor inventory from `relationCode`,
   `SlotSpace`, ProductDomain registration, and closed coordinate inventories.
3. Add a formal terminal arena around the existing `decisionKernel`. The arena
   accepts only existing arena-qualified terms/effects and opaque registered
   factors; no `State` terminal exists.
4. Implement sparse deltas and the typed leaf Join/Meet/Widen/Narrow dispatch.
5. Implement dependent coordinate-family group restriction and the
   skeleton-unchanged sparse fast path; retain the exact one-family operation
   for skeleton changes.
6. Bind formal-region node/step/outcome equations to tuple roots. Keep the
   outer contribution repair owned by Forge `#1907`.
7. Wire sole production Apply and delete the route runtime atomically, per
   `symbolic_apply_implementation_map.md`.

Before production wiring, tests must prove:

- identity, guarded return, ordered effects, and heap/path alias witnesses;
- exact tuple equality against whole registered ProductDomain operations for
  every lane and coordinate family;
- a one-fiber transfer visits no unrelated lane/family descriptor;
- unchanged operands allocate zero tuple/component nodes;
- one changed fiber allocates at most tree depth plus its DD/component result;
- skeleton change scans only its family and matches the whole-lane oracle;
- 1/10/100 callers leave target tuple roots, cell evaluations, DD nodes, WTO
  iterations, and retained target memory unchanged; and
- full unskipped oracle equivalence with route count exactly zero.

Useful always-on counters are `tupleDirectoryNodes`, `tupleRootReuses`,
`fiberLeafCombines`, `laneFactorOps`, `familySkeletonChanges`,
`familyScalarVisits`, `decisionApplyOps`, and `runtimeTermNodesCreated` (which
must remain zero after sealing).

## Bottom line

The product decomposition is not the defect. The defect is storing and
combining that product as route-owned whole-State vectors. A persistent sparse
fiber directory over the already-registered laws removes the 17-axis scan from
unchanged and locally changed transfers without creating another semantic
implementation. The one remaining generalization frontier is explicit and
small: opaque keyed lanes and skeleton-changing coordinate families need
registered affected-cone laws if profiling shows their family-local work is
still material.
