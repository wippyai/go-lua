# Sealed Relational Analysis Engine

Status: canonical implementation design, pre-implementation

This document defines the replacement for the analyzer's form-specific runtime.
It is intentionally a small, specialized system: a sealed, lattice-valued,
incremental relational engine for static analysis. It is not a general-purpose
database and it is not a retrofit of `analysis/engine/execution`.

The implementation proceeds in isolated packages. Existing rule declarations
remain the authoring source. Production is rewired once, after the replacement
executes the complete corpus, and the old execution protocol is deleted in the
same cut.

## 1. Governing decisions

1. Domains declare relations and retain their irreducible mathematics: lattice
   algebras, typed judgments, typed derivations, and codecs.
2. Schema seal compiles declarations into one immutable logical execution
   schema. Runtime never classifies rule forms or reconstructs logical routes.
3. A separate checker certifies that schema. It does not reuse compiler
   validation helpers.
4. Mount specialization binds stable logical identities to solve-local physical
   addresses and arrangements. Dense ordinals are never logical identities.
5. The engine maintains solve-local relation state by monotone ascent and
   incrementally evaluates standing plans to a fixpoint.
6. `analysis/snapshot` remains the canonical immutable publication store. It is
   not duplicated or wrapped by a parallel snapshot representation.
7. Scope, semantic values, lineage, and deltas are separate algebras.
8. The current runtime may be used only as a separately built external oracle.
   There is no in-process adapter, shadow runtime, feature flag, or family-by-
   family production cutover.
9. Version one performs deterministic lowering. It has no cost-based optimizer
   and does not reorder authored joins.
10. A new logical operator is admitted only after an actual analysis query is
    proven inexpressible using the existing algebra and semantic ABI.

## 2. The three floors

```text
domain declarations and named semantic operations
                        |
                        v
        immutable logical ExecutionSchema
    relations, typed expressions, dependencies, SCCs
                        |
                        v
       mounted physical plan and typed runtime
 arrangements, relation state, delta evaluation, WTO
                        |
                        v
        immutable snapshot and read-only consumers
```

The middle floor is the missing abstraction today. Declarations currently fall
through into generated descriptors, installers, form classification, and
hand-written execution choreography. The new logical schema is the single
canonical statement between declarations and execution.

## 3. Package placement

No production files live directly under either `analysis/relation` or
`analysis/engine/relation`.

```text
analysis/relation/
    schema/
        model/    immutable relation, column, key, scope and presence vocabulary
        algebra/  closed logical expression vocabulary
        plan/     immutable ExecutionSchema, dependency graph and SCC policy
    semantic/
        outcome/   closed outcome and presence vocabulary
        signature/ sealed operation signatures, delivery and outcomes
        binding/   typed factories and solve-local workers
    check/
        typing/ authority/ recurrence/ certificate/
                  independent logical proof and opaque certificate
    mount/
        address/ arrangement/ witness/
                  physical specialization and opaque mounted witness
    internal/
        architecture/ executable namespace and import-direction laws

analysis/schema/rule/
    relcompile/   existing rule.Program declarations to relation/schema
    relbindgen/   thin typed semantic-binding generation only

analysis/engine/relation/
    state/
        column/ index/ transaction/
                  immutable guarded columns, arrangements, prepared ascent
    operator/
        unary/ join/ group/ merge/
                  physical relational operators
    apply/        authenticated semantic invocation and proposal validation
    publish/      the sole atomic relation publication door
    solve/
        dependency/ fixpoint/
                  delta wake-up, SCC/WTO iteration and convergence
    runtime/      sole composition entry point and snapshot publication

internal/relationoracle/
                  deliberately small reference evaluator used only by tests

analysis/snapshot/
                  existing immutable committed store
```

### 3.1 Import direction

```text
identity, lattice, schema vocabulary
                |
                v
 schema/model -> relation/semantic -> schema/algebra -> schema/plan
                                      |                 |
                                      +-------> check <-+
                                                   |
                                                   v
                                                 mount
                v
  engine/relation/state
       +---------+----------+
       v         v          v
   operator     apply     publish
       +---------+----------+
                v
  engine/relation/solve
                v
 engine/relation/runtime
                v
 snapshot -> result / diagnostics / query / JIT export
```

