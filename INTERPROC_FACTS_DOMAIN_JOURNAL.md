# Interproc Facts And Checker Domain Design Journal

## 2026-05-19 Design Consolidation Checkpoint

This document records the design model before the next implementation pass. It is
not an implementation plan for incremental bridges. The intended correction is a
flash migration: design the final shape, migrate directly to it, delete the old
helper clusters, and do not leave compatibility wrappers or fallback layers in
the production checker.

## Goal

The checker should read as one abstract interpreter over a product domain.

The current implementation is already powerful:

- it tracks flow-sensitive path facts,
- it narrows through guards and assertions,
- it propagates table and container mutations,
- it infers local and interprocedural function facts,
- it correlates value/error return slots,
- it handles soft annotation evidence,
- it uses Salsa-style query inputs for function-result invalidation.

The design problem is that these capabilities are encoded by many local helper
clusters. That makes the system hard to reason about even when the behavior is
mostly correct. Helpers such as `typeRefinesTableKeyByTruthiness` are not just
helpers; they are domain laws living in the wrong place.

The target is a smaller, clearer checker where each law has exactly one owner.

## Non-Negotiable Constraints

- No production transition layer.
- No legacy mirror fact channels.
- No raising iteration caps to hide non-convergence.
- No external application-code edits as part of go-lua design correction.
- No weakening soundness by making `any` assignable to concrete contracts.
- No helper-specific exceptions for external lint targets.
- No pools as the first answer to performance; use structural ownership and
  caching first.
- Every final abstraction must have law tests and paired positive/negative
  behavioral tests.

## Current Mental Model

The checker is a multi-phase abstract interpreter:

1. Scope and CFG construction establish symbols, lexical parents, control-flow
   points, and function graph identity.
2. Declared-phase synthesis extracts initial types, table literal shapes,
   function literal signatures, and call/effect evidence.
3. `flowbuild` lowers AST and synthesis facts into flow inputs:
   declarations, assignments, table/index mutations, call effects, branch
   predicates, return constraints, numeric constraints, aliases, and termination
   facts.
4. `types/flow` solves a forward dataflow problem over canonical SSA path keys.
   The persistent solved state is currently split across value maps, conditions,
   numeric states, alias maps, field overlays, and local caches.
5. Narrowing queries are demand-side interpretation: read solved facts at a
   point, apply propagated constraints, and answer refined path/type questions.
6. Return inference and local function SCC solving use the flow result plus
   interprocedural snapshots to infer return vectors, parameter hints, function
   facts, captured fields, and captured container mutations.
7. The interprocedural store combines same-iteration deltas with a precise join
   and combines recursive fixpoint boundaries with widening.
8. Salsa-style snapshot inputs connect function-result queries to exact
   interproc facts, refinements, and constructor-field snapshots.

This is the right high-level shape. The weakness is that the product domain is
not first-class enough in code.

## Clean Abstract Interpreter Target

The final checker should be explainable as:

```text
AbstractInterpreter = CFG + AbstractState + Transfer + Join + Widen + Query
```

Where:

- `CFG` owns control-flow order and dominance.
- `AbstractState` owns the full product of memory, value, numeric, relation,
  effect, and termination facts.
- `Transfer` is the only way statements and expressions change state.
- `Join` is the only way same-phase branch/predecessor evidence combines.
- `Widen` is the only way recursive or interprocedural cycles are forced to
  converge.
- `Query` reads solved state without inventing another analysis path.

This is the mental model the code should expose. If a rule cannot be explained
as one of these operations, it is either orchestration or a design smell.

The current checker has the right ingredients but not the right ownership. It
has preflow inference, flow solving, narrowing queries, return SCC inference,
overlay refresh, mutation replay, and interproc widening. Those should become
clients of the same abstract-state and domain APIs. They should not remain
separate places where local helpers decide what refinement means.

## State-Of-The-Art Bar

The target is not just cleaner Go packages. The target is a modern static
analysis engine with explicit theory:

- monotone abstract domains with named `Normalize`, `Leq`, `Join`, `Meet`, and
  `Widen` operations;
- transfer functions over a product state instead of helper-specific rewrites;
- a first-class memory model for paths, fields, indexes, aliases, mutations,
  row tails, and dominance;
- relational facts for tuple slots and path correlations instead of hardcoded
  error-return branches;
- principled distinction between `unknown`, `any`, `nil`, absent fields, soft
  evidence, hard evidence, table top, and open row tails;
- explicit widening at recursive boundaries and optional narrowing only after a
  post-fixpoint is reached;
- deterministic canonicalization and equality, never equality-time repair;
- cache keys derived from immutable inputs and domain snapshots, not incidental
  phase call order;
- paired positive/negative law tests so the implementation cannot get faster by
  becoming less sound.

Anything less will keep producing local helper patches. The migration should
make the checker look like the theory it is implementing.

## Core Moral Model

The checker should be taught and reasoned about with one sentence:

```text
Evidence is produced by transfer, combined by domains, stabilized by widening,
and observed by queries.
```

That sentence is the guardrail.

- Extraction does not decide lattice policy. It only converts source syntax into
  typed evidence and transfer instructions.
