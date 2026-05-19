# Interproc Facts And Checker Domain Design Journal

## 2026-05-19 Design Consolidation Checkpoint

This document records the design model before the next implementation pass. It is
not an implementation plan for incremental bridges. The intended correction is a
flash migration: design the final shape, migrate directly to it, delete the old
helper clusters, and do not leave compatibility wrappers or fallback layers in
the production checker.

## 2026-05-19 Implementation Checkpoint

First flash-migration slice landed for parameter evidence ownership.

Changed production shape:

- `api.FunctionFact` now owns canonical parameter evidence in `Params`.
- `api.Facts.ParamHints` was removed.
- `api.SnapshotStore.GetParamHintsSnapshot` was removed.
- same-iteration merge and interproc widening now combine parameter evidence
  through `FunctionFacts`.
- post-flow call observation publication now emits `FunctionFacts` deltas with
  `Params` instead of writing a side channel.
- return inference seeds local function parameter evidence from canonical
  `FunctionFacts`.
- Salsa snapshot facts now track one canonical fact product for parameters,
  returns, narrow returns, and function type projection.

This is intentionally not a bridge. No production code reads a legacy
`ParamHints` fact channel and no compatibility writer reconstructs it from
`FunctionFacts`.

Second cleanup slice in the same migration:

- the local inference package was renamed from `infer/paramhints` to
  `infer/paramevidence`, then the domain was moved to
  `domain/paramevidence` when its lattice laws were consolidated;
- `LocalFuncInfo.ParamHints` became `LocalFuncInfo.ParameterEvidence`;
- phase input `ParamHintSignatures` became `ParameterEvidenceSignatures`;
- local call-graph propagation now exposes `PropagateParameterEvidence`;
- helper files and regression fixtures were renamed to parameter-evidence
  terminology;
- production checker code no longer contains `ParamHint` or `paramhints`
  identifiers.
- parameter-use projection now treats builtin `type(param)` checks and
  `param = param or {}` self-default assignments as shape-neutral guard/default
  operations instead of whole-parameter escapes. Those operations must not turn
  a call-site record observation into a closed public contract.

Verification notes:

- `go test ./...` passes.
- `git diff --check` passes.
- `../scripts/verify-suite.sh` passes go-lua checker tests and builds the Wippy
  binary, then exits non-zero in external lint targets while building Wippy
  against `github.com/wippyai/go-lua v1.5.16`.
- A temp local-replace replay under `/tmp/wippy-golua-local-replace` builds
  Wippy against this checkout without editing external code. It reduced the
  projection-related false positives, but the full external sweep is still not
  clean: tests/app 2 errors/4 warnings, session 20, actor/test 3, agent/src 12,
  docker-demo 72, llm/src 10, llm/test 9, migration 1, views 1.

Remaining cleanup after this parameter-evidence slice:

- return/narrow/type projections still need the same treatment: read-only views
  over the canonical function summary product, not separate authorities.
- Remaining local-replace external diagnostics must be classified in the next
  engine slice. Some are soundness-preserving real-code issues (`any` flowing
  into concrete contracts); some still expose missing checker power, especially
  public functions that validate invalid input with `type(...)` guards and
  should infer a wider accepted input domain without weakening the guarded body.

## 2026-05-19 Domain Rectification Checkpoint

The next flash-migration slice moved parameter evidence out of inference/return
orchestration and into a domain owner:

- `compiler/check/infer/paramevidence` was moved to
  `compiler/check/domain/paramevidence`;
- shared value-shape predicates that were duplicated during the first move were
  factored into `compiler/check/domain/value`;
- parameter-evidence vector/map normalization, join, widening, table-top
  absorption, nilability splitting, soft/concrete selection, and truthy-key
  refinement now live under domain packages;
- `returns` no longer owns parameter evidence merge helpers. Function-fact
  parameter slots delegate to `paramevidence.JoinVectors`,
  `paramevidence.FilterEmptyVector`, and `paramevidence.RefinesFunctionParam`;
- return-summary and parameter-evidence code both call `domain/value` for
  optional elision, truthy refinements, soft/concrete preference, recursive
  structural scanning, and record-extension checks;
- parameter-evidence law tests moved with the domain, so the tests describe the
  owner instead of the old return package.

This is not a compatibility bridge. The old package path and old
`WidenParameterEvidence` API were deleted. Call sites moved directly to the
domain package.

Verification for this slice so far:

- `go test ./compiler/check/domain/value` passes.
- `go test ./compiler/check/domain/paramevidence` passes.
- `go test ./compiler/check/returns` passes.
- `go test ./compiler/check/...` passes.
- `go test ./...` passes.
- `git diff --check` passes.
- Standard `../scripts/verify-suite.sh` passes the go-lua checker tests and
  Wippy binary build, then exits non-zero on external lint targets while the
  Wippy checkout is still using its pinned go-lua module: session 8 errors,
  agent/src 8 errors, docker-demo 21 errors and 2 warnings.
- Local-replace replay with
  `WIPPY_DIR=/tmp/wippy-golua-local-replace GOFLAGS=-buildvcs=false` also
  passes the go-lua checker tests and Wippy binary build, then exits non-zero
  on known external diagnostics: tests/app 2 errors/4 warnings, session 20,
  actor/test 3, agent/src 11, docker-demo 72, llm/src 9, llm/test 9,
  migration 1, views 1.

Design result:

- orchestration still decides when evidence is collected from calls, body use,
  post-flow observations, or signatures;
- the parameter-evidence domain now decides how evidence combines;
- the value domain owns shared structural predicates instead of duplicating them
  under returns and parameter evidence;
- helper names that encode parameter-specific lattice laws are no longer local
  return-package predicates.

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
   interprocedural snapshots to infer return vectors, parameter evidence, function
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

## One-Page Doctrine

The final checker should fit in this operational doctrine.

```text
1. Source syntax is lowered once into graph-indexed transfer IR.
2. Transfer IR is interpreted over one product AbstractState.
3. AbstractState owns every persistent intraprocedural fact.
4. Domain objects own every combine/refine/widen law.
5. Queries are read-only views over solved AbstractState.
6. Function inference publishes immutable InterprocDelta values.
7. FactsDomain is the only interprocedural merge/widen authority.
8. Salsa tracks immutable inputs and query dependencies.
```

Everything else is implementation detail.

The doctrine gives a direct review test:

- If code lowers syntax, it belongs in graph/IR/extract.
- If code changes state, it is transfer.
- If code combines facts, it is a domain operation.
- If code forces convergence, it is widening.
- If code answers a question, it is a query.
- If code crosses function/module boundaries, it emits or consumes a delta.
- If code caches, it must name immutable inputs and invalidation.

No rule should need to be implemented twice under different helper names.

## Abstract Machine Specification

The final checker should be specified as a small abstract machine. This gives the
code a single target shape and gives reviews a way to reject scattered helper
logic.

```text
Machine =
  Inputs
  + Program
  + State
  + Domains
  + Worklist
  + QueryView
  + Publisher
```

### Inputs

Inputs are immutable during one function analysis query:

- graph identity,
- parent scope identity,
- manifest/module environment,
- declared type environment,
- canonical interproc snapshot,
- constructor snapshot,
- effect/refinement snapshot,
- graph summaries,
- pure type-query engine.

Inputs are the only values allowed to affect the answer besides the transfer
program. If an answer depends on something not listed here, the dependency model
is incomplete.

### Program

The program is normalized checker IR:

- no source AST policy decisions,
- no hidden synthesis callbacks,
- no direct store mutation,
- no cache-dependent control flow.

Each instruction has one meaning as a transfer over `AbstractState`.

### State

The state is the product:

```text
State =
  Memory
  x Values
  x Shapes
  x NumericFacts
  x Relations
  x Effects
  x Termination
  x DiagnosticsEvidence
```

`DiagnosticsEvidence` is not user diagnostics. It is proof metadata such as
"this constraint failed here" or "this widening lost precision here". User
diagnostics are emitted after solving by querying this evidence. This keeps
diagnostic formatting out of domain semantics.

### Domains

Domains define the algebra:

```text
Normalize, Leq, Join, Meet, Refine, Widen, Equal
```

Every operation must be local to its owned component or explicitly part of a
product operation. For example, relation transfer can ask value and memory
domains to interpret a path predicate, but it cannot create a private value
merge law.

### Worklist

The worklist owns traversal, not meaning.

Allowed:

- schedule CFG points,
- schedule SCC members,
- detect local stabilization,
- invoke loop/SCC widening at declared boundaries.

Forbidden:

- prefer one fact over another,
- normalize facts,
- publish interproc state,
- recover precision after widening.

If the worklist needs semantic information to decide convergence, that
information must be exposed through `Leq` or `Equal` on the relevant domain.

### QueryView

The query view is a read-only projection over solved state.

It answers:

- type at location/point,
- relation at location/point,
- effect summary at call/function boundary,
- return tuple summary,
- parameter obligation summary,
- diagnostic projection.

It must not write facts, widen, repair state, or backfill caches that later act
as analysis state.

### Publisher

The publisher converts solved state into immutable deltas:

```text
State -> FunctionResult -> InterprocDelta
```

The publisher does not merge with previous results. It does not reconstruct
legacy channels. It emits the final product-domain representation expected by
`FactsDomain`.

### Machine Transition Rules

The core machine transitions are:

```text
step(instruction, state) = Transfer.Apply(instruction, state, domains)
join(predStates)         = AbstractState.Join(predStates, domains)
widen(prev, next)        = AbstractState.Widen(prev, next, domains)
query(state, question)   = QueryView.Answer(state, question)
publish(state)           = InterprocDelta
```

Every specialized feature should reduce to these transitions:

- branch narrowing is transfer plus join;
- field writes are memory transfer;
- table mutators are effect transfer plus memory transfer;
- assertions are effect transfer plus relation/value refinement;
- error-return behavior is relation transfer over tuple slots;
- callback behavior is higher-order effect transfer;
- local function inference is an SCC over function-state summaries;
- interproc inference is a fixpoint over `InterprocDelta` values.

If a feature cannot be expressed this way, either the machine is missing a
domain or the feature is implemented at the wrong layer.

### Machine Laws

The implementation should preserve these laws:

- Transfer is monotone with respect to domain `Leq`.
- Join is least-upper-bound or a documented approximation.
- Meet/refine never invents evidence without provenance.
- Widen is only applied at explicit recursive boundaries.
- Normalize is idempotent and is not hidden in equality.
- Query is pure over solved state.
- Publication is deterministic.
- Cache hits do not change semantics.
- Diagnostics are projections of evidence, not sources of evidence.

These laws should become test names. A regression that violates one of them is a
design regression, not a local bug.

## Ownership Ledger

