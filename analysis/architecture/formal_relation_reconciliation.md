# Canonical formal-relation reconciliation

Status: active, 2026-07-18. This file is the current execution contract. It
supersedes concrete-`State` application worlds and route-owned callee equation
state described by the historical v4 migration document.

## Goal

The production analyzer has one engine and one semantic fixed point. Each
lexical body is reduced once to a reusable formal relation. A known call
composes that relation by exact substitution; it never re-enters or re-solves
the callee body. Projection and publication consume the stabilized relation
once. There is no legacy fallback, feature flag, semantic cap, skipped unit, or
second executable interpretation.

The 17-axis product, transfer semantics, diagnostics, manifests, observations,
and oracle meanings remain. Adding or removing an axis is registered product
algebra, boundary transport, and codec work—not scheduler surgery.

## Canonical carrier

The previously proposed plain registered product over physically disjoint
`IN`, `MID`, and `OUT` vocabularies is rejected as the complete transformer
carrier. It separates roots correctly, but the Values factor is non-relational:
solving `return p` with an unknown formal input records `OUT(result)=Top`, and
later binding `IN(p)` cannot recover the caller value. Store relations are
ownership evidence, not a universal copy relation. A plain doubled product
would therefore lose precision on the identity function before considering
guarded correlation.

The sole transformer syntax must reuse the existing immutable `relationCode`
and its hash-consed `ValueTerm`/`Guard` DAG. That syntax already represents
roots, copies, refinements, selections, operators, calls, and typed recurrence;
creating a second expression IR is forbidden. Registered product factors remain
the exact algebra for heap/path/effect residue. The final carrier is not sealed
until it proves all three witnesses without concrete body replay:

1. `return p` instantiated at two distinct caller abstract values returns each
   value exactly;
2. a guarded correlated return preserves the guard/result relationship; and
3. heap alias/effect substitution retains the registered factor semantics.

Until those proofs pass, there is no executable formal schema. In particular,
the engine may not bridge through concrete `State`, treat term structure as a
semantic lattice order, or retain the route runtime as a fallback.

Identity-bearing factors use one closed term type:

```text
Concrete(identity.ID) | Formal(FormalVar) | Allocation(AllocationTemplate)
```

`FormalVar` is lexical schema identity plus boundary vocabulary. Allocation
templates are instantiated only by the boundary allocation authority. No
string-encoded template ID and no second formal factor representation may
remain. Path roots are separated by the existing sealed boundary rebase laws.
Scalar relation roots do not reuse concrete packed `key.Value` cells: the
relation Values factor instantiates the same generic value-map lattice over a
typed `FormalSlot`. A forest-owned `SlotSpace` maps its dense runtime indexes
bijectively to full 32-byte lexical body identities and validates root ordinals;
canonical slot identity is the full body identity, root ordinal, and vocabulary.
This prevents `IN(g)` and `OUT(g)` from collapsing without hashes, global
interning, synthetic call-result cells, or widening concrete state keys.
That structural identity is the single neutral `formal.Root` used by both
`FormalSlot` and formal path roots. Registered residual-lane dependency laws
report a closed concrete-slot-or-formal-root carrier, so path evidence and its
Values coordinate cannot detach during composition. The transformer may not
recover this coupling by inspecting opaque lane payloads.
Identity substitution generalizes the existing all-lane quotient walker; it
does not add another traversal.

Composition must remain one exact symbolic transaction:

```text
caller OUT -> MID
callee IN  -> MID
callee OUT -> fresh OUT
compose the existing ValueTerm/Guard equations and registered residue factors
existentially eliminate MID and target-local coordinates
project caller IN -> fresh OUT
alpha-rename allocation templates at the call boundary
```

`Bottom`, singleton, and `Top` substitution images retain their lattice
meaning. Direct-key `Bottom` makes the relation unreachable. Direct-key `Top`
uses each registered factor's exact unknown-image law; it is never a failure
budget or an approximation.

## Sole schedule

The cell universe is frozen from lexical relation code before evaluation and
keyed only by relation variable plus structural node/step/outcome identity.
One frozen WTO schedule owns call-SCC and lexical-loop feedback. Cells may not
be discovered while evaluating. Widening is applied only at declared feedback
heads. Joint narrowing continues to equality without a pass count; growth
returns to ascent. Cancellation publishes nothing.

## Atomic cut

Preserve the immutable relation-code syntax, registered product laws, shared
decision kernel, frozen WTO inventory, and generic region solver. In the same
cut that makes formal relations executable, delete the route-owned concrete
runtime: relation code executor, per-route coordinate schedulers/equations,
dirty/contribution/block schedulers, guarded runtime vocabulary, concrete
application projections, and route-owned publication views.

Publication becomes one lexical result per body with a route-free semantic
fingerprint and an input-boundary-closed proof. Only after those exist may the
remaining `ApplicationDependency`, call-context suppression, and route lineage
fields be deleted; they must not be bridged with placeholders.

## Gates and performance

The first performance gate proves that one helper's body work is identical for
1, 10, and 100 callers while only cheap Apply instantiations scale. Then run the
full, unskipped oracle on the sole engine under `GOMEMLIMIT=3GiB` and the built-in
4 GiB RSS fuse. Only after it is green, measure cold Kickside and iterate on its
actual offenders and allocations toward 30–40 seconds. No fixture, unit, or
diagnostic may terminate early to improve the number.

Current order:

1. finish canonical identity terms and exact registered-lane substitution;
2. seal the formal relation carrier and composition algebra;
3. wire the frozen WTO cells and atomically delete route-owned execution;
4. bind route-free publication and delete remaining route lineage;
5. full unskipped oracle, then real Kickside profiling and optimization.