- Transfer does not decide cross-iteration convergence. It only updates the
  current abstract state.
- Domains do not inspect AST. They only combine abstract values and facts.
- Widening does not recover precision. It only guarantees convergence.
- Queries do not produce new facts. They only read the solved state and apply
  already-recorded constraints.
- Interprocedural producers do not mutate old state. They emit deltas.

If a function violates one of these rules, it is a design smell even if the
behavioral test passes.

## Canonical Dataflow Contract

The final dataflow should have explicit boundary objects.

```text
Source
  -> GraphBundle
  -> CheckerIR
  -> TransferProgram
  -> AbstractState
  -> QueryView
  -> FunctionResult
  -> InterprocDelta
  -> FactsDomain
  -> SnapshotInputs
```

### GraphBundle

Owns:

- AST function body,
- CFG,
- symbol table,
- parent scope identity,
- dominance/post-dominance indexes,
- local function indexes,
- parameter-use summaries.

It is immutable after construction. Anything expensive and graph-derived should
be cached here or through a Salsa query keyed by graph identity.

### CheckerIR

Owns the normalized checker program:

- declarations,
- assignments,
- branch predicates,
- calls,
- returns,
- table constructors,
- field/index writes,
- mutation effects,
- termination effects.

It should be AST-free except for source spans and stable graph references. This
is where the checker stops being syntax-driven and becomes analysis-driven.

### TransferProgram

Owns executable transfer instructions over `AbstractState`.

Examples:

```text
Assign(Location, ValueExpr)
Assume(Condition)
Mutate(Mutation)
Call(CallSite)
Return(ReturnTuple)
Terminate(Reason)
```

Every statement-level fact should enter the solver through an instruction like
this. A table insert, captured mutation replay, field assignment, and dynamic
index write should not each invent their own path rules.

### AbstractState

Owns the whole product:

```text
AbstractState =
  MemoryState
  x ValueFacts
  x NumericFacts
  x ShapeFacts
  x RelationFacts
  x EffectFacts
  x TerminationFacts
```

This must be the persistent state of the intraprocedural solver. Query-time
`ProductDomain` construction should be replaced by reading this product, or by
creating a cheap view over it. The state product is the source of truth.

### QueryView

Owns read-only answers:

- type at point,
- narrowed path type,
- field/index presence,
- tuple relation at call site,
- constant/numeric facts,
- reachability.

It cannot write facts. It cannot perform fresh synthesis that changes the
answer independently from `AbstractState`.

### InterprocDelta

Owns facts emitted by a completed function analysis:

- function fact,
- parameter evidence,
- literal signatures,
- captured field mutations,
- captured container/table mutations,
- constructor fields,
- relation summaries.

The delta is immutable. The store combines it through `FactsDomain` only.

## Evidence Lifecycle

Every fact in the checker should have a visible lifecycle:

```text
Observed -> Located -> Qualified -> Transferred -> Joined -> Widened -> Queried -> Published
```

### Observed

Evidence starts from one of a small number of sources:

- source annotation,
- literal syntax,
- assignment,
- guard/predicate/assertion,
- call argument,
- call return,
- effect spec,
- table/container mutation,
- imported manifest,
- previous interproc snapshot.

Observation records provenance. It does not decide final authority.

### Located

Every observation must attach to a location:

- symbol,
- field path,
- index path,
- tuple slot,
- function graph,
- parent scope,
- call site,
- return site.

Location must be canonical before the evidence enters transfer. This prevents
one helper using AST paths while another uses SSA path keys.

### Qualified

The evidence is tagged with its authority:

```text
explicit annotation > hard proof > body obligation > call observation >
soft annotation > unresolved evidence
```

`any` is not "very strong evidence." It is dynamic top. `unknown` is not "safe
to ignore." It is unresolved evidence. These two facts must remain distinct in
every domain.

### Transferred

Transfer applies evidence to the current `AbstractState`.

Examples:

- assignment writes memory/value facts,
- guard writes relation and shape facts,
- call writes return tuple and effect facts,
- table insert writes a mutation fact,
- error-return check reads a tuple relation and narrows linked slots.

Transfer does not call interproc merge functions. Transfer does not widen.

### Joined

Control-flow joins combine same-phase predecessor states through domain `Join`.
This is where branch evidence meets.

Branch joins must preserve runtime alternatives. For Lua, `x or y` and `x and y`
return actual operand values, so the value domain cannot prune a live branch just
because the other branch is more precise.

### Widened

Widening is allowed only at named recursive boundaries:

- loop fixpoint,
- local function SCC,
- interprocedural fixpoint,
- recursive type/shape growth boundary.

Widening must be visible in code. If a helper "prefers" one side to force
stability, it is a widening rule and belongs to the domain that owns that
cycle.

### Queried

Queries produce read-only views:

- type at point,
- narrowed path,
- field/index evidence,
- relation state,
- effect summary.

Queries cannot publish facts. If a query has to synthesize new evidence to
answer correctly, that evidence belongs in transfer or in a cached derived input
computed before solving.

### Published

Only completed function analysis publishes interproc deltas. Publication is a
data move:

```text
FunctionResult -> InterprocDelta -> FactsDomain.Join/Widen -> SnapshotInputs
```