Every semantic object should have one home. This table is the fastest review
tool for the future flash migration.

| Object | Born In | Canonical State | Transformed By | Queried By | Published As | Cache Boundary |
|---|---|---|---|---|---|---|
| symbol identity | graph build | graph bundle | never semantically transformed | location resolver | graph key/symbol key | graph input |
| parent scope | scope build | immutable scope state | never semantically transformed | analysis key lookup | parent hash | `FuncKey`/`GraphKey` |
| field/index path | IR/path lowering | `Location` / `MemoryState` | memory transfer | query view | captured path/mutation delta | location interning |
| local value fact | transfer | `AbstractState.Values` | value domain | type-at query | return/param/capture delta when exported | per-function state |
| table shape fact | literal/assignment transfer | value + memory domains | value/memory domains | field/index query | function/captured/container delta | type query + local state |
| branch truthiness | condition transfer | relation/value constraints | relation/value domains | query view | relation summary if it crosses boundary | per-function state |
| nil/absent evidence | assignment/field transfer | memory + value domains | memory/value domains | field query | return/param/capture delta | per-function state |
| parameter observation | call transfer | parameter evidence domain | parameter domain | function summary query | function fact delta | interproc facts input |
| body obligation | body transfer | parameter evidence domain | parameter domain | function summary query | function fact delta | graph summary + state |
| return tuple | return transfer | return summary domain | return domain | return query | function fact delta | interproc facts input |
| tuple/path relation | predicate/effect/return transfer | relation domain | relation domain | relation query | relation summary delta | local state / interproc facts |
| table mutation | assignment/effect transfer | memory domain | memory domain | iteration/field query | captured container delta | local state / interproc facts |
| call effect | effect resolution | effect domain | effect domain | transfer/query view | refinement/effect delta | effect snapshot input |
| termination fact | transfer/effect transfer | termination domain | termination domain | reachability query | function effect delta | per-function state |
| diagnostic evidence | failed constraint transfer/query | diagnostics evidence state | diagnostic projection only | diagnostics pass | no semantic delta | result only |
| constructor field | constructor transfer/publication | constructor field domain | memory/value domains | constructor query | constructor snapshot | constructor input |
| external dynamic value | manifest/effect transfer | value evidence with provenance | value/domain checks | assignability query | only if exported with provenance | manifest/type input |

Design rule:

```text
If a row needs two canonical states, the model is split incorrectly.
If a row has no cache boundary, the implementation will invent one locally.
If a row has two publishers, legacy mirror channels are coming back.
```

## Dataflow Moral Rules

The checker should be easy to explain because the direction of information never
reverses.

### Syntax To Evidence

Syntax can create observations. It cannot create authority by itself.

Examples:

- a table literal observes fields;
- a call observes arguments;
- a guard observes a branch condition;
- a return observes tuple slots.

These observations become evidence only through transfer and domain
qualification.

### Evidence To Fact

Evidence becomes a fact when the owning domain accepts it into state.

Examples:

- a field observation becomes a memory fact at a canonical location;
- a truthy guard becomes a relation/value constraint;
- a body use becomes a parameter obligation;
- a call argument becomes a parameter observation.

No producer decides global precedence. The evidence order belongs to the domain.

### Fact To Answer

Answers are read-only projections.

Examples:

- "what is the type here?",
- "does this path exclude nil?",
- "what does this function return?",
- "does this call terminate?",
- "which diagnostic should be emitted?".

An answer cannot become a fact unless a later transfer explicitly observes it
and routes it through the owning domain. This prevents query-time analysis.

### Fact To Delta

Only solved facts that cross a function or module boundary become deltas.

Examples:

- local temporary narrowing does not publish;
- body obligation publishes as parameter evidence;
- return tuple publishes as return summary and relation summary;
- captured mutation publishes as memory/effect summary;
- external contract application does not rewrite the contract.

The publisher emits a delta; `FactsDomain` combines it.

### Delta To Snapshot

Snapshots are cache inputs, not semantic repair points.

Examples:

- changed canonical facts update snapshot inputs;
- unchanged canonical facts do not invalidate queries;
- empty canonical facts clear stale inputs;
- compatibility projections are not written.

This keeps incremental revalidation honest: Salsa tracks dependencies, domains
track meaning.

## Boundary Invariants

Every boundary in the dataflow should have a small invariant that can be tested
or reviewed directly.

### Graph Boundary

Invariant:

```text
Graph identity changes only when syntax/binding identity changes.
```

This boundary may cache syntax summaries. It may not depend on interproc facts,
solved flow state, or expected call types.

### IR Boundary

Invariant:

```text
Checker IR contains operations, not answers.
```

The IR may say "apply this call effect" or "assign this value to this
location". It may not pre-decide the result type of an operation whose answer
depends on flow/interproc state.

### Transfer Boundary

Invariant:

```text
Transfer is the only state-writing semantics inside a function.
```

All writes to memory, value, relation, effect, and termination state must be
visible as transfer operations. A helper that writes state outside transfer is a
hidden interpreter.

### Join Boundary

Invariant:

```text
Branch merge uses domain Join and nothing else.
```

A branch-specific merge helper is allowed only if it is the domain's exported
join/meet/refine operation. If it knows about AST shape, it is in the wrong
layer.

### Widen Boundary

Invariant:

```text
Widen happens only at named recursive boundaries.
```

Loop widening, local function SCC widening, and interproc widening may have
different schedules, but they must call the same domain-level widening laws for
the same fact family.

### Query Boundary

Invariant:

```text
Query answers cannot become stored evidence.
```

Query caches are permitted only for answers. They must not publish facts or
change future convergence.

### Publication Boundary

Invariant:

```text
Publication emits immutable deltas and never merges them.
```

The same solved state must always produce the same delta. If publication reads
previous facts to decide how to shape the delta, it is doing merge work in the
wrong layer.

### Snapshot Boundary

Invariant:

```text
Snapshot updates are semantic no-ops except for dependency invalidation.
```

Setting a snapshot input can make queries rerun. It cannot normalize, widen,
infer, or delete evidence except by reflecting the already-canonical facts.

### Diagnostic Boundary

Invariant:

```text
Diagnostics observe proof failure; they do not define type behavior.
```

A diagnostic pass may ask why a check failed. It may not make the check pass or
fail by changing evidence.

## Evidence Authority Model

The checker should be precise because it carries proof, not because it guesses.
Authority is therefore part of evidence. It is not a global total order; it is a
domain-specific partial order over a specific question.

Canonical evidence shape:

```text
Evidence =
  Location
  + Value/Predicate/Effect
  + Provenance
  + Authority
  + Scope
  + Phase
  + SourceSpan
```

`SourceSpan` may be absent for synthetic or imported evidence, but provenance
must not be absent.

### Authority Classes

The final design should name these authority classes explicitly.

| Authority | Meaning | Can Prove Concrete Contract? | Can Be Weakened By Join? | Can Publish? |
|---|---|---|---|---|
| explicit contract | user/API annotation or manifest contract | yes | only through declared variance/summary abstraction | yes |
| hard runtime proof | guard, assertion, dominance-proven assignment | yes | yes at control-flow join | if it crosses boundary |
| relation proof | fact derived from tuple/path relation | yes for related locations | yes when relation path is lost | if relation crosses boundary |
| effect proof | applied call/effect summary | yes if effect declares it | yes at join/widen | yes as effect/summary |
| body obligation | function body requires a shape | yes for parameter contract inference | yes at recursive widen | yes |
| call observation | caller passed a shape | no by itself | yes | yes as weak evidence |
| contextual literal evidence | expected type applied at literal boundary | yes for that literal | yes | yes if literal escapes |
| soft annotation | low-authority annotation hint | no without compatible proof | yes | only as soft evidence |
| unresolved observation | `unknown` | no | yes but not erased silently | yes as unknown |
| dynamic top | `any` | no without explicit cast/contract | yes as dynamic top | yes as any |

This table prevents the common mistake of treating all useful evidence as the
same. A call observation is useful for inference, but it is not proof that the
callee accepts that shape. An explicit `any` is useful information, but it is
not proof of a concrete field.

### Conflict Resolution

Conflicts should be resolved by the owning domain, not by producer preference.

| Conflict | Owner | Correct Resolution |
|---|---|---|
| hard proof vs soft annotation | evidence/value domain | hard proof wins for the proven path |
| explicit `any` vs expected concrete param | assignability/value domain | reject unless cast/contract proves concrete |
| unknown return vs concrete return | return domain | preserve unresolved behavior unless domain law proves refinement |
| call observation vs body obligation | parameter domain | body obligation is stronger contract evidence |
| parent table shape vs child-path write | memory domain | child-path fact wins for that path |
| closed missing field vs open row tail | value/memory domain | closed absence and open unknown tail stay distinct |
| relation proof vs unrelated assignment | relation/memory domain | relation survives only if location identity is preserved |
| widening precision loss vs later query | owning domain | query observes widened state; no post-widen repair |

Conflict policy must be testable as a domain law. If the test has to construct a
whole checker to decide the conflict, the domain boundary is still too implicit.

### Proof-Carrying Facts

Every persistent fact should be explainable as:

```text
fact = domain.accept(observation, provenance, authority, location)
```

Queries should be able to answer both:

- the abstract answer, such as "this value is string";
- the proof route, such as "truthy guard on this location removed nil".

The proof route does not need to be exposed in normal diagnostics, but it must
exist in the design. Without it, the checker cannot distinguish real precision
from accidental broadening.

### Precision And Soundness Contract

Precision can increase only by proof.

Allowed precision gains:

- guard removes nil/false from the exact guarded location;
- assertion effect narrows the declared target relation;
- body obligation records a parameter shape the body actually reads;
- table literal contextual typing applies at the literal boundary;
- relation summary narrows linked tuple slots after a predicate.

Forbidden precision gains:

- callee expected type rewrites caller evidence;
- repeated callers vote a parameter into a concrete contract;
- `any` becomes a concrete record because a later field is used;
- closed missing field becomes open unknown tail to avoid an error;
- cached answer is reused after an untracked dependency changed.

Precision can decrease only at named abstraction boundaries:

- branch join,
- loop widening,
- local function SCC widening,
- interproc widening,
- published summary abstraction.

Precision must not decrease at:

- equality,
- snapshot update,
- diagnostics,
- compatibility projection,
- query cache lookup.

This is the soundness/performance contract. Faster analysis is valid only if it
computes the same evidence or a documented domain approximation at a named
boundary.

### Absence Of Evidence

Absence is not a proof.

Rules:

- no field evidence does not mean field is nil;
- no relation evidence does not mean slots are independent if a relation was
  dropped by a bug;
- no return evidence does not mean zero returns unless arity is known;
- no effect evidence does not mean pure call unless the effect row is closed;
- no param evidence does not mean `any`; it means unresolved until declared or
  inferred evidence exists.

