# Discriminant-Keyed Partitioning Design

## Problem

The forward state is currently a single product value at each CFG point. At a
merge, ordinary joins correctly forget path-local facts that are not true on all
incoming edges. That loses correlations where the same discriminant is checked
again after the merge:

```lua
local x
if ok then x = load() end
if ok then use(x) end
```

At the join after the first `if`, `x` is maybe-nil. The second `if ok` selects
the same path family as the assignment, but the current state has no relational
fact saying "when `ok` is true, `x` is the value assigned on that edge".

The same shape appears in status/payload pairs (`value, err = f()`) and in
separate locals correlated by a literal tag. Some status/payload correlation
already exists: transfer lowering infers return-slot presence relations from
body returns (`analysis/lua/transferfacts/return_presence.go`), effect lowering
recovers error-return relations from signatures, and fact application publishes
those through `CallReturnPresenceRelation` into the existing persistent
`PathPresenceImplication` lane.

## Embedding Decision

Use the conditional-facts lane embedding, not full state partitioning.

Full state partitioning would replace each point's state with up to K states
keyed by discriminant valuations. It is the most general Astrée-style trace
partitioning model, but it makes every state lane, join, widen, narrowing pass,
and worklist edge partition-aware. That is disproportionate for phase 1 because
the canonical bug is not that every lane needs a separate trace. It is that a
bounded set of facts should reactivate when the same discriminant is re-proven.

The existing engine already has most of the conditional-facts embedding:

- `analysis/engine/state/pathevidence` owns a must lane for path refinements,
  branch proofs, static member facts, and `PathPresenceImplication`.
- `analysis/engine/factapply/path_presence_implication.go` activates an
  implication when the trigger path is proven and applies the target fact.
- `analysis/engine/factapply/path_state.go` and
  `State.EquivalentStateKeys` provide subtree and equivalent-path invalidation.
- `analysis/lua/transferfacts/conditional_assignment_implications.go` already
  derives local join-point implications for guarded assignments.
- The solver (`analysis/engine/solve`) remains a normal product-lattice
  worklist with widening at `transfer.DefaultWidenAt` points and a bounded
  narrowing phase from c06c7c844.

Phase 1 extends the existing implication lane rather than adding a second
relational state product. A guarded assignment publishes a persistent implication
of the form:

```text
target fact F holds when discriminant path D has abstract value V
```

A later branch on `D == V` reactivates `F`. Writes to either `D` or the target
path invalidate the implication through the same path-evidence invalidation used
for refinements and branch proofs. Dropping an implication is always sound; it
only loses precision.

## Phase 1 Scope

Discriminants are syntactically selected and bounded:

- boolean locals and parameters checked by truthiness (`if ok then ...`);
- literal-string tag fields checked by equality (`if tag == "ready" then ...`);
- no arbitrary expressions, computed keys, cross-function partition carry, or
  unbounded path families.

Phase 1 publishes every canonical partition implication. The candidate set is
naturally finite: each implication is derived from a syntactic branch and a
syntactic assignment or channel-select site in the body. A fixed per-body
quantity cap would silently discard valid correlations and is therefore not an
analysis bound.

Target facts are also deliberately small:

- root locals/parameters assigned under a selected guard;
- tag-correlated separate locals when the assigned target value is concrete
  enough to store as a value refinement;
- existing status/payload return relations remain handled by the call-return
  presence machinery.

## Soundness Rules

The implication is a must fact. It can only remain in state when every incoming
path carries the same implication. Product-state join therefore intersects it
with other must facts, matching the existing `lift.MustSet` semantics.

Invalidation is conservative:

- any write to the discriminant path kills the implication;
- any write to the dependent target path or a subtree that can alias it kills
  the implication;
- equivalent aliases found through `State.EquivalentStateKeys` are included;
- escaping or opaque writes that invalidate the dependent path also invalidate
  the implication;
- function call boundaries do not carry new phase-1 partition implications.

Aliasing through mutable heap paths is handled by existing subtree/equivalence
invalidation. Where alias information is missing, losing the implication is the
sound result. Merging, widening, or cap overflow may drop implications; no rule
promotes a conditioned fact to an unconditional proof unless the trigger has
already been established in the current state.

## Termination

The discriminant universe is finite per body because phase 1 selects only
syntactic boolean roots and literal-string tag paths. The number of published
conditioned facts is bounded by the body cap of 64. The path-evidence lane stores
implications in a finite must set, so each point has a finite ascending chain for
these facts.

Existing widening remains sufficient. At loop heads, the state product calls the
path-evidence lane's `Widen`; the lane uses must-set widening, which can only
drop facts toward top and therefore cannot grow forever. Dropping a conditioned
fact is a sound over-approximation. The bounded solver narrowing pass may recover
ordinary numeric precision, but it does not introduce an unbounded partition
dimension.

Invalidation is monotone with respect to the must set: writes remove implications
or leave them unchanged. Activation meets or writes target constraints in the
ordinary value/path lanes and then re-runs to a local fixed point over the finite
snapshot. It never allocates new discriminants during application.

## Limits And Phase 2

This embedding does not model arbitrary nested trace partitions. It will not keep
different whole-state versions alive for independent discriminants across complex
control-flow joins. If a future corpus needs precision that depends on multiple
simultaneous partitions of every lane, full state partitioning should be
revisited with explicit costs for state explosion, partition-aware widening, and
query APIs.

Phase 2 should consider call-boundary transport for conditioned facts, richer
status/payload inference where today only return presence is known, and a more
general discriminant selector for literal tags without weakening invalidation.