Publication is not another inference pass.

## Required Domain API Shape

Every domain should expose the same conceptual operations even if Go uses
concrete types instead of generics everywhere.

```go
type Domain[T any] interface {
    Normalize(T) T
    Leq(a, b T) bool
    Join(a, b T) T
    Meet(a, b T) T
    Widen(prev, next T) T
}
```

Transfer is separate:

```go
type Transfer[I any, S any] interface {
    Apply(input I, state S) S
}
```

Query is separate:

```go
type Query[S any, Q any, A any] interface {
    Answer(state S, question Q) A
}
```

This separation is important:

- `Join` and `Widen` do not inspect AST.
- `Transfer` does not know interproc storage.
- `Query` does not mutate state.
- `Normalize` is explicit and not hidden in equality.

## Dataflow Walkthroughs

### Guarded Field To Call Argument

Pattern:

```lua
if options.model then
    provider.open(options.model)
end
```

Correct dataflow:

1. `options.model` is observed as a field read.
2. The guard transfers a truthy relation for `Location(options, "model")`.
3. The call argument query reads that relation and answers `NonNil(modelType)`.
4. Parameter evidence records a call observation for the callee.
5. If the callee body requires `string`, body obligation and call observation
   combine in `ParameterEvidenceDomain`.

Wrong shape:

- special-case `options.model` in call checking,
- make all truthy fields strings,
- accept `any` as string.

### Table Insert To Later Iteration

Pattern:

```lua
table.insert(state.items, value)
for _, item in ipairs(state.items) do ... end
```

Correct dataflow:

1. `state.items` resolves to one memory location.
2. `table.insert` transfers a `MutationTableElement` to that location.
3. Memory join preserves the element fact at the exact child path.
4. `ipairs` queries the array element evidence from memory.

Wrong shape:

- replay captured table insert through generic container mutation,
- let parent table literal shape override explicit child-path evidence,
- infer element type from the loop variable without memory provenance.

### Error Return Correlation

Pattern:

```lua
local value, err = f()
test.is_nil(err)
value.field
```

Correct dataflow:

1. `f()` returns a tuple with a relation summary.
2. Assignment binds tuple slots to locations.
3. `test.is_nil(err)` transfers a relation constraint on the error slot.
4. Relation query narrows the linked value slot.
5. Field access reads the narrowed value slot.

Wrong shape:

- hardcode `test.is_nil` as a value-slot refinement,
- assume every two-return function is `(value, err)`,
- drop tuple relation when a wrapper forwards returns.

### Unknown External Payload

Pattern:

```lua
local payload = json.decode(raw)
needs_string(payload.name)
```

Correct dataflow:

1. `json.decode` returns dynamic/unresolved data.
2. `payload.name` is unresolved or `any` depending on API contract.
3. Passing it to `string` must fail unless a guard, schema, cast, or contract
   proves it.

Wrong shape:

- treat unknown external fields as strings because most callers expect strings,
- let table shape contextualization silently rewrite explicit `any`,
- clear global lint by broadening assignability.

## Inference Model

Inference is not a separate magical subsystem. It is the process of solving for
unknown slots in the product domain under the evidence produced by transfer.

The final model should distinguish these inference layers:

### Local Value Inference

Scope:

- local variables,
- field/index reads,
- table literals,
- expression results,
- branch-local values,
- loop-carried values.

Authority:

```text
AbstractState.ValueFacts + MemoryState + RelationFacts
```

Rules:

- local value inference reads declared types, transfer assignments, and
  constraints;
- it never writes interprocedural facts directly;
- it must preserve the distinction between `unknown` and `any`;
- table literal contextualization is a transfer/type-domain operation, not a
  one-off hook;
- logical `and`/`or` inference must preserve actual Lua branch values.

### Parameter Inference

Scope:

- call-site argument observations,
- body-derived obligations,
- source annotations,
- soft annotations,
- current function facts,
- function literal expectations.

Authority:

```text
ParameterEvidenceDomain
```

Rules:

- call-site observations are evidence, not contracts;
- body obligations are contracts only when the function body proves it requires
  that shape;
- explicit source annotations dominate inferred hints;
- soft annotations refine only when hard evidence proves the refinement;
- recursive parameter evidence must join/widen through the parameter domain.

There should be no separate ad hoc policy for "param hints" versus "function
fact params". Both are parameter evidence with different provenance and merge
mode.

### Return Inference

Scope:

- return statements,
- tuple/multivalue expansion,
- nil padding,
- recursive return vectors,
- summary and narrow summary slots,
- wrapper forwarding.

Authority:

```text
ReturnSummaryDomain
RelationDomain
```

Rules:

- return arity is part of the tuple domain;
- `unknown` return evidence is unresolved runtime behavior, not bottom;
- recursive return vectors widen only at the SCC/fixpoint boundary;
- relation facts such as `(value, err)` attach to return tuples explicitly;
- wrapper forwarding propagates tuple and relation facts together.

### Function Type Inference

Scope:

- local function literals,
- method receiver `self`,
- higher-order callbacks,
- literal signatures,
- exported functions,
- imported module functions.

Authority:

```text
FunctionFactDomain
ParameterEvidenceDomain
ReturnSummaryDomain
RelationDomain
```

Rules:

- function type inference is a product of parameter evidence, return summary,
  and relation/effect summaries;
- a same-body function fact may seed analysis only through non-narrowing domain
  merge;
- higher-order signatures must use variance-aware merge rules;
- literal signatures are facts in the interproc product, not a second function
  authority.

## Phase Responsibility Table

| Phase | May Create | May Combine | May Widen | May Query | Forbidden |
|---|---|---|---|---|---|
| Scope/CFG | graph identity, symbols | no type facts | no | no | type merge policy |
| Extract/IR | transfer instructions | no domain joins | no | declared-only queries | fixpoint repair |
| Flow solve | abstract state updates | domain joins at CFG joins | loop-local widening only if owned by flow domain | internal state reads | interproc fact writes |
| Narrow/query | read-only answers | no persistent joins | no | yes | producing facts |
| Return SCC | local return/param deltas | local domain joins | SCC widening through domain only | solved flow state | AST-specific merge laws |
| Interproc store | immutable deltas | `FactsDomain.Join` | `FactsDomain.Widen` | snapshot reads | producer-specific callbacks |
| Salsa | dependencies/cache | no semantic joins | no | query execution | hidden state mutation |

## Foundational Diagnosis

The checker has accumulated strong behavior before it acquired the right
vocabulary.

Current scattered concepts:

- value-type joins,
- return-slot joins,
- function-param fact joins,
- param-hint joins,
- table-top absorption,
- soft-placeholder replacement,
- open-record row-tail merging,
- recursive structural-growth cutoffs,
- truthiness refinements,
- error-return tuple correlations,
- captured table/container mutation replay,
- path identity and alias identity,
- body-derived parameter contracts,
- call-site observations,
- signature projection to body use.

These concepts are real. The problem is not that they exist. The problem is that
they appear as local helpers in `returns`, `paramhints`, `flow`, `synth`, and
`typ`, with overlapping responsibilities.

That creates the "guacamole" feeling: behavior is strong, but the mental model
is not visible at the package boundary.

## Canonical Product Domain

The final checker should have these explicit products.

### Value Domain

Owns pure type operations that are independent of checker phase:

- `NormalizeType`
- `JoinValue`
- `JoinReturnSlot`
- `Meet`
- `WidenShape`
- `Refines`
- `TruthinessRefinement`
- `Nilability`
- `SoftEvidence`
- open/closed record row-tail policy
- map/array/table-top classification

Candidate home:

```text
types/typ/domain
```

or, if it needs checker-only evidence policy:

```text
compiler/check/domain/value
```

Rule: domain-level predicates such as "candidate refines baseline by removing a
falsy table key" cannot live in `compiler/check/returns/join.go`.

### Memory And Path Domain

Owns the question "what program location does this fact describe?"

It must unify:

- `constraint.Path`
- CFG symbol/version identity
- SSA path keys
- field/index segments
- aliases
- dynamic index writes
- table mutator paths
- captured mutation paths
- field overlays

Candidate home:

```text
compiler/check/memory
```

The public model should be:

```go
type Location struct { ... }
type MemoryState struct { ... }
type Mutation struct { Kind, Target, Key, Value, Dominance }
```

Current scattered path helpers should collapse into this package. The solver
should not need to know whether a fact came from a table literal, field write,
alias replay, or captured mutation to apply the same path-law rules.

### Flow State Domain

Owns the persistent state of intraprocedural analysis.

The final `AbstractState` should be a product:

- memory facts,
- numeric facts,
- shape/presence facts,
- relation facts,
- termination facts,
- effect facts.

Candidate home:

```text
compiler/check/flowstate
```

or inside `types/flow` if it remains independent of checker-specific APIs.

Current weakness:

`types/flow.ProductDomain` is the closest modern abstraction, but it is mostly
used transiently during narrowing queries. The main solver still stores raw
maps and side caches. That split should disappear. Query-time narrowing should
read from the same abstract state product that transfer functions update.

### Relation Domain

Owns facts that connect multiple paths or tuple slots:

- error-return `(value, err)` correlation,
- sibling return-slot narrowing,
- predicate links,
- assertion links,
- type-test links,
- tuple-slot relation facts,
- custom error records.

Candidate home:

```text
compiler/check/domain/relation
```

This is where error-return convention should live. It should not be encoded as
scattered checks for exactly two return slots at call sites. The canonical
shape is a relation:

```go
type TupleRelation struct {
    Slots []SlotPredicate
}
```

The current `(value, err)` convention is then one predefined relation, not a
special checker behavior.

### Effect Domain

Owns facts about what a function or call can do:

- termination,
- error-return relation attachment,
- path refinements caused by assertions/predicates,
- table/container mutation effects,
- callback effects,
- key-collector effects,
- external contract effects.

Candidate home:

```text
compiler/check/domain/effect
```

Effect inference must be a normal abstract-interpretation output:

```text
CallSite + CalleeSummary + AbstractState -> EffectDelta
```

The effect delta is then applied by transfer or stored in function facts. It
must not be an after-the-fact patch that rewrites types without going through
the memory/relation/effect domains.