This is where many false positives and false negatives start. The final domains
should model absence explicitly instead of using nil maps as semantic answers.

## Dataflow Proof Traces

Every important inference should have a trace format. This is not a logging
requirement for the first implementation. It is the mental model for proving the
checker did the right thing.

Trace skeleton:

```text
Observation
  -> Location
  -> Evidence
  -> Domain acceptance
  -> State fact
  -> Join/Widen if any
  -> Query answer
  -> Publication if any
```

### Guarded Field Trace

```text
Observation: if options.model then
Location:    Location(options).field("model")
Evidence:    truthy predicate, hard runtime proof
Domain:      RelationDomain + ValueDomain
State:       path excludes nil/false on true branch
Query:       provider.open argument reads non-nil field type
Publish:     none unless the relation escapes through a summary
```

Wrong trace:

```text
provider.open expects string -> options.model becomes string
```

The wrong trace reverses dataflow.

### Error Return Trace

```text
Observation: local value, err = f()
Location:    return tuple slots assigned to local locations
Evidence:    f publishes tuple relation
Domain:      RelationDomain accepts slot correlation
State:       err nil branch relates value slot to success case
Query:       value.field sees success-side value evidence
Publish:     wrapper republishes tuple relation only if slot identity is preserved
```

Wrong trace:

```text
function has two returns -> assume value/error convention
```

The wrong trace invents relation evidence from arity.

### Dynamic Payload Trace

```text
Observation: payload = json.decode(raw)
Location:    payload
Evidence:    imported dynamic value
Domain:      ValueDomain records any/unknown with provenance
State:       payload.name remains dynamic/unresolved
Query:       needs_string(payload.name) requires proof
Publish:     dynamic evidence only if exported
```

Wrong trace:

```text
needs_string expects string -> payload.name becomes string
```

The wrong trace treats expected type as evidence.

### Captured Mutation Trace

```text
Observation: nested function inserts into state.items
Location:    canonical location for state.items
Evidence:    mutation effect with captured provenance
Domain:      EffectDomain applies MemoryDomain mutation
State:       array element fact at state.items
Query:       ipairs reads element fact if dominance/escape permits it
Publish:     captured container mutation delta if it crosses function boundary
```

Wrong trace:

```text
captured mutation replay builds a new parent table shape
```

The wrong trace loses operator kind and child-path authority.

### Trace Review Rule

For any new inference, a reviewer should be able to ask:

- What was observed?
- What is the canonical location?
- What authority does the evidence have?
- Which domain accepted it?
- Where can it lose precision?
- Which query read it?
- Does it publish, and if so as which delta?
- Which cache boundary owns reuse?

If the answer starts with "this helper checks whether...", the design likely
needs another domain operation instead of another helper.

## Semantic Atoms

The final design should use a small shared vocabulary. These words should have
one meaning everywhere in the checker.

### Value

A `Value` is an abstract runtime Lua value.

It can be concrete, literal, structural, function-like, `nil`, `unknown`, or
`any`. It is not a source annotation and not a location. A value domain may say
how values combine; it may not decide where a value came from.

### Location

A `Location` is an abstract program place where evidence can attach.

Examples:

- symbol at SSA version,
- field path,
- index path,
- tuple slot,
- receiver slot,
- captured variable,
- return slot,
- graph/function identity.

Locations are canonical before transfer. AST paths and SSA paths cannot both be
authoritative.

## Location And Memory Calculus

The final checker needs one answer to the question:

```text
Are these two pieces of evidence about the same runtime place?
```

If that answer is local to each helper, precision will stay fragile. Guarded
fields, captured mutations, alias replay, tuple relations, and table-key
refinements all depend on the same location calculus.

### Location Shape

A location should be a canonical structured value, not a string path and not an
AST node.

```text
Location =
  Root
  + Version
  + PathSegments
  + ScopeIdentity
  + ProvenanceClass
```

Roots:

- local symbol root,
- parameter root,
- receiver `self` root,
- upvalue/captured root,
- return tuple root,
- temporary tuple result root,
- module export root,
- constructor instance root,
- external/imported value root.

Segments:

- named field,
- literal index,
- dynamic index with key evidence,
- array element,
- map value,
- tuple slot,
- metatable/member access when modeled,
- synthetic effect target.

`Version` belongs to the root or to a versioned location identity. It should not
be smuggled into a string suffix. `ScopeIdentity` is required for parent-scoped
facts so two equal-looking symbols in different parent scopes do not collide.

### Canonicalization Laws

Location canonicalization should obey these laws:

- resolving the same symbol/path at the same CFG point returns the same
  canonical location;
- resolving different lexical symbols never collides, even when names match;
- aliases are explicit equivalence/forwarding facts, not path rewrites;
- field and index segments are interned/normalized before storage;
- dynamic index evidence is preserved and not collapsed to `string` unless a
  proof refines it;
- tuple slots remain tuple slots until assignment or forwarding gives them a
  concrete destination;
- captured locations retain lexical owner identity;
- module/export locations retain module identity;
- open row-tail access and closed missing-field access produce different
  locations/evidence.

These laws should be tested without a whole checker. A location unit test should
be able to prove whether two references alias, differ, or are unknown.

### Memory State Shape

Memory state should be the product of several maps with one owner:

```text
MemoryState =
  ValueAt(Location)
  + PresenceAt(Location)
  + Children(Location)
  + AliasFacts
  + MutationLog
  + DominanceFacts
  + EscapeFacts
```

`ValueAt` says what value evidence is known at a location.
`PresenceAt` distinguishes present, absent, nil value, unknown presence, and
open row-tail unknown.
`Children` records known child facts without forcing a parent table rewrite.
`AliasFacts` records location identity relations and their dominance.
`MutationLog` records effectful writes with operator kind.
`DominanceFacts` tells whether a write/guard reaches a query point.
`EscapeFacts` tells whether a local fact can publish across a boundary.

None of these should be represented by "map missing means nil". Absence of a map
entry means no stored fact for that component.

### Read Law

A memory read answers by ordered evidence, not by helper preference.

Read order for a path should be:

1. exact dominated location fact;
2. exact relation-refined fact for the same location;
3. exact child-path mutation fact;
4. alias-forwarded fact whose alias is valid at the query point;
5. declared/constructed parent shape projected through the path;
6. open row-tail evidence;
7. unresolved evidence.

Forbidden read behavior:

- expected callee type becomes read evidence;
- parent table shape overwrites explicit child mutation;
- closed missing field becomes open row-tail unknown;
- dynamic index write broadens every named field without proof;
- stale query cache answers for a different location version.

This read law is where many current helper clusters should collapse.

### Write And Mutation Law

A write is not just "join this type into a table".

Write shape:

```text
Write =
  Target Location
  + OperatorKind
  + ValueEvidence
  + Dominance
  + Provenance
```

Operator kinds:

- assignment,
- field write,
- nil overwrite,
- deletion/absence write if Lua semantics or API effect establishes deletion,
- dynamic index write,
- array element insert,
- map value update,
- container send/receive,
- captured mutation replay.

The operator kind is semantic. `table.insert(x, v)`, `x[k] = v`, and
`x.field = v` may all affect a table, but they do not have the same path law.
Captured replay must preserve the original operator kind.

### Alias And Dominance Law

Alias facts are valid only over a control-flow region.

Rules:

- alias created by assignment is valid until reassignment or invalidating
  mutation;
- field alias preserves the exact field path it came from;
- dynamic index alias preserves key evidence;
- branch-local alias facts do not leak unless dominance proves they reach the
  query point;
- loop-carried aliases widen at the loop boundary;
- captured aliases include lexical owner and escape information.

Relation facts must reference canonical locations, not syntactic expressions.
If assignment preserves location identity, relations can transfer. If it copies
only a value and loses tuple/path identity, relation facts must not silently
survive.

### Tuple Slot Law

Tuple slots are locations, not just positions in a slice.

Rules:

- return arity is part of tuple identity;
- nil padding is explicit;
- wrapper forwarding preserves tuple-slot relation only when forwarding is
  identity-preserving;
- assignment from tuple slot to local location records a relation edge from slot
  to local;
- swapped or reordered returns update relation mapping explicitly;
- vararg expansion has its own location/evidence policy and cannot be treated
  as fixed tuple identity without proof.

This prevents the `(value, err)` convention from becoming an arity heuristic.

### Presence Law

Presence is separate from value type.

States:

- present with value evidence,
- present with nil value,
- absent from closed structure,
- optional in declared structure,
- unknown via open row tail,
- unknown via dynamic table top.

Important distinctions:

- `field = nil` is not automatically the same as absent unless the domain rule
  for that context says so;
- optional declared field is not the same as proven absence;
- open record tail gives unknown evidence, not nil evidence;
- map value may be nil even when key presence is unknown;
- table top preserves that a value is table-like without proving named fields.

Presence should be tested as its own domain law. It is too important to hide in
record subtyping or field lookup helpers.

### Publication Law

Only memory facts that escape the local function become interproc deltas.

Publishable memory evidence:

- captured variable type,
- captured field assignment,
- captured container mutation,
- constructor field,
- return value/tuple slot,
- parameter obligation/effect,
- module export field.

Non-publishable memory evidence:

- branch-local narrowing,
- local alias that does not escape,
- temporary tuple slot after assignment unless relation summary requires it,
- diagnostic-only failure evidence,
- query cache answer.

Publication should project from memory state. It should not reconstruct memory
facts by rescanning AST or replaying helper-specific summaries.

### Performance Consequences

The location calculus is also a performance boundary.

Expected wins:

- interned locations make map keys cheap and stable;
- path parsing disappears from hot query paths;
- child-path facts avoid rebuilding whole parent tables;
- alias and dominance checks become graph-indexed facts;
- relation queries compare location IDs instead of syntactic paths;
- captured mutation replay reuses the same mutation operator.

Rejected performance shapes:

- stringifying paths to compare them in hot loops;
- reparsing path suffixes during every narrowed query;
- rebuilding parent records for each child write;
- using object pools before ownership of locations and memory facts is proven;
- caching read answers without a solved-state/location-version key.

### Location Law Tests

The flash migration should add focused tests for:

- same expression at same point resolves to same location;
- same name in different scopes resolves to different locations;
- alias validity ends at reassignment;
- branch-local alias does not leak;
- dynamic index write does not overwrite unrelated named field;
- child field write outranks parent shape at that child;
- closed missing field differs from open row-tail field;
- tuple relation survives identity forwarding;
- tuple relation dies on reorder unless remapped;
- captured mutation preserves operator kind and target location;
- nil value and absence remain distinguishable.

These tests are foundational. If they pass, many higher-level inference tests
become much simpler because they no longer need to encode location policy.

