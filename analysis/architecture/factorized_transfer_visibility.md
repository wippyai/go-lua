# Canonical Forest: Factorized Transfer Visibility

Status: design freeze before implementation, 2026-07-17.

This note records the verified production surface behind the current guarded
coordinate pathology.  It is intentionally narrower than the semantic-engine
charter: its purpose is to prevent local performance repairs from growing a
second execution model.

## Required outcome

The production analyzer has one relation forest, one equation scheduler, one
decision kernel, one fixed point, and one publication path.  Every semantic
operation must be represented by one immutable typed plan which supplies its
input roles, exact component dependencies, algebra, application and
publication.  Concrete and guarded execution are adapters over that same plan.

There are no semantic work/depth/node budgets, skipped operations, fallback
solvers, or alternate whole-state execution paths.  Adding or removing a State
axis changes its registration, not forest orchestration.

## Evidence base

The visibility pass combined:

- PromptMap deterministic architecture/import map over 1,284 production files
  (`job_3`);
- PromptMap targeted transform census over 89 transformer files and three
  independent questions (`job_5`, 267 rows);
- PromptMap whole-analysis semantic/dependency refine over 2,084 files
  (`job_4`);
- deterministic `rg` enumeration and direct source inspection; and
- two independent read-only agent audits of transform producers and semantic
  authorities.

PromptMap language-model findings were treated only as leads.  Counts and the
claims below were verified against source.

## The one production spine

```text
relationCodeRuntime.emitNode / emitStep
  -> dirtyCoordinateEdge
  -> coordinateTransform
  -> guardedWorldArena.applyCoordinateTransform
  -> guardedStateKernel.applyCoordinateBlock
  -> ProductDomain.SealProductPatch / ProductPatchBuilder
  -> coordinate contribution
  -> tuple-mu admission, widening/narrowing and publication
```

The forest has 11 production edge-construction sites, six literal
`coordinateTransform` construction sites plus five field-built transforms,
and 25 direct `coordinateBlock` construction sites.  There is one scheduler
and one decision kernel.

The service/lint route uses this spine exclusively.  A separate exported
concrete body engine nevertheless remains callable through `body.Check*`,
`Static.Solve`, `SolvePrepared` and retained sessions.  It owns independent
FIFO/WTO/dense/resume/compare routing in `engine/transfer`, `engine/solve`,
`engine/region` and `engine/solve/concreteflow`, plus observation/edge replay
callbacks retained in `body.Result`.  It has no non-test in-repository service
caller, but it is a real parallel production implementation and must be
deleted after its remaining neutral topology/DTO types are moved.  The
canonical transformer still needs WTO topology construction; it does not need
the old solver.

`executeBoundaryPrefixStep` is a whole-State leaf adapter, not a second solver.
It is nevertheless semantic-ownership debt because its operation switch and
the transformer's separately maintained access certificates can drift.

## Current transform inventory

The edge producers are:

1. choice refinement;
2. normal terminal;
3. prefix operation;
4. identity transport;
5. call selection;
6. call input;
7. normal call output;
8. nonreturning call output;
9. guarded call skip;
10. definition BindIn; and
11. recursive definition-resource feedback.

Prefix operations are effect, external call, root assignment, environment
write, generic-for, diagnostic contribution, branch relations, call results,
presence implications, channel select and covariant exposure.

The 25 direct decision blocks are distributed as follows:

- presence implication: 1;
- outbound boundary transport: 14;
- call-return presence: 1;
- normal terminal/return: 3;
- guarded path/value evaluation: 1;
- generic access-selected transform: 1;
- Values-only term evaluation: 1;
- object literal/member evaluation: 2; and
- inbound receiver completion: 1.

Only the generic access-selected block currently supplies component-kind
metadata to the profiler.  The other 24 increment block/terminal counts but
are invisible in per-component input statistics.  Any performance conclusion
which treats those counters as a complete input-width census is invalid.

## Root cause

The factored State representation is not the defect.  It correctly stores
Values slots, registered lanes, coordinate-family skeletons/scalars,
reachability and diagnostics as separate shared decision roots.