`relcompile` imports the existing declaration vocabulary and produces
`schema/plan`; nothing below `relcompile` imports rule declarations.

No package under `analysis/relation` imports `analysis/engine`,
`analysis/result`, or a domain implementation. Physical engine packages do not
import domain implementations either; they redeem authenticated semantic
bindings through `relation/semantic`.

The architecture law protects altitude and rejected dependencies, not an exact
package inventory. A worker may create a focused child package inside its owned
subsystem when implementation evidence calls for it; the child inherits that
subsystem's altitude. New cross-subsystem edges require an architect-reviewed
law change. Aggregation roots remain free of production Go files.

## 4. Canonical data model

A logical relation is a typed mapping:

```text
(stable key, decision scope) -> (presence, semantic cell, lineage reference)
```

The components have different laws:

- The semantic cell belongs to an owner lattice and ascends by owner `Join` or
  certified widening.
- Decision scope uses entailment and conjunction. Scope filtering happens
  before semantic application; semantic operations never inspect masks.
- Lineage is an idempotent support/proof sidecar. It cannot affect semantic
  convergence.
- Delta records that a cell version ascended. It is not lattice subtraction and
  is not part of logical identity.

Presence is not encoded as a domain lattice value. At minimum it distinguishes:

```text
Present
ProvenAbsent
UnprovenMissing
AuthenticatedOpaque
Refused(reason identity)
```

`NoCandidate` and `NoSelection` remain distinct semantic outcomes. Neither is
fabricated as Bottom or Unknown.

Logical keys are owner-issued, content-addressed identities. Physical ordinals,
hash slots, trie positions, MTBDD variable order, worker addresses, epochs, and
scratch indexes are mount/runtime data only.

## 5. Logical expression language

Version one has this closed logical grammar:

```text
Input       name a sealed base or derived relation
Select      retain rows by sealed scope entailment
Project     rename or compute schema-defined columns and keys
Join        one oriented equijoin over logical column vectors
Merge       combine alternative derivations using each column TypeID's algebra
Group       collect or reduce rows under a declared key and delivery contract
Complete    close a relation against an authenticated denominator
Apply       invoke one authenticated typed semantic operation
Publish     propose rows to one declared destination relation
```

Positive recursion is represented by dependency edges and SCCs in the
`ExecutionSchema`, not by an arbitrary expression node. WTO and widening are
execution policies attached to certified recurrence heads.

Lookup, scan, hash join, merge join, ordering, materialization, MTBDD layout,
and exchange are physical choices. They are not logical operators.

Parent, occurrence, address, key-vector, activation, route, and correspondence
roles are relation data, not operator variants. A correspondence relation is an
ordinary `Input` joined by the same equijoin. Keys and index choice are physical
mount concerns. Equality and tag predicates normalize to joins over declared
fact relations; arbitrary semantic predicates must first produce a fact through
`Apply`.

True anti-join/negation is not in version one. Existing absence-shaped rules
must compile through `Complete` and explicit presence. If a future census proves
real negation necessary, it is added only as a stratified operator with no
negative recursion.

### 5.1 Analysis queries

Queries, diagnostics, and inference rules use the same expression language.
An inference plan ends in ascending `Publish`. A read-only analysis query ends
in a query result relation consumed from the converged snapshot. Query planning,
indexes, scope, lineage, and dependency tracking are therefore shared rather
than rebuilt by query-specific access code.

This separation leaves room for future certified rewrites:

- predicate and projection pushdown;
- index and arrangement selection;
- join reordering where outcome, scope, and refusal equivalence are proven;
- demand-driven or magic-set rewrites for sparse queries;
- semi-naive and differential physical strategies;
- hash, trie, columnar, or MTBDD relation backends.

No such optimization is needed for the first cut.

## 6. The semantic ABI

Relational operators cannot implement domain mathematics. Conversely, domain
mathematics must not receive storage or scheduling authority. `Apply` is the
explicit boundary.

A semantic operation is named by an owner-issued identity and has a sealed
signature containing:

- ordered typed input columns, exact denominators, presence contracts, and
  scalar/bounded-span/complete-span delivery contracts;
- ordered typed output columns and one output denominator;
- cardinality: exactly one, optional, or denominator-bounded many;
- one closed outcome vocabulary;
- the declaring owner and exact logical schema fence.