Effect summaries should be explicit:

```go
type EffectSummary struct {
    Mutations []memory.Mutation
    Relations []relation.TupleRelation
    Refinements []relation.PathRelation
    Terminates TerminationEffect
}
```

Current effects such as error-return correlation, captured container mutation,
and key-collector propagation become instances of this summary.

### Function Fact Domain

Owns all interprocedural facts about functions.

The stored authority remains:

```go
type FunctionFact struct {
    Summary []typ.Type
    Narrow  []typ.Type
    Type    typ.Type
}
```

But its operations should move out of `returns`:

```text
compiler/check/domain/functionfact
```

It owns:

- same-shape function fact merge,
- param-slot merge,
- return-vector merge delegation,
- effect/spec/refinement merge,
- function fact widening,
- function fact normalization,
- function fact equality.

The param-slot policy must be a named domain object, not scattered helpers:

```go
type ParamSlotDomain struct {
    Mode MergeMode // precise join or convergence widening
}
```

The current `candidateRefinesFunctionParam`, `typeRefinesTableKeyByTruthiness`,
`preferConcreteOverSoftType`, and related functions become methods or private
support functions of this domain.

### Return Summary Domain

Owns return-vector shape and convergence:

- arity normalization,
- nil-slot handling,
- `unknown` as unresolved runtime behavior,
- stale nil-only regression prevention,
- recursive structural-growth cutoff,
- concrete-over-soft container refinement,
- return-slot row-tail merging,
- function-return widening.

Candidate home:

```text
compiler/check/domain/returnsummary
```

The existing `returns` package can either become this package or stop owning
non-return policy.

### Parameter Evidence Domain

Owns all evidence about parameters:

- call-site observations,
- body-derived contracts,
- signature facts,
- param-use projection,
- soft annotations,
- table-top absorption,
- nilability splitting,
- map/record joins,
- call graph propagation.

Candidate home:

```text
compiler/check/domain/paramevidence
```

Current split:

- some policy lives in `compiler/check/infer/paramhints`,
- some lives in `compiler/check/returns/widen.go`,
- some lives in return SCC inference,
- some lives in interproc postflow.

Final rule:

Orchestration may stay in inference packages, but merge/canonicalization policy
belongs to the parameter evidence domain.

### Interproc Fact Domain

Owns the whole product:

```go
type FactsDomain struct {
    FunctionFacts FunctionFactDomain
    ParamEvidence ParamEvidenceDomain
    LiteralSigs   LiteralSignatureDomain
    Captures      CaptureDomain
    Constructors  ConstructorDomain
    Effects       EffectDomain
}
```

Candidate home:

```text
compiler/check/domain/interproc
```

It exposes only:

```go
Normalize(facts)
Leq(a, b)
Join(a, b)
Widen(prev, next)
Equal(a, b)
```

The store calls this domain. Producers emit deltas. Producers do not call local
helper joins directly.

## Helper Cluster Ownership

| Current Cluster | Current Location | Final Owner |
|---|---|---|
| `JoinFacts`, `WidenFacts`, fact equality | `compiler/check/returns` | `domain/interproc` |
| function fact type merge | `compiler/check/returns/join.go` | `domain/functionfact` |
| function param-slot refinement | `compiler/check/returns/join.go`, `widen.go` | `domain/functionfact.ParamSlotDomain` |
| return-vector merge/repair | `compiler/check/returns/join.go` | `domain/returnsummary` |
| table-top absorption | `infer/paramhints`, `returns/widen.go` | `domain/paramevidence` plus value-domain classifier |
| soft vs concrete evidence | `typ/soft.go`, `returns/widen.go`, return overlay | `domain/value` evidence policy |
| open-record row-tail merge | `types/typ/policy.go` | `domain/value` row-shape policy |
| path/query/alias identity | `constraint`, `flowbuild/path`, `flow/pathkey` | `memory` |
| table/container mutation replay | `nested`, `returns`, `flowbuild`, `flow` | `memory` mutation domain |
| error-return convention | `erreffect`, call/return inference | `domain/relation` |
| effect inference | `effects`, `erreffect`, `flowbuild`, `nested`, `returns` | `domain/effect` |
| body parameter contracts | `infer/return`, `flowbuild/assign` | `domain/paramevidence` |
| Salsa snapshot inputs | `store/snapshot_inputs.go` | keep in store, but document as cache boundary |

## Worked Consolidation Examples

### Table-Key Truthiness Refinement

Current smell:

```go
candidateRefinesFunctionParam(candidate, baseline)
typeRefinesTableKeyByTruthiness(candidate, baseline)
recordRefinesTableKeyByTruthiness(candidate, baseline)
```

These helpers are trying to express one domain law:

```text
A table-like parameter fact may refine its key domain by removing falsy key
members only if the table value domain and structural frame are preserved.
```

Final home:

```text
domain/value.Refinement
domain/functionfact.ParamSlotDomain
domain/paramevidence
```

Final expression:

```go
refinement := value.Refinement{
    Kind: value.RefineTruthyKey,
    PreserveFrame: true,
    PreserveValue: true,
}
paramSlot.Join(existing, candidate, refinement)
```