The defect is that a sparse access set is re-densified before evaluation.
`applyCoordinateTransform` gathers every selected root into one
`coordinateBlock`.  `applyCoordinateBlock` then performs a memoized Shannon
traversal over the combined decision vector and calls a whole-fragment leaf
evaluator.  Its work is bounded by the size of the reachable combined decision
product, not by the sum of independent component DAGs.  Memoizing the complete
input vector cannot recover independence that the plan failed to declare.

Observed broad blocks include:

- root assignment (operation 3): up to about 358 roots;
- branch relations (operation 7): about 180 roots;
- non-prefix orchestration (operation 0): about 151--224 roots; and
- presence implication: formerly about 402 roots before its partial local
  specialization.

The many-implications fixture exposes the same structural problem with 65
independent implications.  The former traversal retained histories for their
Cartesian combinations.  The presence-only repair reduced the symptom but
also proved that operation-specific adapters are the wrong abstraction: root
assignment, branch relations and orchestration have the same defect.

A subsequent carrier audit exposed the corresponding cross-axis semantic
defect.  Publishing an absent path consequence is not a path-evidence-only
operation: `State.InvalidatePathKeyDescendants` atomically updates path
evidence, length-floor facts and dynamic-index facts.  Therefore a
presence-family carrier, even with root Values attached, is not an exact
semantic component.  Path mutation/invalidation must be a
ProductDomain-registered capability with an exact dependency closure and
factorwise apply law.  Every participating axis registers its part; the
transfer executor never enumerates those axes.  This same primitive is needed
by root assignment and branch refinement, so it is foundational machinery
rather than a presence exception.

## Why the dependency contract is incomplete

One logical operation's contract is presently split among:

- positional edge inputs;
- `coordinateValueAccess` slot/lane/reachability/diagnostic sets;
- optional operation-specific transform fields;
- explicit input-root lists inside specialized block helpers;
- factapply lane-contract switches;
- the transformer-owned effect Kind x lane catalog; and
- separately inferred profiler metadata.

This causes five concrete design failures:

1. Point-entry, current, caller, callee-outcome, resource and historical roles
   are conventions rather than sealed types.
2. A lane set does not express which reads affect which writes.
3. Coordinate-family transitive closure is operation-specific side machinery.
4. New axes can require edits in access sealing, reconstruction, boundary
   helpers and profiling.
5. Missing exact dependency information defaults to `allCoordinateValues`,
   turning uncertainty in orchestration into multiplicative DD work.

The static operation inventory is also split.  `operationplan.Kind` does not
include signature calls, module loads, metatable attachment or signature
allocations, which use separate point tables, while generic-for uses a second
`ExtensionKind` namespace.  These executable payloads and opaque provider
dependencies must be sealed under one top-level program/operation catalog;
call topology, boundary schemas and observation requirements remain structural
sections of that same plan rather than executable kinds.

## Exact factorization law

Let a semantic transfer `F` write component blocks `B_1 ... B_n`.  A block
`B_i` may be evaluated independently only when its sealed read projection
`I_i` satisfies:

```text
project_Ii(x) = project_Ii(y)
  => project_Bi(F(x)) = project_Bi(F(y))
```

and the plan proves:

- output write sets are disjoint;
- ordered RAW/WAW dependencies are represented as stage edges;
- every family/key/identity/alias dependency is closed transitively;
- all writes are accepted by the same `SealProductPatch` contract; and
- composing the block patches yields exactly `F`, including Bottom,
  reachability, defaults, diagnostics and publication barriers.

Shared read-only inputs do not by themselves join two blocks.  Joint semantic
rules, joint reducers and witness groups do.  Cyclic dependency groups remain
one connected component.  A genuinely joint component is a mathematical
component of the one plan, not a fallback engine.

Factorization occurs inside an equation transfer.  The resulting complete
candidate is still admitted, joined, widened and narrowed at the existing
tuple-mu coordinate.  Widening or narrowing individual transfer blocks would
change the lattice equation and is forbidden.

## Required joint products versus accidental products

Boundary quotienting contains legitimate finite products.  If a destination
must-fact relation has inverse fibers `P1`, `P2` and `P3`, preserving it can
require considering the exact fiber `P1 x P2 x P3`.  This is one joint read
group for one destination fact with a declared quantifier and reducer.