At runtime the generic engine carries schema-, mount-, and generation-fenced
cell tokens, not `any` payloads or reflection. Every input cell carries its own
relation-safe denominator witness. The relational layer conjoins decision
scope before `Apply`, so the frame and all its cells carry one opaque invocation
scope and the worker never sees masks. A generated binding decodes those
tokens to concrete domain values and encodes proposals back. Its factory checks
the sealed signature and creates a solve-local worker with reusable scratch.

The operation receives only a typed immutable frame of borrowed scalar or span
slots. It returns an outcome and denominator-bounded output rows or update
proposals through a preallocated proposal buffer. It cannot:

- read a relation that is absent from its signature;
- inspect engine state, points, ports, stages, masks, tickets, or schedules;
- mint logical identities;
- mutate relation state or publish directly;
- choose a destination that is absent from its output schema;
- hide an unbounded callback-driven traversal.

Lattice algebra is canonical per declared `TypeID`, resolved independently of
operation bindings. `Join` and `LessOrEqual` prove ascent; `Widen` is invoked
only at certified recurrence heads. The generic state layer validates every
proposal, proves it is an ascent, and
commits all rows from one invocation atomically.

This boundary covers the irreducible cases honestly:

- `RawGet` uses relational joins for its dependency graph and typed expansion/
  reduction operations for receiver routes, heap payloads, Pack sources, and
  Value results.
- `RawSet` returns authenticated heap update proposals; the generic state layer
  performs the atomic ascending commit.
- Query folds use grouped typed operations while their population, filtering,
  joining, scheduling, and result publication remain generic.
- Activation and transport use sealed identities and relational routes; domain
  code cannot construct topology.

The ABI is closed by signature and binding-conformance laws, not by hard-coding
every domain operation into the engine. Scalar/span shape comes from input
delivery; expansion comes from output cardinality; an update is publication to
an existing authenticated row. These are derived facts, not capability labels.
Every admitted binding must prove monotonicity globally. Arbitrary function
values, reflection and untyped payload boxes are forbidden.

### 6.1 Irreducible generated code

Go requires a boundary between heterogeneous payload types and generic runtime
storage. Generation may remain only for:

1. a thin typed semantic-operation binding; and
2. a thin typed owner-column publisher.

Generated code must contain no relational reads, joins, routing, scheduling,
ticketing, outcome settlement, form selection, or publication choreography.

## 7. Complete sealed artifacts

`ExecutionSchema` is immutable and contains:

- one exact schema identity and canonical relation, column, key and scope
  registries;
- owner-issued correspondence, address, parent and occurrence relations;
- expression DAGs for rules, queries, observations, diagnostics and seeds;
- semantic-operation signatures, per-TypeID algebra requirements, delivery
  contracts, denominators and output destinations;
- dependency graph, SCCs, WTO constraints, and widening heads;
- lineage policy and publication schema;
- generic relation plans that lower every candidate, read, write, transport,
  activation and route declaration without retaining those roles as engine
  forms.

Queries and diagnostics are canonical relations in that same schema, including
the equivalents of `query_family`, `query_site`, `query_answer`,
`observation_site`, `observation_producer`, `value_observation`,
`send_decision`, and `diagnostic_finding`. Rich result families may declare
typed child relations. `Complete` materializes explicit denominator rows and
presence; consumers never reconstruct missing answers after solve.

It contains no local ordinal, runtime pointer, storage handle, Go function,
index implementation, or backend-specific ordering.

The independent checker returns an opaque `Certificate`. Mount accepts only a
certificate, never a raw schema. Runtime accepts only a mounted certificate,
never declarations or an unchecked plan.

Conceptual public surface:

```go
relcompile.Compile(declarations) (plan.ExecutionSchema, error)
check.Check(plan.ExecutionSchema) (check.Certificate, error)
mount.Specialize(check.Certificate, mount.Inventory) (mount.Mounted, error)
runtime.Solve(mount.Mounted, runtime.Inputs) (snapshot.Snapshot, error)
```

The concrete Go APIs may use explicit result types rather than `error`, but the
authority flow may not change.

## 8. Physical execution