The check is no longer a local function-param helper. It is a value-domain
refinement rule reused by parameter evidence, function facts, and return
summary map-key refinement.

### Soft Evidence Replacement

Current smell:

```go
preferConcreteOverSoftType(a, b)
typ.PruneSoftUnionMembers(t)
reconcileSoftAnnotatedInference(base, inferred)
```

These are fragments of one evidence-ordering law:

```text
hard concrete evidence dominates soft placeholder evidence, but nil alone does
not erase soft structured evidence.
```

Final home:

```text
domain/value.EvidenceOrder
```

Final expression:

```go
EvidenceOrder.Select(existing, candidate)
```

Every caller gets the same policy:

- soft annotation refinement,
- function parameter facts,
- parameter evidence,
- return-summary container refinement,
- flow assignment refinement.

### Open-Record Row Tail

Current smell:

Open-record behavior is split between record join, subtyping, table literal
contextualization, and external-regression fixes.

Canonical law:

```text
A missing field on an open record is row-tail evidence, not proof of nil.
A missing field on a closed record is absence.
```

Final home:

```text
domain/value.RowShape
```

Final API:

```go
RowShape.FieldEvidence(record, fieldName) FieldEvidence
```

The rest of the checker asks for field evidence. It does not rediscover whether
the record is open, closed, map-like, or table-top.

### Captured Table Mutation Replay

Current smell:

Captured table inserts, generic container mutations, parent replay, direct
flow mutators, and nested function calls have separate paths.

Canonical law:

```text
A mutation has one semantic operator and one memory location. Replay is valid
only when alias identity, dominance, and operator kind are preserved.
```

Final home:

```text
compiler/check/memory
```

Final expression:

```go
MemoryState.Apply(Mutation{
    Kind: MutationTableElement,
    Target: Location,
    Value: Type,
    Provenance: CapturedCall,
})
```

The same apply path handles direct `table.insert`, nested captured insert, and
exported callback replay.

### Error-Return Correlation

Current smell:

Several phases know about the `(value, err)` convention, arity checks, and
success/failure narrowing.

Canonical law:

```text
Error-return behavior is a tuple relation over return slots, not a special case
of a two-result function.
```

Final home:

```text
domain/relation
```

Final expression:

```go
RelationDomain.Attach(ReturnTupleRelation{
    Success: { ErrSlot: Nil, ValueSlot: NonNilOrUnknown },
    Failure: { ErrSlot: NonNil, ValueSlot: NilOrUnknown },
})
```

The canonical Lua `(value, err)` convention is one predefined relation. Future
relations do not require new helper clusters.

## Target Data Flow

The final flow should be:

```text
source
  -> CFG + symbol graph
  -> normalized checker IR
  -> abstract transfer over AbstractState
  -> queryable solved state
  -> function result
  -> interproc fact delta
  -> FactsDomain.Join or FactsDomain.Widen
  -> Salsa input update
  -> dependent function-result query revalidation
```

Every arrow has one owner.

No phase should secretly perform another local abstract interpretation unless
that interpretation is a named domain transfer over the same `AbstractState`.

Preflow, local SCC inference, and return overlay currently exist for good
reasons. The design target is not to delete their semantics. The design target
is to make them clients of the same domain objects instead of separate local
machines.

## Salsa And Cache Model

Current good shape:

- function-result keys are stable graph/parent identities,
- interproc snapshots are `db.Input`s,
- updating facts bumps dependent queries through the database,
- core type queries are Salsa-style pure queries.

Current weak shape:

- the checker still has several non-Salsa local caches with implicit lifetimes,
- flow solution caches are manually invalidated,
- some expensive shape scans are repeated because domain operations are not
  centralized,
- param-use projection can rescan AST bodies instead of reading a graph-indexed
  use summary.

Canonical Salsa wiring:

```text
db.Input[ManifestKey]        -> module/type environment queries
db.Input[GraphKey]           -> graph-derived summaries
db.Input[InterprocGraphKey]  -> function-result queries
db.Input[SymbolKey]          -> constructor/refinement/effect summaries

FuncResultQuery(GraphID, ParentHash)
  reads graph bundle
  reads interproc snapshot inputs
  builds transfer program
  solves abstract state
  publishes immutable result
```

The query key is stable identity. The dependency edges come from the exact
inputs read during analysis. There should be no artificial revision number in
the function key and no manual cache clearing for correctness.

Final cache contracts:

1. Source inputs are `db.Input`s:
   - manifests,
   - parent scope,
   - CFG identity,
   - interproc facts,
   - constructor fields,
   - function refinements.
2. Pure expensive computations are `db.Query`s:
   - core type lookup/index/method/operator queries,
   - function result,
   - parameter-use summary by graph/function,
   - shape classification for large recursive types if profiling confirms it.
3. Intraprocedural flow state remains per-function and ephemeral unless it is
   keyed by the exact immutable input bundle. Do not put hot per-edge transfer
   into Salsa if dependency recording costs more than recomputation.
4. Domain operations must be pure and deterministic so they can be memoized
   safely when profiling justifies it.
5. Cache lifetime must be explicit in package docs. No cache should depend on
   call order for correctness.