### Evidence

`Evidence` is a value plus provenance and authority.

Examples:

- explicit annotation,
- hard runtime proof,
- body obligation,
- call observation,
- soft annotation,
- unresolved observation,
- imported dynamic value.

Evidence is not automatically truth. Domains decide how evidence combines.

### Fact

A `Fact` is evidence that has been accepted into a domain state.

Facts are persistent inside `AbstractState` or inside an immutable
`InterprocDelta`. Raw observations are not facts until transfer/domain logic
accepts them.

### Constraint

A `Constraint` restricts possible facts along a control-flow path.

Examples:

- truthy/falsy,
- type test,
- nil/non-nil,
- has-field,
- numeric bound,
- relation branch.

Constraints do not mutate storage by themselves. Transfer applies them to
`AbstractState`; queries read the result.

### Relation

A `Relation` connects multiple locations.

Examples:

- return slot 1 being nil implies return slot 0 is non-nil,
- assertion on one symbol narrows a sibling path,
- method receiver relation to `self`,
- callback argument relation to caller state.

Relations are not encoded as special value types. They are first-class domain
facts.

### Effect

An `Effect` describes what execution of a call or instruction can do.

Examples:

- mutate memory,
- terminate,
- refine an argument,
- produce a tuple relation,
- call a callback,
- collect keys.

Effects are applied by transfer. They do not rewrite types directly.

## Relation And Effect Calculus

Relations and effects are the bridge between local flow precision and
interprocedural power. They must be first-class domain facts, not names of known
functions.

Core rule:

```text
Relations describe conditional truth between locations.
Effects describe state transitions caused by execution.
```

An assertion, predicate, table mutator, callback, error-return convention, and
terminating function all fit this rule.

### Relation Shape

A relation should be represented as a structured fact:

```text
Relation =
  RelationID
  + Participants
  + Arms
  + Directionality
  + Validity
  + Provenance
```

Participants are canonical locations:

- tuple slots,
- locals,
- fields,
- indexes,
- receiver/self,
- callback arguments,
- captured paths.

Arms describe conditional cases:

- success/failure branch,
- true/false predicate branch,
- nil/non-nil branch,
- type-test branch,
- discriminant branch,
- custom effect branch.

Directionality matters. Some relations are bidirectional; many are not. For
example, `err == nil` may imply success-side value evidence, but using a value
does not necessarily prove `err == nil` unless the relation declares that
reverse implication.

Validity records when the relation is safe to apply:

- CFG region,
- dominance/post-dominance requirement,
- location identity requirement,
- alias validity,
- tuple-slot identity,
- function summary boundary,
- effect precondition.

### Relation Operations

The relation domain should own these operations:

```text
Attach(relation, state)
Assume(location predicate, state)
Remap(relation, location mapping)
Project(location, state)
Join(a, b)
Widen(prev, next)
Publish(relation, boundary)
```

`Attach` stores a relation after validating participants.
`Assume` applies a branch predicate and derives consequences.
`Remap` preserves a relation through assignment, wrapper forwarding, or tuple
reordering only when identity mapping is explicit.
`Project` answers what a relation proves about a queried location.
`Join` keeps only facts valid on all incoming paths or marks path-conditional
arms explicitly.
`Widen` bounds recursive relation growth.
`Publish` emits only relations that remain meaningful across the boundary.

Forbidden relation operations:

- infer relation from return arity alone;
- preserve relation after assignment without location mapping;
- treat a predicate function name as proof outside effect transfer;
- erase relation provenance during join;
- encode relation as a special `typ.Type`.

### Tuple Relation Law

The `(value, err)` convention is one tuple relation instance:

```text
SuccessArm: err is nil     -> value is success value
FailureArm: err is non-nil -> value is nil/unknown failure value
```

It is not:

- any two-return function,
- any call followed by `test.is_nil`,
- a special return-summary vector,
- a call-checking hack.

Custom error records, boolean-success APIs, result objects, and status-code
APIs should be expressible by defining different relation arms over locations.

### Predicate Relation Law

Predicate/assertion functions apply relations through effects.

Examples:

- `is_nil(x)` proves nil/non-nil branches for `x`;
- `is_string(x)` proves string/non-string branches for `x`;
- `assert_type(x, "string")` refines `x` or terminates;
- `has_field(x, "name")` proves presence for `x.name`;
- custom manifest predicate proves declared relation arms.

The function name is only a lookup key for an effect summary. The effect summary
is the semantic object.

Wrong shape:

```text
if call name == "test.is_nil" then patch value type
```

Correct shape:

```text
call -> effect summary -> relation transfer -> query
```

### Effect Shape

An effect summary should be a structured transition:

```text
Effect =
  EffectID
  + Preconditions
  + MemoryEffects
  + RelationEffects
  + ValueEffects
  + TerminationEffect
  + CallbackEffects
  + PublicationPolicy
  + Provenance
```

Preconditions decide when the effect is valid.
Memory effects mutate locations through `MemoryDomain`.
Relation effects attach or assume relations through `RelationDomain`.
Value effects refine or produce value evidence through `ValueDomain`.
Termination effects update reachability through `TerminationDomain`.
Callback effects describe higher-order execution.
Publication policy decides whether the summary can cross a function/module
boundary.

### Effect Application Law

Applying an effect is transfer:

```text
Call instruction
  -> resolve callee/effect summary
  -> instantiate summary with actual argument/receiver/return locations
  -> validate preconditions
  -> apply memory effects
  -> apply relation effects
  -> apply value effects
  -> apply termination effects
  -> schedule callback effects if invoked
```

Every sub-step calls the owning domain. The effect domain coordinates; it does
not own memory, value, relation, or termination laws.

### Callback Effect Law

Callbacks are effectful calls whose callee is a parameter or field.

Rules:

- callback invocation has its own call site and locations;
- callback argument evidence flows as call observations;
- callback return/effect evidence flows back only through declared callback
  summary;
- captured caller memory can be mutated only through explicit captured location
  effects;
- unknown callback effects are not pure unless the effect row is closed.

This prevents higher-order code from becoming a blind spot or a source of
unsound broadening.

### Termination Law

Termination is an effect, not a diagnostic side channel.

Examples:

- `error()` terminates the current path;
- assertion failure terminates one branch;
- `return` terminates the current function path;
- infinite loop may terminate analysis reachability differently from runtime
  non-return depending on proof.

Reachability must update before value queries observe post-call state. Otherwise
the checker can report false positives from impossible paths or accept values
from dead branches.

### Open And Closed Effect Rows

Effects need the same open/closed discipline as structural types.

Closed effect row:

```text
This call has exactly these modeled effects.
```

Open effect row:

```text
This call has at least these effects; unknown effects may remain.
```

Rules:

- no effect summary does not mean pure call;
- closed pure summary can prove no mutation/termination/refinement;
- open summary cannot prove absence of unknown mutation;
- unknown external call must not refine values without a declared effect;
- manifest effects are typed inputs, not hardcoded behavior.

### Relation/Effect Join And Widen

Join:

- keeps relations/effects valid on all joined paths;
- preserves path-conditional arms when the domain represents them explicitly;
- drops or weakens facts whose participant locations are no longer identical;
- never converts absence of relation into proof of independence.

Widen:

- bounds recursive relation chains;
- bounds callback/effect expansion;
- bounds recursive captured mutation growth;
- preserves sound top/unknown effects when precision is lost.

Precision loss here must be visible as domain widening, not hidden in query or
publication.

### Publication Law

Publishable relations/effects:

- function return tuple relation,
- predicate/assertion function relation summary,
- captured memory mutation effect,
- callback invocation effect,
- termination/non-returning effect,
- external manifest effect,
- constructor/receiver mutation effect.

Non-publishable relations/effects:

- branch-local guard that does not escape;
- local assertion proof after the checked value dies;
- relation over temporary tuple slots unless remapped to exported locations;
- query-only refinement;
- diagnostic-only proof.

Publication should remap local locations to boundary locations. If a relation or
effect cannot be remapped, it does not publish.

### Performance Consequences

The relation/effect calculus should improve performance by making reuse
structural.

Expected wins:

- relation queries index by participant location;
- effect summaries are cached by callee identity and manifest/source version;
- effect instantiation is local and cheap because locations are canonical;
- callback expansion is bounded by summary widening;
- wrapper forwarding remaps relation IDs instead of resynthesizing return
  behavior;
- predicate handling uses one transfer path.

Rejected shapes:

- scanning all relations for every type query;
- recomputing effect summaries inside every call check;
- using string function names in hot semantic paths;
- replaying captured mutations by rebuilding table types;
- preserving all recursive callback effects without widening;
- clearing false positives by treating unknown effects as pure.

### Relation And Effect Law Tests

The flash migration should add focused tests for:

- tuple relation attaches only from declared summary, not arity;
- tuple relation survives identity wrapper forwarding;
- tuple relation remaps through swapped returns only with explicit mapping;
- predicate effect narrows only declared participants;
- assertion termination removes impossible paths before value query;
- unknown external call does not refine argument;
- closed pure effect proves no mutation;
- open effect row does not prove no mutation;
- callback call observation reaches callback parameter evidence;
- callback unknown effects do not mutate closed state without declaration;
- captured mutation effect preserves operator kind and target location;
- relation join does not invent independence;
- recursive relation/effect widening converges without erasing all useful proof.

## Function Boundary Summary Calculus

A function boundary is where local abstract state becomes reusable evidence for
callers. This boundary must have one product-domain object. It should not be
spread across parameter evidence, return summaries, narrow summaries, function
types, captured fields, captured containers, literal signatures, and effect
maps as independent authorities.

Core rule:

```text
FunctionSummary = abstraction(QueryView(SolvedState), BoundaryMap)
```

The summary is not a second analysis. It is a deterministic abstraction of the
solved state through the function boundary.

### Boundary Map

The boundary map explains how local locations become external locations.

```text
BoundaryMap =
  Parameters
  + Receiver
  + Returns
  + Captures
  + Exports
  + Constructors
  + CallbackSlots
```

Examples:

- parameter location maps to parameter slot;
- receiver `self` maps to receiver slot;
- local return tuple slots map to return slots;
- captured upvalue paths map to captured locations;
- module fields map to export locations;
- constructor writes map to constructor instance fields;
- callback parameters map to callback function slots.

Any summary fact that cannot be expressed through the boundary map is not
publishable. It remains local evidence.

### Summary Product

The canonical function summary should be a product:

```text
FunctionSummary =
  SignatureSurface
  x ParameterEvidence
  x ReturnTupleSummary
  x RelationSummary
  x EffectSummary
  x CaptureSummary
  x ConstructorSummary
  x ExportSummary
```