The mounted plan derives required arrangements from logical access patterns.
Version one uses deterministic choices. Multiple arrangements may share the
same logical relation; none changes its identity.

The solve-local state reuses proven mathematical kernels where they are
genuinely generic: immutable decision diagrams, typed terminal interning,
guard/support algebra and scheduling. It does not reuse `carrier`,
`factbinding`, `demand`, `equation`, `linkexecutionplan`, execution catalogs,
or their transaction/change vocabulary; those are the old protocol under
generic-looking names.

`state/column` owns immutable versioned guarded columns and keeps the existing
MTBDD/FDD compression behind its API. Scope, presence, semantic value and
lineage remain separate; a stored domain default is not sparse absence.
`state/index` owns only arrangements that are extensionally equivalent to a
column scan. `state/transaction` validates and prepares a complete immutable
candidate plus its deterministic delta. `publish` alone makes a prepared
candidate live. There is no second commit or mutable write door.

Evaluation is semi-naive:

```text
committed cell ascent
        -> relation delta
        -> dependent plan nodes
        -> semantic/relational output proposals
        -> authenticated atomic ascent
        -> repeat until no delta remains
```

Work is driven by changed rows, not by rescanning every rule. Within a solve,
semantic state only ascends. Implementations may mutate private scratch and
unpublished buffers; immutability begins at the committed version boundary.

## 9. Soundness gates

### 9.1 Relation schema

- stable identity is independent of declaration and physical order;
- keys and columns are nominally typed;
- owner, generation, scope and correspondence fences are total;
- malformed or duplicate declarations refuse deterministically;
- the four algebras cannot be substituted for one another.

### 9.2 Semantic ABI

- every binding exactly matches its sealed signature;
- outputs are range-restricted to declared identities and denominators;
- scalar and update operations are monotone;
- expansion is finite under its declared denominator;
- refusal, opacity, absence and empty selection remain distinct;
- foreign schema, mount, owner, generation, denominator or signature refuses.

### 9.3 Checker

- every expression input is defined and type-compatible;
- every output has one publication authority;
- every `Complete` has an authenticated denominator;
- every semantic operation is available with the exact signature;
- every SCC is positive and monotone, with widening only at certified heads;
- scope filtering precedes `Apply` and publication;
- lineage cannot influence dependency or convergence;
- mutation tests exercise every individual refusal rule.

### 9.4 Mount

- logical key to local address round-trips;
- physical reorder leaves logical and certificate identities unchanged;
- lookup/range/correspondence arrangements are extensionally equal to scan;
- every required route, semantic signature and TypeID algebra binds exactly
  once;
- stale and foreign addresses refuse.

### 9.5 Evaluator and solver

- every physical operator is differential-tested against
  `internal/relationoracle`;
- full recomputation equals delta evaluation;
- legal join reorderings preserve values, scope, refusal and lineage;
- publication is atomic and has one write door;
- WTO scheduling and widening preserve the certified fixpoint contract;
- lineage collection does not change values or convergence;
- deterministic inputs produce deterministic snapshots.

Focused tests and fixtures must execute in under five seconds, excluding Go
compilation. A longer fixture is reduced to its shortest converging witness
before it enters an iteration loop.

## 10. Hostile closure corpus

Implementation does not proceed to production integration until the compiler,
checker, reference evaluator, and physical evaluator cover:

- ordinary exact and seed families;
- selected and correspondence reads;
- summary and complete vectors with absent cells;
- routed and transformed publications;
- activation, transport, and structural publication;
- placement capture, containment, formal, publication escape and suspension;
- typed query folds and selected query-site closure;
- diagnostic observations and decision scope;
- `RawGet`'s dependent six-read chain and typed route expansion;
- `RawSet`'s dependent chain and atomic heap update proposal.

No specimen may require an operator switch arm, domain import in the engine, or
an undeclared runtime callback.

## 11. Implementation and fan-out sequence

The architect owns shared contracts. Worker lanes own disjoint child packages.
No worker changes a shared interface while another layer is implementing it;
proposed contract changes return to the architect with a concrete hostile
specimen and a failed law.