Performance target:

- fewer repeated shape scans,
- fewer temporary maps in hot merges,
- copy-on-write vectors and maps,
- immutable fact snapshots,
- stable interning/hash-consing where already available,
- no object pools until ownership is proved and structural wins are exhausted.

## Weak Points To Fix In The Design

### 1. Domain Laws Are Not Named

The checker has laws such as:

- hard evidence dominates soft evidence,
- `unknown` in return summaries is unresolved runtime behavior,
- open record absent field means row-tail, not nil,
- nil field can satisfy optional absence in record subtyping,
- table-top can absorb precise table evidence in parameter hints,
- truthy refinement can remove falsy key alternatives.

Today many of these appear as function names buried in unrelated packages. They
must become named laws of specific domains.

### 2. Too Many Local Abstract Interpreters

`flowbuild`, `types/flow`, return SCC inference, preflow synthesis, return
overlay refresh, condition extraction, and interproc widening each perform part
of the abstract interpretation.

The final design should have one abstract state model and several orchestration
phases. The orchestration may be complex; the lattice rules cannot be local.

### 3. Memory Is Not First-Class Enough

Field writes, table inserts, dynamic indexes, aliases, captured mutations, and
path queries all affect the same memory model. They are currently split across
multiple packages.

This causes bugs where:

- parent-derived structure outranks explicit child-path facts,
- captured table inserts replay through the wrong mutator kind,
- alias identity and dominance are checked locally,
- nil overwrite and optional absence need separate fixes.

The final memory domain must own these rules.

### 4. Parameter Evidence Has Multiple Authorities

Parameter evidence currently comes from:

- call sites,
- body contracts,
- function facts,
- literal signatures,
- soft source annotations,
- param-use projection.

The final design needs one `ParameterEvidence` lattice with evidence provenance
and merge mode. The implementation should not need separate helpers for
"param hints" and "function param facts" that rediscover the same truthiness,
softness, and table-key laws.

### 5. Relation Facts Are Under-Modeled

The system supports powerful correlations, especially error-return behavior, but
the relation model is still too tied to known patterns.

The final design should model tuple/path relations directly. `(value, err)` is
then one relation instance. This keeps the system extensible without hardcoded
branch helpers or return-slot checks.

### 6. Effect Inference Is Too Distributed

Effects are currently inferred and replayed from several places:

- call specs,
- error-return inference,
- captured field/container mutation collection,
- nested mutator replay,
- key collector detection,
- predicate/assertion refinements.

Those are all effect facts. They need one summary model and one application path
through transfer. Otherwise each new effect creates its own mini analysis and
its own invalidation/caching risks.

### 7. Tests Are Too Positive-Heavy

Many external-lint regressions are "this must type-check" tests. Those are
useful, but insufficient. They can pass through accidental broadening.

Every major law needs:

- a positive test proving wanted inference,
- a negative test proving sound rejection,
- a domain law test proving normalize/join/widen idempotence and monotonicity.

## Anti-Pattern Catalog

These shapes should be rejected during the flash migration.

### Local Domain Predicate In An Orchestration Package

Example smell:

```go
func typeRefinesTableKeyByTruthiness(...)
```

If the helper defines what refinement means, it belongs to a domain package.
Orchestration packages can ask a domain whether a refinement is valid; they
cannot define the refinement locally.

### Equality-Time Repair

If equality normalizes, rebuilds, or reconciles facts to make two states look
equal, convergence bugs become invisible.

Correct shape:

```text
write boundary -> Normalize
merge boundary -> Join/Widen
equality -> structural comparison of canonical state
```

### Query-Time Fact Production

If a query discovers a fact that later code relies on as if it were stored
analysis state, the system has a hidden analysis path.

Correct shape:

```text
query can memoize an answer, but cannot publish evidence
```

### Producer-Specific Merge

If one producer has its own merge rules for a fact family, the product domain is
not canonical.

Correct shape:

```text
producer emits delta
store calls FactsDomain.Join or FactsDomain.Widen
```

### Compatibility View As Authority

A projection may exist for display or API response, but not as stored authority.
If production code writes through a view, it recreates the legacy mirror problem.

### Soundness Shortcut

Any change whose main effect is "fewer external diagnostics because `any` now
passes" is rejected unless a domain proof explains why that `any` was not truly
dynamic.

### Cache Without Input Contract

Every cache must state:

- exact key,
- immutable inputs,
- invalidation mechanism,
- whether it is semantic or performance-only.

If the cache depends on phase call order, it is not SOTA.

## Edge-Case Matrix

The migration must consider edge cases beyond the failures already seen. The
design is not complete until each row below has an owner domain and tests.