`SignatureSurface` is the user-facing callable type projection. It is derived
from the product. It is not the stored authority.

`ParameterEvidence` records annotations, body obligations, call observations,
soft evidence, contextual literal evidence, and recursive widening state.

`ReturnTupleSummary` records explicit arity, nil padding, unknown slots, any
slots, multivalue expansion policy, and per-slot provenance.

`RelationSummary` records tuple/path relations that survive the boundary map.

`EffectSummary` records memory, relation, value, termination, and callback
effects that callers must apply through transfer.

`CaptureSummary` records captured value/path/mutation evidence that escaped the
function body.

`ConstructorSummary` records constructor field facts only when construction
semantics prove them.

`ExportSummary` records module-visible fields and functions.

### Parameter Summary Law

Parameters have several evidence sources, but one domain.

Evidence sources:

- explicit parameter annotation,
- manifest/API contract,
- body obligation,
- call observation,
- function literal expected type,
- soft annotation,
- recursive SCC seed,
- interproc snapshot.

Merge policy:

- explicit contracts define the checked surface;
- body obligations can infer required structure;
- call observations are weak evidence and cannot create a hard contract alone;
- soft evidence refines only when compatible proof exists;
- recursive evidence widens only at SCC/interproc boundaries;
- optionality and nilability are separate axes;
- `any` remains dynamic top unless explicit cast/contract changes the question;
- absence of parameter evidence is unresolved, not `any`.

Wrong shape:

```text
ParamHints merge differently from FunctionFacts.Params
```

Correct shape:

```text
ParameterEvidenceDomain.Join(existing, candidate)
```

### Return Summary Law

Returns are tuples with attached relations and effects.

Rules:

- arity is explicit;
- nil padding is explicit;
- zero returns differ from one nil return;
- unknown return evidence is not bottom;
- any return evidence remains dynamic top;
- recursive return growth widens at the return domain boundary;
- narrow/success returns are derived views over tuple relation state;
- wrapper forwarding preserves return relations only through explicit location
  remapping;
- vararg return expansion has a distinct summary policy.

Wrong shape:

```text
ReturnSummaries and NarrowReturns are stored as separate truths
```

Correct shape:

```text
ReturnTupleSummary + RelationSummary -> projected narrow/success view
```

### Function Type Projection Law

A function type is a projection, not an authority.

Projection:

```text
FunctionType =
  params(ParameterEvidence)
  + returns(ReturnTupleSummary)
  + effects(EffectSummary)
  + relation metadata if the surface type can carry it
```

Rules:

- projection is deterministic and cacheable;
- projection does not write facts;
- projection does not reconcile legacy channels;
- projection must be invalidated by changes to the canonical summary product;
- two projections of the same summary must be equal.

This removes the need for bridge shapes such as "function types from facts" as a
semantic layer. A projection function may exist as a read-only view, but it is
not a merge or fallback path.

### Capture Summary Law

Captures are memory/effect facts remapped through lexical ownership.

Publishable capture facts:

- captured variable value evidence;
- captured field write;
- captured nil overwrite/deletion when modeled;
- captured table/container mutation;
- captured relation over exported/captured locations;
- captured callback effect.

Rules:

- captured paths use canonical locations with lexical owner identity;
- mutation operator kind is preserved;
- dominance/escape controls whether the mutation publishes;
- parent-derived table shape cannot overwrite child captured mutation;
- captured facts are applied by transfer in the receiving context, not by
  rebuilding parent table types.

### Constructor And Export Summary Law

Constructor and export facts are boundary memory facts.

Rules:

- constructor fields are published only from construction evidence;
- module export fields are published only from export locations;
- local helper facts do not publish just because the name is visible;
- exported functions publish their function summary product;
- imports read snapshots and apply summaries through transfer/query, not through
  local special cases.

### Call Application Law

Calling a function applies its summary to actual locations.

```text
CallSite
  + FunctionSummary
  + ActualArgumentLocations
  + ReturnDestinationLocations
  -> Transfer over AbstractState
```

Application steps:

1. check actuals against projected parameter contracts;
2. record call observations as weak parameter evidence;
3. instantiate effect summary over actual locations;
4. instantiate relation summary over return and argument locations;
5. bind return tuple summary to destination locations;
6. update termination/reachability;
7. publish caller-side deltas only after the caller solves.

Forbidden:

- expected parameter type rewrites actual evidence;
- callee summary mutates interproc store during call checking;
- caller synthesizes a new callee summary from local expectations;
- return arity heuristic creates relation summary;
- call application bypasses transfer.

### Summary Join And Widen

Function summaries combine through their domains.

Join:

- combines independent observations within one iteration;
- preserves provenance and authority;
- keeps tuple arity explicit;
- joins relations/effects only when participant remapping is compatible;
- avoids rebuilding equivalent maps or slices on no-op joins.

Widen:

- applies at local function SCC and interproc boundaries;
- bounds recursive parameter, return, capture, relation, and effect growth;
- preserves sound unknown/any distinction;
- emits precision-loss evidence for diagnostics/profiling;
- never hides convergence by equality-time normalization.

Leq/Equal:

- compare canonical summary state only;
- do not rebuild projections;
- do not normalize as repair;
- are the basis for fixpoint convergence and snapshot invalidation.

### Summary Storage Law

The stored authority should be one canonical product.

Allowed stored authority:

```text
FunctionSummary product
```

Allowed derived views:

- callable `typ.Function` surface;
- display signature;
- backward-compatible API response if needed outside production semantics;
- narrow/success return projection;
- parameter hint projection for UI/debugging.

Forbidden stored authority:

- parameter evidence as separate merge truth;
- return summaries as separate merge truth;
- narrow returns as separate merge truth;
- function type cache as separate merge truth;
- captured mutation helper summaries with custom merge;
- legacy compatibility view written back into facts.

The final flash migration should delete duplicate stored channels in the same
change that introduces the canonical product.

### Performance Consequences

The boundary summary calculus should make interproc faster because summaries
become smaller and more stable.

Expected wins:

- one summary hash/equality path instead of multiple channel comparisons;
- no function-type projection during convergence unless a caller asks for it;
- no return narrow projection during convergence unless a query asks for it;
- no-op joins can reuse previous summary components;
- snapshot inputs update only changed canonical summaries;
- wrapper forwarding remaps summaries instead of resynthesizing them;
- parameter-use graph summaries feed parameter evidence without AST rescans.

Rejected shapes:

- rebuilding all derived views on every merge;
- writing projections back into canonical facts;
- comparing function summaries by formatting types;
- widening by dropping entire summary families;
- adding iteration caps instead of domain widening;
- clearing caches manually to repair stale summary dependencies.

### Function Boundary Law Tests

The flash migration should add focused tests for:

- function type projection is deterministic from the same summary;
- parameter body obligation outranks call observation;
- call observation alone does not prove concrete callee contract;
- explicit `any` parameter does not become concrete from calls;
- zero returns differ from one nil return;
- unknown return survives merge with concrete return when unresolved;
- narrow/success return is derived from relation summary;
- wrapper forwarding preserves relation through explicit remap;
- captured field write and captured container mutation use same memory law;
- constructor field publishes only from constructor evidence;
- export summary does not include non-escaping locals;
- no-op summary join preserves equality and avoids snapshot rewrite;
- recursive function summary widens and converges without erasing all relation
  proof.

### Delta

A `Delta` is a completed analysis contribution to another scope or iteration.

Examples:

- function fact delta,
- parameter evidence delta,
- captured mutation delta,
- constructor field delta,
- relation summary delta.

Deltas are immutable. The store never lets a producer mutate canonical state in
place.

### Snapshot

A `Snapshot` is the immutable state observed by a query.

Snapshots are cache inputs. If a snapshot changes, dependent queries must
revalidate through Salsa or an explicitly documented cache invalidation rule.

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

The authority order is partial, not a simple global priority. For example:

- explicit annotation dominates inferred shape for assignment checking;
- hard branch proof dominates soft annotation for narrowing;
- body obligation dominates call observation for parameter contracts;
- explicit `any` remains dynamic top and does not become concrete because a
  later call expects concrete;
- unresolved `unknown` can be refined by proof, but cannot be silently replaced
  by unrelated precision.

This should become an explicit `EvidenceOrder`, not a set of local `if`
statements.

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

## Domain Invariant Ledger

Each domain needs invariants that can be tested independently from the full
checker. These are the invariants that should guide the flash migration.

### Value Domain Invariants

- `unknown` means unresolved evidence and must not be silently dropped at
  return, branch, table, or relation joins.
- `any` means dynamic top and must not satisfy concrete contracts without an
  explicit proof, guard, schema, or cast.
- `nil` is a Lua value; absent field is structural absence; optional field is a
  type-level allowance for absence/nil depending on context.
- soft evidence is lower authority than hard evidence, but `nil` alone does not
  erase a soft structured shape.
- open row-tail field access produces row-tail evidence; closed missing field
  does not.
- table top absorbs table-like precision only in domains where table-likeness is
  the intended upper bound, not as a general precision eraser.

### Memory Domain Invariants

- every fact has exactly one canonical location;
- child-path facts outrank parent-derived fallback evidence for the same path;
- alias replay preserves identity and dominance;
- mutation replay preserves operator kind;
- nil overwrite and field deletion are represented explicitly;
- branch-local mutation does not leak unless control-flow dominance proves it.

### Relation Domain Invariants

- tuple/path relations are first-class facts;
- relation facts survive assignment, wrapper forwarding, and module export only
  when slot/path identity is preserved;
- relation narrowing is bidirectional only when the relation declares it;
- a guard helper such as `is_nil` can apply a relation but cannot invent one.

### Effect Domain Invariants

- effects are summaries, not post-hoc type rewrites;
- effect application goes through transfer;
- captured effects preserve location, operator kind, and provenance;
- termination effects affect reachability before value queries;
- external contract effects are typed inputs, not hardcoded checker behavior.

### Parameter Evidence Invariants

- call observations are weaker than body obligations;
- body obligations are inferred only from actual body demand;
- source annotations remain authoritative;
- soft annotations can refine but not override hard proof;
- recursive parameter evidence widens at SCC/interproc boundaries only;
- function-fact params and parameter evidence use the same evidence order.

### Return Summary Invariants

- tuple arity is explicit;
- nil padding is explicit;
- unknown return evidence is not bottom;
- relation summaries travel with tuple summaries;
- recursive container growth has one widening policy;
- narrow summary is derived from solved flow facts, not a second stored truth.

### Interproc Facts Invariants

- producers emit immutable deltas;
- store merge uses `FactsDomain.Join`;
- fixpoint boundary uses `FactsDomain.Widen`;
- equality compares canonical state only;
- derived views are not write targets;
- snapshot inputs mirror canonical read state exactly.