Codex owns the generic machine (`analysis/relation/**`,
`analysis/engine/relation/**`, and `internal/relationoracle/**`). The second
orchestrator owns declaration adaptation, domain bindings, consumers, parity,
and final deletion. Until that orchestrator rejoins, Codex does not absorb those
paths casually; it records ready briefs and continues the generic machine.
Each orchestrator may use five workers, but slots remain idle at contract and
composition choke points. Parallelism follows ownership independence, not a
target worker count.

### Wave 0: freeze the contracts — architect-led, disjoint implementation

Owner: architect.

Create only:

- package skeletons and import laws;
- `relation/schema/model` identities, relation/column types;
- `relation/schema/algebra` expression nodes;
- `relation/schema/plan` immutable plan and dependency records;
- `relation/semantic` signatures, delivery contracts and outcomes;
- opaque `ExecutionSchema`, `Certificate`, `Mounted`, and runtime input/output
  boundaries;
- the hostile-corpus table and mutation-test inventory.

Gate W0:

- packages compile independently;
- dependency laws pass;
- no old execution package is imported;
- no domain-specific type or operation appears in the generic vocabulary;
- the interfaces are frozen for Waves 1 and 2.

The architect freezes the decisions serially. Once a decision is journaled,
workers may implement disjoint model, algebra, semantic, plan, and hostile-law
packages in parallel. They may not redesign one another's ABI; a discovered gap
returns to the architect with a hostile specimen and failed law.

### Wave 1: prove the logical layer — three parallel lanes

Starts after W0.

Lane 1A — logical checker

- owns `analysis/relation/check` only;
- implements typing, authority, denominator, scope and SCC checks;
- builds hostile mutation tests;
- must not import `relcompile`.

Lane 1B — declaration compiler and census

- owns `analysis/schema/rule/relcompile` only;
- translates existing `rule.Program`, axis/member declarations, queries, seeds,
  observations and diagnostics;
- produces the machine-readable coverage matrix;
- reports coupling findings rather than compensating for them.
- authors complete `RawGet` and `RawSet` plans and signatures: their existing
  declarations omit receiver-route and raw-access expansion, so translation
  alone is insufficient.

Lane 1C — semantic ABI bindings

- owns `analysis/relation/semantic` implementation tests and
  `analysis/schema/rule/relbindgen`;
- proves typed signature matching, outcome closure, finite expansion and update
  proposal laws;
- implements specimen bindings, not production family choreography.

Gate W1:

- every current and parked declaration compiles;
- hostile `RawGet`/`RawSet` plans compile with explicit typed inputs, outputs,
  denominators and atomic proposals;
- the independent checker accepts the complete valid census and rejects every
  nearest mutation;
- no escape-hatch operator or arbitrary callback exists.

If W1 fails, the architecture returns to Wave 0. Physical implementation does
not begin around a logical gap.

### Wave 2: prove physical foundations — up to five parallel lanes

Starts after W1 freezes the checked artifact format.

Lane 2A — mount specialization

- owns `analysis/relation/mount`;
- binds stable identities to local addresses and derives arrangements;
- uses fake inventories before any production integration.

Lane 2B — solve-local relation state

- owns `analysis/engine/relation/state`;
- establishes the only read/propose/commit API;
- reuses only generic diagram/terminal/guard/support kernels behind its API;
- does not import carrier, factbinding, demand, equation or linkexecutionplan;
- proves atomic ascent, delta production and generation fencing.

Lane 2C — reference evaluator

- owns `internal/relationoracle`;
- implements a deliberately simple full-recomputation evaluator;
- has no physical indexes, WTO optimization, domain imports, or production
  consumers.

Gate W2:

- mounted addressing and arrangement equivalence laws pass;
- state transactions and deltas pass independently;
- the reference evaluator runs the hostile synthetic corpus;
- none of these packages imports the old form runtime.

### Wave 3: implement execution — up to five parallel lanes

Starts after the mount, state, and reference APIs are frozen.

Lane 3A — relational kernels

- owns `analysis/engine/relation/operator`;
- implements Input, Select, Project, Join, Merge, Group and Complete;
- differentially tests every kernel against the reference evaluator.

Lane 3B — semantic application

- owns `analysis/engine/relation/apply`;
- invokes authenticated typed bindings;
- validates bounded outputs and submits proposals through `publish` only;
- proves `RawGet`, `RawSet`, query-fold, refusal and atomic-update laws.

Lane 3C — publication