Different destination fibers remain separate even when they share read-only
sources.  The executor's intended cost is the sum of exact destination-fiber
costs, not the product of every destination, State coordinate and guard root.
Injective quotients remain unary.

This distinction applies to diff/store relations, key memberships and path
evidence boundary rebasing.  Removing their exact inverse-fiber products would
lose must-fact semantics and is not an optimization.

## Target plan contract

The final name and package are deliberately left to the implementation review,
but there must be one concept, not parallel operation/access/block vocabularies.
Its minimum information is:

```text
Typed transform plan
  semantic identity and barrier stage
  typed inputs
    PointEntry | Current | Caller | CalleeOutcome | Resource | Historical
  component reads
    Values slots/top
    ordinary registered lanes
    family skeletons and dynamically selected scalar slots
    reachability
    diagnostics
  component writes and structural carry owner
  dependency hyperedges and ordered stage edges
  family-owned transitive closure from semantic seeds
  algebra
    independent homomorphism | finite joint component | quotient group
  the sole semantic application authority
  mandatory profile identity and component metadata
```

The plan is frozen before equation solving.  Runtime abstract values never
change its topology.  Dynamic coordinate inventories may select members of a
predeclared family closure, but cannot discover a new semantic equation.

## Semantic ownership corrections

The following are not optional micro-optimizations.  They are required to make
the typed plan truthful and remove parallel semantics.

1. Move effect, branch and channel lane access from transformer/factapply
   switches into the same registered operation/component authority.
2. Unify path-value and call-return presence plans.  Publication, consequence
   closure and barriers must be one algebra; delete guarded partial-State
   reconstruction.
3. Build symmetric factorwise inbound boundary transport using the existing
   boundary projection/rebase/apply laws; delete active whole-State inbound
   helpers.
4. Extend root-assignment source demand and mutation closure to every shape;
   delete its whole-State mode rather than retaining it as fallback.
5. Introduce one resolved return plan owning result bindings, identity closure,
   heap/placement projection, presence and diagnostics; delete the manual
   guarded N5 semantic island.
6. Freeze one guard plan owning Boolean decision, feasibility, atom inventory
   and backward refinement; delete repeated guard switches.
7. Consolidate diagnostic sequencing and boundary transport.
8. Route standalone body execution through a one-body RelationProgram and
   delete aggregate concrete node/edge production solving, retained sessions,
   alternate schedules and result replay callbacks.

Object construction is the reference model: its Lua value plan and registered
State family protocols are shared by concrete and guarded adapters.  Preserve
that ownership pattern.

Opaque providers such as external-call and generic-for semantics remain one
declared joint component until they expose a typed observation contract.  They
must not silently use an undeclared narrow projection, and the engine must not
fall back to another solver.

## Implementation acceptance

- one mandatory plan is attached to every production transform and every
  direct decision block;
- the 25 handwritten block input inventories disappear or become generated
  views of that plan;
- profiler component counts cover every transform;
- adding a State axis requires only its registered lattice/component laws and
  explicit semantic participation classification;
- concrete and guarded adapters invoke the same semantic algebra;
- no whole-State/fallback mode remains for migrated operations;
- focused State/factapply/transformer tests pass;
- the full unskipped oracle passes under the 4 GiB RSS fuse; and
- only then is cold Kickside measured against the 30--40 second goal.

## Remaining semantic caps found by the full scan

These are not the guarded-transfer pathology, but they violate the same final
architecture contract and are recorded so they cannot be rediscovered later:

- `typ.DefaultRecursionDepth = 4096` changes subtype and generic-call results
  on sufficiently deep acyclic type graphs.  Replace those recursive walkers
  with exact iterative node/pair graph algorithms; do not raise the cap.
- the evidence axis retains at most four origins.  Replace this with an exact
  interned provenance set/graph, or remove provenance from semantic state; do
  not silently truncate evidence.
- retained-solver record budgets abort transactionally, but disappear with the
  parallel retained solver.

Presentation formatting limits, cooperative cancellation, the process-level
4 GiB RSS fuse and mathematical widening are not semantic caps.