| Area | Edge Cases To Model |
|---|---|
| `unknown` | branch join with concrete, return merge with concrete, exported summary, table field, array element, call argument, relation slot |
| `any` | explicit cast to any, imported dynamic data, any flowing to concrete param, any in record field, any as table key/value, any through relation facts |
| `nil` | nil as Lua value, nil as field deletion, nil satisfying optional absence, nil array slot, nil map value, nil return slot |
| absent field | closed record absence, open row-tail unknown, map-tail optional value, table-top field access, absence after mutation |
| soft evidence | soft table top, soft array element, soft map value, nil plus soft shape, hard evidence replacing soft evidence, soft evidence across imports |
| table top | `table`, `{...}`, `{[any]: any}`, arrays, maps, closed records, open records, unions with precise tables |
| row shape | open vs closed, readonly fields, optional fields, metatables, map component overlap, discriminant tags |
| truthiness | false/nil removal, literal false keys, `and`/`or` branch values, truthy field guards, truthy dynamic indexes |
| mutation | field write, nil overwrite, dynamic index write, table insert, container send, captured mutation, exported callback mutation |
| aliasing | local alias, field alias, imported alias, method receiver alias, self alias, cyclic alias, alias after reassignment |
| dominance | dominating writes, branch-local writes, loop-carried writes, post-dominated assertions, early returns, dead paths |
| functions | optional function values, union of function signatures, method `self`, varargs, higher-order callbacks, recursive locals |
| returns | zero returns, one return, two returns, more than two returns, tuple expansion, nil padding, recursive containers |
| relations | `(value, err)`, custom error record, multiple independent relations, swapped slots, relation through wrapper, relation through any |
| effects | termination, assertion refinements, callback effects, captured mutation effects, key collection, external contract effects |
| interproc | parent scope change, module boundary, literal signatures, captured fields, constructor fields, sibling overlay, stale snapshots |
| caching | stale query after fact change, query reuse after no-op fact change, cache key missing parent scope, cache key missing graph identity |
| performance | recursive structural scan, repeated AST projection, repeated map allocation, query dependency overhead, equality-time canonicalization |

Adversarial cases must include both:

- precision cases where the checker should infer the strongest provable type;
- soundness cases where similar-looking code must still fail.

Examples:

- guarded `options.model` should infer `string`; `provider_info as any` should
  not become `string` without proof;
- `response.body or ""` should be `string`; `response.body` alone remains
  `string?`;
- open row-tail field access is `unknown`; closed missing field is absent/nil
  evidence depending on context;
- table insert before an `ipairs` loop should feed element type; branch-local
  insert must not leak if the loop is not dominated by that branch;
- `test.is_nil(err)` may refine a related value slot only if a relation fact
  proves the tuple contract.

The suite should be generated around these matrices, not around the names of
the old helper functions.

## Flash Migration Shape

The implementation should be prepared privately but merged as a direct final
shape. The production branch should not pass through partial API compatibility.

Flash migration means:

1. Introduce final domain packages.
2. Move domain laws into those packages.
3. Replace all call sites in one migration.
4. Delete old helper clusters in the same migration.
5. Delete obsolete tests that asserted old helper behavior.
6. Add law-oriented tests for the new domain boundaries.
7. Run the global replay and classify remaining diagnostics.

No step should leave:

- old helper path plus new helper path,
- adapter projections like "legacy view from canonical facts",
- duplicate merge functions for the same semantic slot,
- fallback normalization in equality,
- broad `any` acceptance to clear lints.

## Proposed Final Package Map

```text
compiler/check/domain/interproc
compiler/check/domain/functionfact
compiler/check/domain/returnsummary
compiler/check/domain/paramevidence
compiler/check/domain/relation
compiler/check/memory
compiler/check/flowstate
```

Existing packages remain as orchestration:

```text
compiler/check/flowbuild
compiler/check/synth
compiler/check/infer/return
compiler/check/infer/interproc
compiler/check/store
compiler/check/pipeline
```

Low-level pure type mechanics remain under:

```text
types/typ
types/subtype
types/query/core
types/db
```

The key rule:

Orchestration packages may decide when a fact is produced. Domain packages
decide what that fact means and how it combines.

## Verification Model For The Future Migration

Required proof after the flash migration:

```text
go test ./...
git diff --check
../scripts/verify-suite.sh
```

Required domain law tests:

- `Normalize(Normalize(x)) == Normalize(x)`
- `Join(a, b) == Join(b, a)` where the domain is intended commutative
- `Join(Join(a, b), c) == Join(a, Join(b, c))` where applicable
- `Widen(Widen(a, b), b) == Widen(a, b)`
- `a <= Join(a, b)`
- `a <= Widen(a, b)`
- derived function type equals canonical function fact projection
- no equality-time normalization bridge

Required behavior suites:

- soft vs hard evidence,
- any vs unknown,
- nil vs absent,
- open vs closed records,
- table top vs precise table shapes,
- captured table/container mutations,
- alias and dominance,
- error-return tuple relations,
- local SCC parameter evidence,
- interproc non-convergence fixtures,
- external replay reductions.

## Current Conclusion

The checker is not fundamentally the wrong idea. It is closer to a serious
abstract interpreter than it looks from isolated helper functions.

The foundational problem is organizational: the product domain exists in
behavior but not cleanly enough in code. The next design correction should not
add more local helpers. It should move the existing laws into explicit domain
objects, make memory/path identity first-class, and make Salsa/cache boundaries
documented and deliberate.

If this is done as a flash migration, the codebase should become smaller because
many helper clusters collapse into a few named domains. It should also become
easier to reason about because every merge/refinement/widening decision will
have one owner and one law-test suite.