## Domain Interaction Protocol

The product-domain design is only useful if packages interact through a small
set of verbs. These verbs are the mental model for the future implementation.

```text
Syntax/Graph -> Instruction -> Transfer -> Domain operation -> AbstractState
AbstractState -> Query -> Answer
FunctionResult -> InterprocDelta -> FactsDomain -> Snapshot
```

### Transfer

Transfer applies one semantic instruction to one abstract state.

Allowed:

- read the instruction payload,
- ask a domain for local semantic operations,
- produce a new abstract state.

Forbidden:

- scan unrelated AST,
- publish interproc facts,
- mutate Salsa inputs,
- call compatibility projections,
- repair old facts.

If a transfer needs a type meaning question, it asks `ValueDomain`. If it needs
a location question, it asks `MemoryDomain`. If it needs a correlation question,
it asks `RelationDomain`. It does not inline those laws.

### Domain Operation

A domain operation defines what a fact means and how it combines.

Allowed:

- normalize owned values,
- compare owned values,
- join owned values,
- meet or refine owned values,
- widen owned values,
- answer pure owned-domain predicates.

Forbidden:

- depend on source syntax,
- depend on checker phase order,
- allocate hidden facts in another domain,
- read mutable store state,
- perform query invalidation.

Domain operations must be deterministic and law-testable without constructing a
whole checker.

### Abstract State

`AbstractState` is the one mutable semantic product during analysis.

Allowed:

- hold domain components,
- combine components through their domains,
- expose read-only query views after solving.

Forbidden:

- keep shadow facts that duplicate domain-owned facts,
- hide a second mini solver,
- let equality normalize,
- let queries write analysis evidence.

### Query

Queries answer questions against solved state.

Allowed:

- read state,
- memoize performance-only answers keyed by immutable input,
- project final user-facing answers.

Forbidden:

- create new evidence,
- change convergence,
- backfill facts into the store,
- call `Join` or `Widen`.

If a query discovers that useful information is missing, the correct response is
to add a transfer/effect/domain fact that produces it before solving. The query
must not become a hidden analysis phase.

### Publication

Publication converts a solved function result into an immutable interproc delta.

Allowed:

- summarize returns,
- summarize parameter obligations,
- summarize captured effects,
- summarize relations,
- emit deltas.

Forbidden:

- merge deltas directly,
- reconcile legacy channels,
- mutate existing facts,
- apply caller-specific preferences.

The only writer of canonical interproc state is `FactsDomain`.

### Snapshot

Snapshotting wires canonical facts into Salsa inputs.

Allowed:

- copy canonical facts into inputs,
- invalidate dependent queries through Salsa dependency tracking.

Forbidden:

- normalize,
- widen,
- infer,
- drop fields for compatibility,
- reconstruct projections that were not canonical facts.

Snapshotting is cache plumbing. It is not part of type semantics.

## Layering And Import Rules

The final code should make illegal designs difficult to express. Package
dependencies should encode the semantic architecture.

### Domain Packages

Domain packages may import:

- low-level type structures,
- subtype/query primitives,
- small immutable domain-local helper packages.

Domain packages must not import:

- AST packages,
- flow builders,
- checker store,
- Salsa database handles,
- diagnostics emitters,
- compatibility view builders.

Reason: a domain is a pure algebra over facts. If it can see syntax or mutable
store state, local helper logic will grow back.

### Memory And Location Packages

Memory/location packages may import:

- symbol/location identity,
- type values needed to represent field and container facts,
- relation keys where tuple/path identity must be preserved.

They must not import:

- call checking,
- interproc store,
- return inference,
- diagnostics formatting.

Reason: every producer must use the same path identity rules. No producer should
construct its own equivalent of "field path", "tuple slot", or "receiver self".

### Transfer Packages

Transfer packages may import:

- normalized checker IR,
- abstract state,
- domain set,
- memory/location model.

They must not import:

- old fact bridges,
- compatibility projections,
- checker diagnostics as control flow,
- global interproc store mutation.

Reason: transfer is the executable abstract semantics for one instruction. It
can create deltas inside state, but publication happens later.

### Store And Pipeline Packages

Store/pipeline packages may import:

- domain interfaces,
- abstract interpreter engine,
- Salsa database handles,
- diagnostics/reporting.

They must not implement:

- truthiness laws,
- soft/hard evidence ordering,
- return tuple relation semantics,
- path dominance rules,
- recursive type widening.

Reason: orchestration controls when analysis runs. Domains control what analysis
means.

### Query Packages

Query packages may import:

- read-only solved state,
- Salsa query APIs,
- pure domain predicates used for answering.

They must not import:

- mutable transfer state,
- publication writers,
- domain normalization writers.

Reason: a query can be cached aggressively only when it is pure.

### Test Packages

Tests should mirror these boundaries:

- domain law tests construct only domain values,
- transfer tests build small IR fragments and inspect abstract state,
- solver tests check convergence and widening,
- replay tests validate production programs,
- negative tests prove that convenience broadening did not happen.

Tests that require a whole checker to prove a simple domain law are a signal
that the domain boundary is still too implicit.

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

There should be no separate ad hoc policy for "parameter evidence" versus "function
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

### Effect Inference

Scope:

- built-in and manifest call effects,
- assertion/predicate refinements,
- table and container mutations,
- callback invocation effects,
- termination and non-returning calls,
- return tuple relation attachment,
- captured mutation summaries,
- external contract effects.

Authority:

```text
EffectDomain
MemoryDomain
RelationDomain
TerminationDomain
```

Rules:

- an effect is an abstract transfer summary, not a postflow patch;
- applying an effect must produce the same state change as inlining its
  corresponding transfer instructions would produce, up to the abstraction;
- effect summaries preserve target locations, tuple slots, operator kind,
  dominance, and provenance;
- effects that refine values must emit relation/value constraints through the
  owning domains;
- effects that mutate memory must emit memory mutations through the memory
  domain;
- effects that terminate execution must update reachability before any value
  query observes the post-call state;
- callback effects are higher-order summaries and must be applied at the call
  edge that invokes the callback, not at publication time;
- external effects are typed inputs to the domain, not hardcoded names in call
  checking.

Wrong effect inference shapes:

- "after this call, rewrite argument type" in call checking;
- "after this function, patch captured fields" in interproc merge;
- "if function name is `test.is_nil`, narrow slot" outside relation/effect
  transfer;
- "if table mutator is seen later, replay as generic container mutation";
- "if global harness fails, add a special accepted shape".

Correct effect inference shape:

```text
call instruction
  -> resolve effect summary
  -> EffectDomain.Apply(summary, state)
  -> MemoryDomain/RelationDomain/ValueDomain/TerminationDomain operations
  -> new AbstractState
```

Effect inference must be compositional. A user-defined wrapper around an effect
should publish the same kind of summary that the built-in effect uses, so callers
do not need wrapper-specific logic.

### Inference Soundness Boundary

The checker should infer every property that is proven by:

- source annotations,
- reachable transfer facts,
- memory/path identity,
- relation facts,
- effect summaries,
- interproc summaries,
- declared external contracts.

The checker must not infer a property from:

- the type expected by a later failing call,
- most callers preferring a shape,
- `any`,
- absent evidence,
- a compatibility projection,
- a cache hit whose input identity is incomplete.

This boundary is the core soundness rule:

```text
Expected type is a constraint to check against evidence.
It is not evidence unless a declared contract explicitly says so.
```

Contextual typing is still valid, but it must be represented as evidence with
provenance. For example, a table literal checked against an expected record can
receive contextual field types at the literal boundary. A dynamic payload flowing
through `any` cannot acquire those field types because a callee wanted them.

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
- parameter-evidence joins,
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
they appear as local helpers in `returns`, `paramevidence`, `flow`, `synth`, and
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

The previous `candidateRefinesFunctionParam`,
`typeRefinesTableKeyByTruthiness`, `preferConcreteOverSoftType`, and related
return-package functions are being collapsed into domain-owned operations.
Parameter-specific pieces now live in `domain/paramevidence`; the remaining
function-fact merge should move to `domain/functionfact`.

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

Current state after the first domain slice:

- merge/canonicalization policy lives in `compiler/check/domain/paramevidence`;
- return SCC inference and interproc postflow still collect observations, but
  they call the domain to merge them;
- remaining work is to separate collection orchestration from the pure domain
  surface where it improves readability without adding a bridge.

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
| function param-slot refinement | `domain/paramevidence` plus `domain/value`, called by `returns/join.go`, `widen.go` | `domain/functionfact.ParamSlotDomain` delegating value refinements to `domain/paramevidence`/`domain/value` |
| return-vector merge/repair | `compiler/check/returns/join.go` | `domain/returnsummary` |
| table-top absorption | `domain/paramevidence` | `domain/paramevidence` plus value-domain classifier |
| soft vs concrete evidence | `typ/soft.go`, `domain/value`, return overlay | `domain/value` evidence policy |
| open-record row-tail merge | `types/typ/policy.go` | `domain/value` row-shape policy |
| path/query/alias identity | `constraint`, `flowbuild/path`, `flow/pathkey` | `memory` |
| table/container mutation replay | `nested`, `returns`, `flowbuild`, `flow` | `memory` mutation domain |
| error-return convention | `erreffect`, call/return inference | `domain/relation` |
| effect inference | `effects`, `erreffect`, `flowbuild`, `nested`, `returns` | `domain/effect` |
| body parameter contracts | `infer/return`, `flowbuild/assign` | `domain/paramevidence` |
| Salsa snapshot inputs | `store/snapshot_inputs.go` | keep in store, but document as cache boundary |

## Worked Consolidation Examples

### Table-Key Truthiness Refinement

Previous smell:

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

## Dataflow State Machine

The checker should have one visible state machine.

```text
Unbuilt
  -> GraphBuilt
  -> IRBuilt
  -> Solving
  -> Solved
  -> Inferred
  -> Published
  -> Snapshotted
```

### Unbuilt -> GraphBuilt

Input:

- source AST,
- parent scope,
- manifest environment.

Output:

- immutable graph bundle.

No type-domain merge is allowed here.

### GraphBuilt -> IRBuilt

Input:

- graph bundle,
- declared type environment,
- known effect specs.

Output:

- transfer program.

This stage may observe syntax and produce instructions. It may not decide
fixpoint policy.

### IRBuilt -> Solving

Input:

- transfer program,
- initial abstract state,
- domain set.

Output:

- evolving abstract state.

All state changes go through transfer and domain operations.

### Solving -> Solved

Input:

- worklist convergence,
- loop widening if needed.

Output:

- solved abstract state plus query view.

No interproc publication happens before this state.

### Solved -> Inferred

Input:

- query view,
- function body,
- relation/effect summaries.

Output:

- function result and interproc delta.

Inference reads solved state. It does not create another path-sensitive solver.

### Inferred -> Published

Input:

- immutable interproc delta.

Output:

- canonical fact product after join or widening.

Only `FactsDomain` may combine this data.

### Published -> Snapshotted

Input:

- canonical fact product.

Output:

- Salsa snapshot inputs and dependent query invalidation.

No semantic repair is allowed here. Snapshotting is cache wiring only.

## Nested Fixed-Point Model

The final checker has several fixed points, but they should all use the same
domain vocabulary. The existence of multiple schedules does not justify
multiple semantic models.

### Level 0: Pure Graph Summaries

Graph summaries are not fixpoints over types. They are immutable facts about
syntax and binding:

- parameter uses,
- return sites,
- local function edges,
- call sites,
- mutator sites,
- captured path mentions,
- normalized transfer instructions.

They can be cached by graph identity because they do not read interproc facts or
solved flow state.

### Level 1: Intraprocedural CFG Fixpoint

The local solver computes:

```text
CFG x TransferProgram x InitialState -> SolvedState
```

Convergence boundary:

- CFG joins use `AbstractState.Join`;
- loops use the relevant domain widen only when a loop-carried component grows
  past the domain's finite-height fragment;
- dead/unreachable paths update termination/reachability before value queries.

Forbidden:

- AST rescans during solve,
- producer-specific joins,
- query-time narrowing that writes state,
- loop-specific precision hacks outside domain widening.

### Level 2: Local Function SCC Fixpoint

Local functions inside a graph can be mutually recursive. The final model should
treat their summaries as another domain product:

```text
FunctionSummary =
  Parameters
  x Returns
  x Relations
  x Effects
  x Captures
```

Convergence boundary:

- recursive calls read the current SCC summary through the function fact domain;
- each function body emits a new summary delta;
- SCC join/widen uses the same parameter, return, relation, effect, capture,
  and memory domains used elsewhere;
- when the SCC stabilizes, the solved summaries become ordinary evidence for
  the enclosing function analysis.

This replaces "return overlay", "preflow synthesis", and "local function
snapshot repair" as separate semantic concepts. Those may remain as scheduling
or performance techniques, but not as separate laws.

### Level 3: Interprocedural Fixpoint

The outer fixpoint computes canonical facts across function/module boundaries:

```text
InterprocPrev + all FunctionResult deltas -> InterprocNext
InterprocPrev' = FactsDomain.Widen(InterprocPrev, InterprocNext)
```

Convergence boundary:

- producers emit immutable deltas;
- `FactsDomain` is the only merge/widen authority;
- no producer reads its own just-emitted delta except through the declared
  current-iteration overlay contract;
- equality checks canonical state only;
- snapshot inputs are updated only after the canonical product changes.

Iteration caps are diagnostics, not semantics. If convergence requires raising a
cap for normal programs, the relevant `Widen` is missing or too precise.

### Level 4: Incremental Revalidation

Salsa does not define type semantics. It revalidates query results after inputs
change.

Required dependency shape:

```text
FuncResultQ
  reads GraphSummaryQ
  reads Manifest/Input queries
  reads SnapshotInputs
  reads TypeQuery caches
  computes local fixed points
  publishes deltas
```

When a snapshot input is unchanged, dependent results should revalidate without
re-solving. When a graph summary is unchanged, function queries should not rescan
the AST to rediscover it. When a type-query cache hits, it should only avoid
structural recomputation; it must not mask missing checker dependencies.

### Fixed-Point Proof Obligations

Each level needs a proof surface:

- finite input identity,
- monotone transfer or documented approximation,
- explicit join/widen boundary,
- stable equality without repair,
- deterministic publication,
- cache invalidation by immutable dependency.

Performance and soundness meet at these obligations. A cache that is missing a
dependency is unsound. A widen that erases too much precision causes false
positives. A join that keeps rebuilding equivalent maps causes unnecessary
invalidations.

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

### Cache Placement Decision Model

Use Salsa when:

- the computation is pure,
- the inputs are immutable identities,
- dependency tracking can precisely invalidate dependent queries,
- the result is reused across functions, modules, or fixpoint iterations,
- recomputation is more expensive than dependency tracking.

Use a per-function local cache when:

- the computation is hot inside one solve,
- the cache key is a small local identity,
- the result is invalid after the current function solve,
- Salsa dependency tracking would be more expensive than recomputation.

Use no cache when:

- the operation is a cheap domain primitive,
- the input is already interned,
- the allocation is caused by poor ownership rather than repeated work,
- correctness would require observing mutable phase order.

Do not use a pool until:

- the allocation site remains hot after domain consolidation,
- ownership of each pooled object is single-phase and obvious,
- tests prove no retained result can observe a reused object,
- profiling shows the pool wins after synchronization and clearing costs.

The main expected Salsa gains are:

- graph-indexed parameter-use summaries instead of AST rescans,
- function-result queries keyed by graph and parent scope,
- pure type/operator queries,
- shape classification for large recursive types if profiling proves reuse,
- canonical interproc snapshots as inputs instead of manually invalidated maps.

The main non-Salsa gains are:

- domain operations that avoid rebuilding maps for no-op joins,
- path/location interning,
- copy-on-write fact vectors,
- removing compatibility projections from hot publication paths,
- making equality structural instead of repair-driven.

### Concrete Salsa Wiring Plan

The final design should classify every current cache and summary producer before
implementation. The goal is not "put everything in Salsa". The goal is exact
incremental boundaries and no hidden semantic cache.

| Current Component | Final Role | Cache Kind | Owner |
|---|---|---|---|
| `api.FuncKey{GraphID, ParentHash}` | function analysis identity | Salsa query key | pipeline/analysis engine |
| `FuncResultQ` | analyze one function under one parent scope | Salsa query | analysis engine |
| `snapshotInputs.facts` | canonical interproc fact snapshot | Salsa input | store/facts domain boundary |
| `snapshotInputs.refinements` | function refinement/effect snapshot | Salsa input | effect/refinement boundary |
| `snapshotInputs.constructorFields` | constructor field snapshot | Salsa input | memory/constructor boundary |
| `types/query/core.Engine` | pure type operations | query engine cache | type-query layer |
| `types/flow.ProductDomain` | branch-local narrowing algebra | ephemeral domain state | abstract state / flow domain |
| `paramevidence.collectParamUses` | body-demand summary | graph-derived Salsa query | graph summary layer |
| `ProjectHintsToParamUse` | parameter evidence projection | domain operation over cached body summary | parameter domain |
| `PreCache` / `NarrowCache` | repeated expression synthesis inside one solve | per-function local cache | transfer/query phase |
| `FunctionTypeCache` | local function specialization during one solve | per-function local cache unless key is immutable | function analysis |
| `StableFunctionSnapshot` | read canonical function fact snapshot | Salsa query/input read, not ad hoc map | function fact domain |
| flow solution narrow caches | repeated solved-state query | solved-state local cache | query view |
| path suffix/root caches | identity interning | local/global intern cache if immutable | memory/location layer |

This table is a migration contract. If a component does not appear here or in a
successor table before coding, adding a cache for it should be rejected.

### Query Dependency Contract

`FuncResultQ` must read all semantic dependencies through tracked inputs or
tracked pure queries.

Required reads:

- graph bundle by `GraphID`,
- parent scope by `ParentHash`,
- canonical interproc facts by `GraphKey`,
- function refinements/effects by owning symbol key,
- constructor fields by owning symbol key,
- manifest/module environment through manifest inputs,
- graph-derived body summaries through graph summary queries,
- pure type operations through the type-query layer.

Forbidden reads:

- mutable `InterprocPrev` maps without snapshot input tracking,
- current-iteration `InterprocNext` except through the canonical overlay input
  contract,
- ad hoc stable snapshot maps inside synthesis,
- source AST rescans for reusable graph summaries,
- global variables whose mutation does not bump a tracked input.

When a function reads a fact for a graph or symbol, the query database must know
that dependency. When the fact does not change semantically, the input should not
be rewritten. This gives both correctness and performance: no stale result, no
unnecessary invalidation.

### Snapshot Update Protocol

The store should be the only bridge from fixpoint state to Salsa inputs.

```text
producer emits InterprocDelta
  -> FactsDomain.Join/Widen into InterprocNext
  -> iteration boundary computes canonical InterprocPrev
  -> compare canonical old/new with structural equality
  -> set only changed snapshot inputs
  -> Salsa revalidates dependent FuncResultQ entries
```

Required properties:

- `setFacts` receives canonical facts only;
- equality is structural and does not normalize;
- empty facts are represented explicitly enough to clear stale inputs;
- per-symbol inputs are used only for facts whose key is truly symbol-local;
- parent-scoped facts use `GraphKey` or `SymbolKey`, not raw `SymbolID`;
- current-iteration overlay is either part of the canonical input contract or is
  not visible to `FuncResultQ`.

This avoids manual cache clearing as a correctness mechanism. Clearing may still
exist as a memory-pressure tool, but a correct result must not depend on it.

### Graph Summary Queries

Several expensive operations are currently repeated because syntax-derived
summaries are computed by the consumer. These should become graph summary
queries.

Recommended summaries:

- parameter-use summary by `GraphID` and function symbol,
- return-site summary by `GraphID`,
- local function/call graph summary by `GraphID`,
- table mutator call summary by `GraphID`,
- key-collector summary by `GraphID`,
- captured variable/path summary by `GraphID`,
- normalized transfer program by `GraphID` plus declared environment identity.

These queries read immutable graph/source data and produce immutable summaries.
They do not read interproc facts and they do not infer types. The analysis query
then combines those summaries with parent scope and interproc snapshots.

### Hot Local Cache Contract

Some caches should remain local because they are only useful during one solve.

Local cache keys must include:

- phase (`declared`, `preflow`, `narrow`, or final query),
- expression identity or normalized instruction identity,
- CFG point,
- parent scope identity when the answer can depend on scope,
- solved-state token when the answer depends on flow facts.

Local caches must not:

- survive across `FuncResultQ` computations unless the key is fully immutable,
- contain mutable domain state,
- publish facts,
- suppress dependency tracking by reading snapshots behind Salsa's back.

This keeps hot expression synthesis fast without making it a second semantic
store.

### Type Query Layer Contract

The core type query engine is already the right kind of abstraction for
field/index/operator/subtype queries: pure inputs, stable type identities, and
memoized expensive structural work.

Final rules:

- checker domains may call pure type queries;
- type queries must not read checker store state;
- type query caches are performance-only;
- type query answers must be invalidated or keyed by all external type-provider
  inputs they depend on;
- domain law tests should not depend on query cache hit order.

