# Operator Plan Totality: Access Certificates, Selector Algebra, and Deterministic Admission

Status: consultation-converged design, 2026-07-18. Two-round adversarial review
(orchestrator proposing, Sol countering, both verifying against source).
Refines journal #1798 and completes `factorized_transfer_visibility.md` with the
enforcement mechanics that document deliberately left open. Evidence base:
width-incident dossier (nine incidents), verified access-surface map
(11 declaration constructs, 6 whole-width sites, 9 admission mechanisms), and
direct source citation throughout.

## Verdicts

1. The composition (relational summaries over a factored reduced product,
   guarded DD worlds, tuple-mu/WTO, fail-closed exactness) is sound. The nine
   width incidents are implementation-shaped, with two structurally wrong
   implementations inside the sound composition:
   - **Physical full-product boundary transport.** A relation may denote all 17
     lanes; the executor must transport untouched components by root identity.
     Today `applyGuardedBoundaryOutput` clones the full destination and loops
     every family and lane — O(inventory) regardless of write closure.
   - **Quadratic admission refold.** `admitCoordinate` left-folds every
     incoming contribution from seed on each edge replacement: Θ(k²) at
     fan-in k. This, not access width, is the mechanism behind the
     recursive-cyclic-wide regression.
2. Access totality (#1798) is necessary but not sufficient; both structural
   fixes above are required alongside it.
3. The cost asymmetry vs the legacy engine is unproven-leaning-transitional;
   the deciding experiment is a matched-plan dual-adapter benchmark (guarded
   DD vs single-world adapter over the same frozen program, plans, and trace),
   runnable only after plan totality lands.

## The sealed OperatorPlan

Every semantic operation is admitted to the equation graph only as one
immutable `OperatorPlan`:

- closed input-role sum: `PointEntry | Current | Caller | CalleeOutcome |
  Resource | Historical`;
- component references: exact Values slot, Values-top control, ordinary lane
  factor, registered keyed lane projection, coordinate-family skeleton,
  dynamically selected coordinate scalar (via the selector algebra below),
  reachability, diagnostics, typed sidecars;
- per-output read sets, write sets, structural carry source;
- dependency hyperedges and ordered RAW/WAW stage edges;
- algebra kind: independent homomorphism | finite joint group |
  quotient/reducer group;
- evaluator and profile identity.

There is no open-ended `All` constructor. A plan may in fact select every
current lane, but only by exhaustive operation-by-lane declaration; it cannot
say "all current and future lanes". Adding a lane reopens catalog admission
(the #1603 lane-dependency precedent, lifted to operations).

## Selector algebra (dynamic coordinates)

The current `coordinateSelectionContract.Select(domain, []CoordinateSlot)`
hands selectors the complete inventory — All with a callback around it. It is
replaced by a closed algebra:

```text
Selector := Exact(Family, KeyExpr)
          | Closure(Family, CapabilityID, NonEmpty[SeedExpr])
          | Query(Family, CapabilityID, QueryOperandSchema)
          | Union(NonEmpty[Selector])
          | Push(BoundaryMorphismID, Selector)
          | Pull(BoundaryMorphismID, Selector)
```

No predicates, wildcards, complements, empty-seeds-means-all, or inventory
arguments. `CapabilityID` is a mandatory family-catalog registration (exact
key, path-descendant closure, alias closure, dynamic-index query, identity
fiber, route quotient). Families expose capability-specific indexed operations,
never `IterateAll`.

The enforceable width law: a selector may visit and return only the semantic
cone induced by its nonempty seeds/query through its registered capability;
it may not scan the family inventory to discover that cone. A genuinely global
exact cone is paid for or rejected transactionally — never capped.

Solve-time keys: the plan freezes the key constructor schema (structural
path+suffix, allocation/producer site + provenance, dynamic-index
(Table, Site) + evidence, call point/ordinal/identity, canonical
variant/type/literal), not the eventual keys. New-key materialization inserts
into per-capability family indexes and wakes affected selectors in
O(log inventory + affected selectors); no plan re-admission. A key that would
require a new stage, hyperedge, or operator is a topology mutation and rejects.
Missing scalars use the family's declared default/skeleton law — never
Bottom-filling, which converts an illegal read into a silently wrong result.

Selectors cross boundaries through sealed family morphisms only (`Push` via
rebaseKeys, `Pull` via inverseFibers, must-quotients via sourceFiber,
structural carry via destinationAffected). A family that cannot transport a
selector exactly rejects at seal time.

## Deterministic incremental admission (refold fix)

Replace the seed-left-fold with a fixed-shape segment tree over stable
`ContributionID`s (sealed forest identity + edge ordinal, sorted once at
scheduler construction). Absent contributions hold the canonical join
identity; replacing one edge recomputes its ancestor path only: O(log k) with
canonical order preserved. Dynamic insertion after solving begins is forbidden
(topology is frozen). Narrowing rebuilds or transactionally updates the tree.

Required representation law at every lane/scalar join, normalized locally at
each recomputed binary node (never by whole-result re-normalization):

```text
Canon(Join(a,b)) == Canon(Join(b,a))
Canon(Join(Join(a,b),c)) == Canon(Join(a,Join(b,c)))
Equal(a,b) => Canon(a) == Canon(b)
```

`reuseOrInternLaneResult`'s first-lattice-equal-operand fallback is
order-sensitive and must be removed unless catalog normalization proves
equal-implies-identical-spelling. Cross-solve identity is canonical content,
fingerprints, summary bytes, and result versions — never solve-local DD
integers (invariant #13).

## Zero-allocation enforcement (no ReadView interface)

Static operators: at plan seal, every role/component locator compiles to dense
operand and output ordinals; an equation instance owns reusable root,
terminal, patch, and bitset scratch sized from that layout; dispatch is a
closed switch on operator kind. Undeclared reads are physically
unrepresentable — the evaluator receives neither `state.State` nor the
complete leaf vector, so no accessor can name an undeclared component.

Dynamic paths receive a concrete `DemandCursor` yielding only tokens produced
by the sealed selector capability; out-of-capability requests, wrong-family
tokens, non-monotone rounds, and missing defaults reject immediately.

The sole whole-State adapter is `ProductDomain.BindOperands(plan, state,
scratch)` inside the ProductDomain TCB: validates the plan/domain seal,
invokes only the plan-named lane/family projections, returns a concrete
operand bundle. Lane get/set closures remain substrate-internal. Undeclared
writes go through a plan-bound patch writer and abort transactionally.
`State.Edit` becomes internal or requires an unforgeable plan write permit.
`MaterializeInputs` disappears from migrated operators.

## Enforcement layers (affordable noninterference)

1. Always-on structural enforcement (release builds): binders expose only
   declared operands; patch writers accept only declared output ordinals;
   cursors validate every token. No overhead beyond the binding itself.
2. Access-audit CI mode: preallocated bitsets in binder/cursor/family-index
   record resolutions, demands, writes, entries visited, fibers traversed;
   assert touched-subset-of-declared, complete writes, and scanned-keys ⊆
   demanded-cone (component-level tracking alone cannot see intra-lane
   whole-map scans — `projectDynamicReadFacts` is the live example).
3. Catalog lawsuite: the perturbation law (two stores equal on the declared
   projection, different outside; identical declared outputs and
   terminal-vector counts) runs once per registered projection, closure,
   selector capability, and boundary morphism — not per equation.
   Deterministic small cases per PR; wide inventories under nightly fuzz.

Performance gates pin counters — selected roots, scanned entries, DD pair
applications, allocations — on `many-implications`, `recursive-cyclic-wide`,
and `type-engine-edge-matrix`; timing alone is too noisy. Every future closed
pathology joins this gate set.

Admission enforcement is three-way, mirroring the lane-catalog precedent:
catalog build panics on unset/duplicate operation-by-lane cells; scheduler
admission rejects any edge without a sealed plan identity; an AST architecture
test forbids raw `dirtyCoordinateEdge{}`, `coordinateTransform{}`, and
`coordinateBlock{}` construction outside the central builder/executor files.

## Sequencing

1. **Incremental refold** — independent of the relationCode/Apply cutover and
   of plan totality; lands first. Canonical join laws, remove order-sensitive
   reuse, fixed ContributionID segment tree.
2. **Plan totality** — freeze the OperatorPlan layout now; every new
   relationCode/Apply edge takes it as a mandatory constructor argument;
   legacy transforms run through binders generated FROM plans (never plans
   inferred from legacy access fields — that preserves drift). Subsumes the
   split access ownership, the 25 handwritten block inventories, and frozen-doc
   corrections 1, 2, 4, 5, 6.
3. **Boundary transport-by-identity** — depends on total plans; one block per
   affected identity/fiber, project only its inverse source fiber, structural
   retention for all other destination roots; subsumes frozen-doc correction 3
   and the 14 outbound handwritten boundary blocks.

## Incident validation

Each historical incident is unrepresentable or rejected under this design:
1–2 (inventory replay): reverse dependencies derive solely from plan
hyperedges. 3–4 (whole-lane dynamic reads/carriers): ordinary lanes require
registered keyed projections; the surviving whole-map scan in
`projectDynamicReadFacts` is incident 3/4 still alive and migrates first.
5 (union-block conditioning): one block per write-connected dependency
component. 6 (whole-State external call): providers receive checked views or
typed operands, never State. 7 (all-lanes provider): participation decided by
the operation-by-lane catalog, no provider-owned width. 8 (recursive-wide
regression): O(log k) refold plus counter-pinned gates. 9 (many-implications):
per-implication blocks; terminal vectors additive in implication count.