- owns `analysis/engine/relation/publish`;
- provides the sole atomic write door over `state`;
- proves destination authority, ascent, generation, scope and lineage laws.

Lane 3D — solver

- owns `analysis/engine/relation/solve`;
- implements dependency wake-up, semi-naive deltas, SCC/WTO iteration and
  certified widening;
- initially uses fake evaluators and then the frozen operator/apply/publish APIs.

The four lanes own disjoint packages. The architect owns their shared
interfaces and no lane edits another lane's package.

Gate W3:

- physical and reference snapshots agree over the hostile corpus;
- full recomputation equals delta execution;
- convergence, widening, scope and lineage laws pass;
- no rule-family or domain switch exists in engine code.

### Wave 4: end-to-end replacement — staged composition then parallel consumers

Starts after W3.

Lane 4A — runtime composition

- owns `analysis/engine/relation/runtime`;
- composes checked schema, mount, semantic bindings, state, evaluator and solver;
- admits every rule, seed, activation, query and observation.

After Lane 4A freezes the candidate runtime API, the following two lanes may
start in parallel. They do not edit the runtime constructor.

Lane 4B — snapshot and read-only consumers

- owns new snapshot publication integration and later `analysis/result` /
  diagnostic changes;
- publishes through existing `analysis/snapshot`;
- removes engine dependencies from snapshot read paths where possible.

Lane 4C — external parity harness

- owns test tooling outside production packages;
- compares separately built baseline and replacement binaries by stable key,
  scope, value, outcome, diagnostic and canonical lineage;
- never links both runtimes into one process.

Gate W4:

- all deterministic fixtures and the settled corpus agree externally;
- query and diagnostic consumers read the replacement snapshot;
- all production families bind through the semantic ABI;
- the replacement constructor is complete but not yet selected by production.

### Wave 5: atomic production cut — coordinated, not fanned out

Owner: architect with deletion lanes operating from a fixed manifest.

One landing:

1. rewires the analyzer constructor to `engine/relation/runtime`;
2. deletes the old form execution package and execution catalog;
3. deletes generated family workers and installer-only APIs;
4. deletes HotRule/HotOwner/BindHot protocol and obsolete registrations;
5. deletes rule emit/render/install code and obsolete generated descriptors;
6. deletes old runtime assembly, lowering, binding, issuance and seal paths that
   became unreachable;
7. replaces implementation-restatement tests with logical, certificate,
   physical, or external-parity laws;
8. runs typed residue and import-direction scans.

Deletion may be assigned by disjoint directory, but the cut is integrated and
verified as one atomic replacement. No intermediate commit is a supported
production architecture.

## 12. What remains and what disappears

Retain:

- domain lattice algebras and semantic judgments;
- declarative `rule.Program` authoring surface, simplified when fields become
  derivable;
- identity, lattice and immutable snapshot foundations;
- generic decision-diagram, terminal, support/guard, scheduling and fixpoint
  primitives that pass the new package boundary;
- thin typed semantic and column bindings.

Delete after the atomic cut:

- `analysis/engine/execution` form classification and workers;
- `analysis/engine/internal/executioncatalog`;
- generated family executors and installer choreography;
- HotRule, HotOwner and BindHot protocol;
- duplicated family/member/rule registries derivable from declarations;
- emitter/render/installer machinery whose product was executable Go protocol;
- repeated runtime validation, topology reconstruction and address inference;
- query/diagnostic access paths that reconstruct relations outside the canonical
  snapshot.

Do not promise deletion of low-level mathematical kernels merely because they
currently live beneath the old engine. They move or remain only after their
ownership is proven generic.

## 13. Stop conditions

Stop and return to design if any implementation requires:

- a new per-family runtime form;
- an engine import of a domain implementation;
- a semantic operation that reads or publishes outside its sealed frame;
- a second relation, snapshot, outcome, scope, lineage or identity
  representation;
- runtime validation of facts already certified at seal;
- compensation, fallback, inferred Unknown, or compatibility behavior;
- a local ordinal used as a stable identity;
- production dual execution;
- a later layer importing an unfinished lower layer.

The design succeeds when adding a new analysis rule or read-only query normally
requires only declarations, typed semantic operations, and semantic laws—zero
engine choreography.