This means Salsa does not replace `types/query/core`. Salsa coordinates checker
analysis dependencies. The type query engine owns repeated structural type
operations.

### Performance Proof Requirements

A performance correction is accepted only with a before/after profile or
benchmark that names the reduced work.

Required measurements for the flash migration:

- large-function checker benchmark,
- representative interproc convergence fixture,
- production replay wall time,
- allocation profile for hot joins and expression synthesis,
- cache hit/miss counters for `FuncResultQ` and graph summary queries,
- number of snapshot inputs rewritten per fixpoint iteration.

Expected improvements:

- fewer `collectParamUses` rescans,
- fewer repeated local function snapshot syntheses,
- fewer map allocations in no-op fact joins,
- fewer invalidated function queries after no-op fact updates,
- fewer expression synthesis calls during narrow/final query phases.

Regression rule:

```text
If a performance win comes from accepting less precise facts, it is invalid.
If a precision win causes repeated semantic recomputation, the cache boundary is
wrong and must be fixed before the flash migration lands.
```

## Weak Points To Fix In The Design

### 1. Domain Laws Are Not Named

The checker has laws such as:

- hard evidence dominates soft evidence,
- `unknown` in return summaries is unresolved runtime behavior,
- open record absent field means row-tail, not nil,
- nil field can satisfy optional absence in record subtyping,
- table-top can absorb precise table evidence in parameter evidence,
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
"parameter evidence" and "function param facts" that rediscover the same truthiness,
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

## Failure Taxonomy

Future regressions should be classified by failed domain responsibility, not by
the helper function that happened to produce the symptom.

| Symptom | Likely Owner | First Question |
|---|---|---|
| guarded field still nilable at call site | `RelationDomain` or `MemoryDomain` | Did the guard create a path relation for the same location queried by the call? |
| error-return refinement does not affect value slot | `RelationDomain` | Was the tuple relation preserved through return assignment and wrapper forwarding? |
| external dynamic value passes concrete parameter | `ValueDomain` or `ParameterEvidenceDomain` | Did `any` get treated as proof instead of dynamic top? |
| unknown disappears from return summary | `ReturnSummaryDomain` | Did join/widen erase unresolved evidence? |
| nil field write behaves like absent field | `MemoryDomain` | Was nil overwrite represented as a value fact instead of structural deletion? |
| closed missing field behaves like open row-tail | `ValueDomain` or `MemoryDomain` | Was openness carried on the record/map component being queried? |
| table insert lost before iteration | `MemoryDomain` and `EffectDomain` | Was mutation replay attached to the canonical child location and operator kind? |
| recursive type keeps growing | owning domain `Widen` | Is growth bounded at the correct SCC/fixpoint boundary? |
| result changes after no semantic input changed | Salsa/cache layer | Is a cache keyed by mutable state or phase order? |
| result does not change after facts changed | Salsa/cache layer | Did the query read the canonical snapshot input that changed? |
| lint clears by accepting too much | `ValueDomain` or assignability boundary | Which negative test proves the new acceptance is sound? |
| repeated performance hot spot after caching | domain/query boundary | Is the computation duplicated because the owner is unclear? |

Classification rule:

```text
If a symptom requires reading three unrelated helpers to understand why it
happened, the domain model is still wrong.
```

The fix should move the law to the owner, delete the scattered helpers, and add
domain law tests plus one production-shaped replay test.

## Traceability Matrix

Every high-value behavior should be traceable from syntax to proof.

| Behavior | Producer | Canonical Fact | Consumer | Proof |
|---|---|---|---|---|
| truthy field guard | condition transfer | path truthiness relation | call/type query | relation law + guarded-call fixture |
| `test.is_nil(err)` success branch | predicate effect transfer | tuple-slot relation constraint | value-slot query | relation law + error-return fixture |
| body demands parameter field | transfer over field read/use | parameter obligation | interproc fact join | parameter evidence law + SCC fixture |
| call observes argument type | call transfer | call observation | parameter evidence join | authority-order law + negative any fixture |
| table insert mutates array element | effect transfer | container element mutation | iteration query | memory law + dominance fixture |
| nil overwrite | assignment transfer | explicit nil value or deletion effect | field query | nil/absent law + record fixture |
| wrapper forwards returns | return transfer | tuple relation preservation | caller assignment | relation preservation law + wrapper fixture |
| imported dynamic payload | external contract transfer | `any` or `unknown` with provenance | assignability check | value law + negative concrete-param fixture |
| recursive local function | SCC solver | widened param/return evidence | function result query | widen law + convergence fixture |
| module export | publication | immutable interproc delta | dependent Salsa query | snapshot dependency test |

This matrix is not a test list by itself. It is the audit trail showing that a
behavior has one producer, one canonical representation, one consumer path, and
one proof family.

## Design Review Decision Tree

Every future rule should be classified before code is written.

### Is It About What A Type Means?

Examples:

- `unknown` vs `any`,
- open row tail,
- nilability,
- truthiness,
- soft evidence,
- table top.

Owner:

```text
ValueDomain
```

Reject if implemented in return inference, call checking, or postflow writer.

### Is It About Where A Fact Lives?

Examples:

- field path,
- dynamic index,
- alias target,
- tuple slot,
- captured mutation target,
- receiver `self`.

Owner:

```text
MemoryDomain / Location model
```

Reject if every producer computes its own path identity.

### Is It About How Facts Combine?

Examples:

- branch join,
- parameter evidence merge,
- return vector merge,
- function fact merge,
- recursive shape cutoff.

Owner:

```text
The domain that owns that fact family
```

Reject if implemented as a producer-specific helper.

### Is It About When Analysis Converges?

Examples:

- loop widening,
- local function SCC widening,
- interproc widening,
- recursive type growth.

Owner:

```text
Widen operation of the relevant domain
```

Reject if hidden inside equality, query, or local preference helpers.

### Is It About What A Call Does?

Examples:

- mutates a table,
- narrows an argument,
- returns `(value, err)`,
- terminates,
- invokes a callback,
- collects keys.

Owner:

```text
EffectDomain + RelationDomain + MemoryDomain transfer
```

Reject if modeled as a one-off postprocessing pass.

### Is It About Reusing Work?

Examples:

- graph summaries,
- parameter-use summaries,
- function result,
- type operator query,
- shape classification.

Owner:

```text
Salsa query or explicit local cache with named inputs
```

Reject if invalidation depends on call order or hidden mutable state.

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

## Flash Cutover Gate

The flash migration should be reviewed as one semantic cutover, not as a chain
of transitional accommodations. The cutover is ready only when the following
artifacts can be listed before coding starts.

### Deletion Map

For each old helper cluster:

- current file/package,
- semantic law it currently approximates,
- final domain owner,
- final API call site,
- tests that replace helper-specific tests,
- commit in which the helper disappears.

If a helper cannot be mapped to a domain owner, the design is incomplete. If it
maps to more than one owner, the fact representation is probably mixed and must
be split before implementation.

### Replacement Map

For each production call site:

- current call,
- final call,
- expected semantic output,
- changed cache dependency if any,
- changed diagnostic behavior if any.

The migration should not introduce "temporary" calls that are expected to be
removed later. A call site either moves to the final API or stays unchanged until
the cutover is ready.

### Proof Map

For each domain law:

- unit law test,
- one positive checker fixture,
- one negative checker fixture when soundness could be weakened,
- one replay/global-harness case if the law came from real code.

No proof should depend only on external lint going quiet. The suite must show
both the precision gain and the rejection boundary.

### Performance Map

For each expensive operation touched:

- current benchmark/profile location,
- final owner,
- expected cache key or no-cache reason,
- allocation behavior,
- invalidation story.

Performance work should favor fewer repeated analyses and fewer duplicated data
structures before object pools. Pools are allowed only after ownership is clear
and tests prove no fact lifetime can leak across checks.

### Cutover Rejection Rules

Reject the migration if it contains:

- compatibility authority,
- fallback repair,
- two writers for one fact,
- query-time publication,
- equality-time normalization,
- broad assignability introduced only to clear production code,
- new cache without an immutable input contract,
- new helper whose name describes a case instead of a domain law.

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

## Minimum Final-Shape API Sketch

This is not a transitional API. It is the smallest final surface that should
exist after the flash migration.

```go
// compiler/check/analysis
type Engine struct {
    Graphs GraphProvider
    Domains Domains
    Queries Queries
}

func (e *Engine) AnalyzeFunction(input FunctionInput) FunctionResult
```

```go
// compiler/check/flowstate
type AbstractState struct {
    Memory MemoryState
    Values ValueFacts
    Numeric NumericFacts
    Shape ShapeFacts
    Relations RelationFacts
    Effects EffectFacts
    Termination TerminationFacts
}

func (s AbstractState) Join(other AbstractState, d Domains) AbstractState
func (s AbstractState) Widen(next AbstractState, d Domains) AbstractState
```

```go
// compiler/check/transfer
type Instruction interface {
    Apply(state flowstate.AbstractState, d Domains) flowstate.AbstractState
}
```

```go
// compiler/check/domain
type Domains struct {
    Value ValueDomain
    Memory MemoryDomain
    Relation RelationDomain
    Effect EffectDomain
    Parameter ParameterEvidenceDomain
    Return ReturnSummaryDomain
    Function FunctionFactDomain
    Interproc InterprocFactsDomain
}
```

```go
// compiler/check/domain/interproc
type InterprocFactsDomain interface {
    Normalize(api.Facts) api.Facts
    Leq(a, b api.Facts) bool
    Join(a, b api.Facts) api.Facts
    Widen(prev, next api.Facts) api.Facts
    Equal(a, b api.Facts) bool
}
```

```go
// compiler/check/query
type View interface {
    TypeAt(point cfg.Point, loc memory.Location) typ.Type
    RelationAt(point cfg.Point, rel relation.Query) relation.Answer
    EffectAt(point cfg.Point, call CallSite) effect.Summary
}
```

The important part is not exact names. The important part is that:

- state is one product;
- transfer mutates only that product;
- domains own all combination;
- query is read-only;
- interproc publication is delta-based;
- no package owns a shadow merge policy.

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

## Review Checklist Before Coding

Before implementing the flash migration, each proposed package should answer:

- What domain or boundary object does this package own?
- What are the only mutable states in this package?
- Which operation is transfer, join, meet, widen, normalize, query, or publish?
- Which laws are tested at the package boundary?
- Which edge-case matrix rows does it cover?
- Which caches does it introduce, and what exact immutable inputs key them?
- Which old helper clusters will be deleted when this lands?
- Which production call sites will move directly to the final API?
- What negative tests prevent broadening `any`, erasing `unknown`, or treating
  absence as nil in the wrong domain?

If any answer is "handled by a fallback during migration", the design is not
ready. The next implementation must be flash migration, not coexistence.

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
