# Canonical Program and formal engine cut

Status: binding design.

## Goal

Build one formal, symbolic, modular abstract interpreter whose core has no Lua
domain knowledge and whose work is proportional to semantic demand.

The completed pipeline is:

```text
source
  → parse
  → bind
  → Program
      ├→ Rules → Solver → State → Queries/certificates
      └→ backend LIR → bytecode → interpreter
                    └→ MIR/native code → JIT
```

## Frozen target profile

The executable denominator is the parser-produced Wippy Lua profile: every
parser-reachable Lua 5.3 operation plus the parser's typed syntax and sealed
Wippy provider/actor extensions. Coverage means every valid structural
permutation in that closed profile has Program rows and generic executable
behavior. It does not mean accepting arbitrary manually assembled AST states
or silently unioning every historical registry declaration.

Source parsing is the sole public language-construction route. The exported
AST is a parser/lowering representation, not a second manually constructible
language. Canonical compile entrypoints accept source or a validated canonical
Program artifact. Arbitrary `[]ast.Stmt` compilation and manually fabricated
AST combinations are removed from the public runtime/compiler surface in the
Stage 3 cut. Parser-unreachable fields or nodes are either made parser-reachable
with defined semantics in the same vertical tranche or removed; they are never
counted as supported because a Go struct can be instantiated.

The target implements the metamethod alternatives for every parsed operator.
In addition to the existing arithmetic, comparison, concat, length, call, and
index/write fallbacks, Stage 3 implements the missing Lua 5.3 `__idiv`,
`__band`, `__bor`, `__bxor`, `__shl`, `__shr`, and `__bnot` paths. Primitive,
raw, and metamethod behavior remain distinct typed Program alternatives.

The enabled library surface is the intersection of the frozen runtime profile
and sealed host configuration. A declaration in an old analysis signature
registry does not make an operation available. Registry-only `load`,
`loadfile`, `dofile`, `io`, `os`, `package`, `coroutine.close`, and
`coroutine.isyieldable` declarations are removed unless their runtime behavior
is deliberately added as a complete target-contract tranche. This profile
does not add Lua 5.4 facilities merely because historical signatures named
them.

Dynamic code and raw executable injection are excluded authorities. Lua
`load`/`loadfile`/`dofile`, package loaders, raw `FunctionProto` construction
or injection, unchecked bytecode decode, and equivalent provider/host aliases
do not survive the cut. Source compilation produces Program, and stored modules
enter only through the canonical validated Program artifact/link path. VM
construction from backend output is private to that path.

Host callables, globals, modules, and providers may be registered only during
project construction under an exact typed provider contract. Project seal
freezes the external-operation and executable-entry relations. Post-seal
callable/global mutation and execution of an unsealed entry are unavailable.
Callbacks, retained callbacks, host transfer, suspension, resumption,
cancellation, and host/system-yield behavior are explicit typed boundary and
outcome relations.

Authored effects have one authority: a canonical Koka-style row from typed
source, module, or provider contracts. Concrete external causal relations
record callbacks, retention, mutation, suspension/resumption/cancellation, and
outcomes. Analyzed operational path, heap, escape, placement, ownership, or
typestate results are Factor conclusions and certificates; they are never
serialized back as provider authority.

There is one Program. Analysis CFG, WIR, source-program overlays,
engine-program inventories, path-string identities, and reconstructed source
relations do not exist.

The engine's public conceptual vocabulary remains:

```text
Term  Guard  State  Factor[T]  Rule  Solver  Query[T]
```

Program language handles such as body, call, allocation, field, effect
operation, and source span belong to `compiler/program`, not to the engine.
Implementation details such as partitions, roots, schedules, caches, and
equations are private and do not appear as additional public lifecycle nouns.
`Mu` is an annotation on a Program or compiled-equation SCC head; it is not a
second node graph, carrier, or public lifecycle object.

`Term` is the one compact tagged identity minted by an immutable Program shard
for an entity that can be a Factor subject. Its semantic identity is the
shard's complete semantic identity plus a dense shard-local tagged index.
Engine and domains use that exact identity unchanged; they never wrap, alias,
re-intern, or mint a second Term. A project link retains shard identity and
connects shards only through explicit boundary relations; it does not
re-intern shard entities into a project-global Term plane. The in-memory
representation may pack a dense project shard slot with the local index, while
hot body/domain storage indexes directly by the local dense component.

Raw packed bits are stable only while that exact shard namespace is retained.
Eviction/rebuild may choose a different representation but has the same
semantic `(shard, local)` identity; handle-bearing in-memory artifacts then
miss and rebuild rather than remap. Its Go package placement follows the final
dependency graph and is not an architectural law. A domain may instead use its
own typed key (for example a heap slot), but every source identity inside it is
still a Program-minted Term.

## Formal model

For each declared Factor `i`:

```text
A_i = K_i → V_i
```

`K_i` is a stable typed, totally ordered, hashable dependency-unit space.
`V_i` is a complete
abstract lattice with explicit equality, order, join, widening, and any
domain-supported narrowing. Its widening must prove stabilization for every
ascending iteration at `Mu`; an optional narrowing must provide a well-founded
rank that proves its descending phase terminates. These are executable domain
laws, not scheduler limits. A Factor must also prove that its reachable
abstract key support is structurally finite for a sealed Program, or provide a
canonical key abstraction whose map-support widening stabilizes at `Mu`.
Cardinality is never that abstraction. The total map is represented canonically and
sparsely relative to a Factor-declared default element: writing the default
removes the stored entry, so there is one representation of the same semantic
map. An unstored-key read still logs the root/key absence and is invalidated by
the first non-default write. An undemanded computation is a distinct
operational state, never a Factor value. A Factor whose key may not exist lifts
existence into its element/default law; the engine never assumes that storage
absence means either semantic top or nonexistence.

State-dependent partitions never appear as `K_i`. A relational domain chooses
a stable Program-derived variable pack as one key and places the complete
relation over that pack in `V_i`. A heap/key domain chooses a stable aggregate
or allocation root as one key and places its changing symbolic key partition
and slot assignment in `V_i`; join transports assignments through the
domain-defined sound common partition, and its widening proves stabilization
of both partition and contents at `Mu`. Cross-pack or cross-root information
moves only through declared Rules. The Factor key is therefore the atomic
read/invalidation unit unless the owning Factor declares a finer stable
dependency token together with a closure law proving that every affected token
is invalidated. The engine never assumes that individual variables,
equivalence classes, or range cells are independent map keys.

The whole abstract state is not a bag of independently guarded Factors. Let
`L` be the template-local Program locations of a body: source occurrences,
entry, and one exit for each declared outcome. Program seal constructs a
finite candidate universe of typed call, callback, resumption, handler,
module-initialization, and opaque-external relations. A candidate is not an
equation edge merely because it exists. `I` is the stable Solver-private set
of structural incoming-relation coordinates from that universe; it is not an
SCC quotient. A coordinate is never derived from a parent activation,
call-stack path, argument value, discovery order, activation count, or
capacity limit. The Solver maintains a monotonically growing active-edge
subset. Known structural relations seed it; domain call/dispatch Rules can
activate only a sealed candidate. SCCs, WTO order, and `Mu` heads derive
deterministically from the active compiled-equation graph and may be re-formed
when an added edge merges components. Coordinates remain stable and inactive
candidates create no state contribution, join, or premature widening. The
Solver coordinates are:

```text
C = ⨆_body (I_body × L_body)
Σ : C → G(A_1 × A_2 × ... × A_n)
```

For dynamic application, the sealed universe is the finite canonical product
of Program application occurrences and Program body/opaque-target handles
permitted by the target contract; seal need not materialize that product.
Activating one pair only materializes storage for its pre-existing conceptual
coordinate. This preserves linear sealing and finite structural convergence
without making argument values or an admission policy part of identity.

The Program location remains the sole source identity. Invocation identity is
Solver-private and never becomes a second Program coordinate. Separate
structural callers keep separate coordinates; equal observed boundaries may
share the compiled equation result through the semantic cache without merging
their States. Recurrence follows active equation backedges among those stable
coordinates and converges by the owning Factors' widening at the derived
`Mu`, not by creating contexts until a quota is full. Query roots select by
Program body/call/activation relations and an
abstract context predicate; Solver resolves that selector to private
coordinates. Query may instead explicitly ask for their sound join. No private
coordinate handle enters the public API.

Each occurrence location has one fixed phase: the joint state immediately
before that Program operation. Its Rules transfer that input to the
Program-declared normal or non-local successor locations; outcome exits are
terminal coordinates. Produced Values and conclusions are Program subjects in
the outgoing fiber. A Query for an operation input selects its occurrence; a
Query for a produced conclusion selects the declared successor/outcome and the
operation's Program subject. There is no parallel before/after point plane.

`G` is the canonical disjunctive completion over the finite
recurrence-scoped Program-decision atoms reachable at a structural invocation
coordinate. It is represented by a reduced symbolic decision form plus a
disjoint cover of feasible Guards. Each leaf owns one sparse vector of
immutable Factor roots. This joint fiber is required: value, heap, equality,
effect, escape, typestate, and placement-relevant facts that arose on one
alternative remain correlated.

At control confluence:

- incoming Guards are disjointized into a canonical cover;
- alternatives with the same Guard join component-wise;
- Rules execute leaf-wise: under Guard `g`, each compatible joint leaf is
  transferred separately and every Factor read comes from that same product;
  successor fibers merge only after transfer;
- only a Query or an explicitly correlation-insensitive domain operation may
  request the joined Factor projection across compatible leaves;
- the symbolic Guard form is reduced canonically; it never joins semantic
  alternatives merely because there are many of them.

Exact projection occurs at every structural death interface, not only at
`Mu`. Program records the real scope/consumption boundary for source Values and
Cells. Domain-owned Rules at that boundary perform any existential cleanup
inside their own opaque lattice and prove it by the ordinary Rule soundness
law; a domain uses an exact complete projection when it has one, otherwise it
retains the subject or explicitly marks its own precision loss. Guard removes
a decision only when no downstream Rule/registered Query reads it and
canonical reduction sees equal resulting joint products. A demanded
intermediate observation therefore keeps its subjects and decisions live.
The engine has no generic Factor-projection subsystem, and nothing is removed
because of leaf count.

Leaf-wise semantics does not require leaf enumeration. A Rule declares the
decisions and Factor dependency units it reads; the engine applies it once per
distinct read projection over the shared decision form and reuses the result
for every equivalent subgraph. Irrelevant decision subgraphs remain shared.
Worst-case exponential correlation among simultaneously live, observably
distinct values is inherent and reported honestly, but dead decisions, unread
decisions, and equal read projections do not multiply transfer work.

On a backedge to `Mu`, Guard applies a declared recurrence substitution:
completed iteration-local decisions are existentially discharged; predicates
that Program proves live across the recurrence interface are renamed to the
head's canonical parameters and stabilized by Guard's termination-proven
widening. The next evaluation of the same loop/recursive decision therefore
does not mean the same historical Boolean event. Factor and Guard widening
occur only at explicit compiled-equation `Mu`. Away from `Mu`, Guard reduction
and exact death projection preserve the registered observable denotation. No
operation widens because there are many leaves. A widened coordinate retains
precision-loss metadata; exact diagnostics and
backend certificates may not silently use a proof invalidated by it.

A reusable body result is the solution of its formal entry/outcome coordinates
in this same carrier,
not a program-scoped fact and not a private second fact plane. Conditional
existence is represented in the joint product and in the owning Factor's
support lattice.

No feasible leaf is the bottom/unreachable state. A Rule computes a successor
fiber from an input fiber: an exact assignment may strongly replace a key,
whereas an uncertain alias performs the domain's weak update. Confluence joins
successor fibers. Revocation writes the owning Factor's sound default/unknown
through a typed key operation and invalidates causal dependents; it is not an
anti-monotone deletion.

A domain Rule that derives a contradiction may return no current successor
leaf through its declared typed write result. Arbitrary Factor bottom does not imply global
unreachability: the owning domain must declare the contradiction Rule and its
reads. The prune is a derived, dependency-tracked transfer result; if an input
widens or a must-proof is revoked, invalidation recomputes the predecessor and
can restore the leaf. Pruned leaves do not confluence, publish diagnostics, or
produce evidence/certificates.

Every Rule denotes a monotone abstract transformer. Domain law tests establish
local soundness and monotonicity. Without widening, the abstract equation
system has a least fixed point. With widening, the Solver is not claimed to
compute that least abstract point: it computes a deterministic sound
post-fixpoint that contains the concrete least semantics. A domain may declare
a narrowing only with a well-founded termination measure; the Solver applies
it to stability after the widened state is stable, invalidates/rechecks all
dependents it changes, and verifies the same post-fixpoint obligation. Without
that proof the domain does not narrow. No transfer, Query, cache, cardinality
limit, or iteration cutoff may invent semantic truth.

Every cyclic component of a Solver generation's active compiled-equation graph
has an explicit `Mu` head before that component is evaluated. At Solve start,
the Solver seeds known static equation edges and validates them against the
project-sealed recurrence heads/interfaces; an unchanged static SCC references
that exact canonical `Mu` identity and the same discharged decisions/live
parameters rather than selecting a second head. The Solver then forms the
generation's active SCC/WTO schedule.

Activating a sealed candidate pauses and invalidates the affected region,
deterministically re-forms its SCC/WTO and `Mu` heads, and only then resumes
evaluation. If dynamic edges change a static component, the generation
atomically replaces its prior schedule annotation with the one deterministic
head/interface for the new active component; both heads are never active for
the same component. This includes cycles induced by source control, direct or
discovered calls/resumptions, module initialization, boundary transport, and
cross-domain reduction Rules at an otherwise acyclic source occurrence.
Program records the cycles it knows structurally; the Solver alone derives
remaining active Rule-dependency SCCs and assigns their heads. Compilation
produces only static equations/candidates. There is no implicit recursive
evaluator and no cycle hidden behind repeated Rule scheduling.

Cross-domain precision is an iterated reduced product. A domain owns its
Factor and primitive Rules. A reduction is also a monotone Rule, lives in a
domain child package that names the judgment it owns, reads its exact Factor
dependencies, and writes the one Factor that owns the resulting invariant.
Reductions may read more than one Factor when the invariant is genuinely
relational. They stabilize in the ordinary equation system after join,
Guard normalization, widening, narrowing, and revocation. The composition root only
declares Factors and Rules. It contains no semantic closure, authority ladder,
admission registry, priority, or one-shot reduction order.

### Canonical denotation

- Program denotes an exact transition/outcome relation over concrete machine
  states and Program locations, parameterized by the concrete interpretation
  of its sealed external-operation handles. A body denotes the least solution
  of that relation, parameterized by its formal boundary. An opaque external
  operation means nondeterminism in that concrete interpretation, not a
  missing Program edge.
- Factor `i` has abstraction/concretization `α_i, γ_i`; its order is semantic
  inclusion (`a ⊑ b` implies `γ_i(a) ⊆ γ_i(b)`). Its sparse default denotes the
  value of every unstored key.
- The product concretization is conjunction of all Factor observations in one
  joint leaf. For a disjoint guarded cover
  `D = {(g_j, a_j)}`, `γ_G(D)` is the union of concrete states satisfying
  `g_j` and `γ_A(a_j)`.
- Guard canonicalization preserves denotation exactly: if `ρ(D)` is the
  reduced symbolic form, `γ_G(ρ(D)) = γ_G(D)`. No cardinality-triggered Guard
  abstraction exists.
- For a registered observable projection `P`, structural-death projection
  uses ordinary domain Rules to abstract the concrete lifetime boundary.
  Exactness is claimed only for a Rule with a complete domain projection;
  otherwise its normal sound overapproximation is marked or the subject
  remains live. Guard then existentially removes only a decision absent from
  all downstream dependencies whose joint terminals are equal.
- For recurrence head `μ`, Guard transport `ω_G^μ` existentially eliminates
  decisions re-evaluated on the next visit, substitutes the finite live
  recurrence interface, is extensive and monotone, and stabilizes under
  repeated backedges. Its concretization contains relational composition of
  all represented visits. This is the only Guard abstraction beyond exact
  Boolean reduction and exact structural-death projection.
- For each Rule `r`, local soundness requires the concrete successors of
  `γ_G(D)` at its Program operation to be included in `γ_G(F_r(D))`. `F_r` is
  deterministic, monotone, and leaf-wise. A contradiction Rule proves that its
  concrete successor set is empty before returning no leaf.
- The abstract equation system is
  `X_c = Init_c ⊔ ⋁_{d→c} F#_{d→c}(X_d)`. An equation component that is
  acyclic in the current active compiled-equation/Rule-dependency graph
  evaluates once. Every cyclic active component has an explicit `Mu` head.
  Kleene iteration denotes its least
  abstract fixed point when it terminates without widening; the production
  Solver may instead widen the Guard/product at that head and must return a verified
  post-fixpoint `Y` whose concretization contains the least concrete solution.
  A termination-proven narrowing may improve `Y` to stability but must
  preserve that obligation.
- A Program boundary correspondence `σ` has domain-owned abstract entry/outcome
  Rules whose concretization contains the concrete call/module transition.
  Identity, composition, alpha-renaming, activation freshness/restoration, and
  Guard substitution commute with `σ` up to sound abstraction. Outcome
  instantiation is indexed by that same `σ`, so the summarized equation denotes
  only interprocedurally valid call/return and suspend/resume paths.
- The concrete body meaning is
  `Φ_b(p) = outcomes(lfp X. E_b(p, X))`. The compiled abstract body artifact
  is a locally sound `E#_b`; for abstract boundary `p#`, its verified solution
  overapproximates `Φ_b(γ(p#))`. It is symbolic because `p#` remains formal;
  specialization supplies an abstract boundary projection without rebuilding
  `E#_b`.
- A cache hit is valid only when every dependency token read by the prior
  deterministic equation has the same semantic value/version. It therefore
  returns the same abstract result. Cache eviction changes work, not `Φ_b`.
- For registered Query roots `Q`, the dynamic backward dependency closure must
  satisfy `π_Q(Solve_demanded) = π_Q(Solve_full)`. Both modes use the same
  sealed candidate universe and, after domain-driven edge activation reaches
  closure, the same rooted active edges, canonical SCC heads, widening
  operators, Guard normalization, and deterministic per-component order.
  Omitting dependency-disconnected components cannot perturb the rooted
  solution. Undemanded is not an
  abstract value, and demand/cache/evidence have no concrete denotation of
  their own.

Any implementation mechanism that cannot be placed in this denotation is
outside the design.

## Canonical Program

`compiler/program.Program` is the sole immutable semantic IR.
`compiler/bind` is the sole lexical authority. Program is built directly from
the AST and binder result.

Program records exact causes, never abstract conclusions:

- authored bodies, bindings, parameters, captures, globals, imports, exports,
  labels, source occurrences, and spans;
- exact evaluation order and all normal and non-local outcomes;
- values, mutable cells, allocations, reads, delayed write groups, and source
  order;
- allocation kind and structural origin; source record/field schema; symbolic
  size/capacity Values; initialization/publish order; every actor
  send/receive; and each explicit ownership, retention, suspension, or runtime
  allocation boundary supplied by the language/provider ABI;
- scalar versus multiple/open results and Lua list adjustment in assignment,
  calls, returns, table construction, varargs, and generic `for`;
- exact fields, literal keys, dynamic key values, and deep structural lenses;
- branches, short-circuit selection, loops, labels/gotos, suspension and
  resumption sites;
- closure origin, capture installation, direct callee evidence, method receiver
  insertion, arguments, results, and unknown/imported boundaries;
- structural activation lifecycle: which operation creates an invocation,
  closure environment, coroutine, continuation, or module initialization and
  which resume/re-entry operation restores an existing activation;
- static declarations, type parameters, recursive declarations, constraints,
  annotations, assertions, `typeof`, `keyof`, indexed and conditional types;
- authored/static effect schemas where the parser, provider signature, or
  module contract actually supplies them; concrete effectful operations and
  handler/resumption structure implied by supported Lua constructs;
- module/provider identity, initialization order, partial initialization,
  result packs, export surfaces, import cycles, and typed module boundaries;
- explicit `Mu` for exact source-control and direct-call recurrence.

An ordinary branch, loop edge, or goto is not renamed “continuation.”
Continuation identity exists only where the language/runtime can suspend,
resume, throw, yield, or otherwise re-enter at a non-local outcome. This keeps
the vocabulary semantic instead of generic.

Program does not contain:

- inferred types, abstract values, inferred value/heap aliases, inferred effect
  rows, abstract heap slots, escape, placement, ownership, typestate, diagnostics, or
  JIT decisions; source type aliases and exact binding/call relations remain
  Program facts;
- solver contexts, partitions, widening, caches, worklists, registers, bytecode
  PCs, ABI layout, deoptimization state, or runtime shape offsets;
- opaque `any` content, string encodings of paths/facts/effects, rule-kind
  codes, operand-role registries, or generic topology records.

### One IR, closed typed relations

“One Program” means one sealed semantic authority and identity, not one
universal instruction stream. Its physical representation is a product of
private dense typed table families sharing the same Program handles:

```text
Program =
    identity/order relations
  × local-operation relations
  × control relations
  × boundary/schema relations
```

These are storage ownership boundaries, not public lifecycle objects and not
independently buildable or serializable Programs. Cross-family edges are typed
foreign keys to the one owning row. A semantic relation is stored exactly once.
Nothing is copied, translated, wrapped, or re-interned between families.

The Program implementation may not define a universal `Node`, `Operation`,
`Kind + operands` row, optional-slot instruction record, `Operations()` stream,
generic visitor, runtime family/handler registry, reflection/`any` payload, or
central language-semantics switch. Family-local closed enums such as arithmetic
operators are permitted; they may not become a global operation-code taxonomy.

The sole unfinished builder is the final Program storage before seal, not an
intermediate model. Each lowering child receives a narrow concrete typed writer
for its owned relations. It cannot mutate another family, mint or wrap shared
identity, publish a fragment IR, install a callback, or add an alternate
builder. Seal validates referential integrity, freezes direct ranges/indexes,
links shards, and annotates exact recurrence; it never translates the typed
tables into a central executable graph.

Program exposes immutable typed dense ranges and direct lookups. Backend
family lowerers and domain Rule constructors consume only the ranges they
need. Neither scans a generic operation stream, dispatches through an
interface/registry, or reconstructs a private Program view. The root package
owns identity, storage, sealing, and generated contracts only; language
judgments remain in lowering children, domain judgments remain in domains, and
physical judgments remain in the backend/runtime.

One canonical Program schema is the source of the typed rows, concrete writers,
frozen views, validators, backend completeness obligations, and persistence
sections. Generated code is static and typed; it creates no runtime metadata
plane. Adding a language operation or boundary event changes the closed schema
and fails generation/build until lowering, sealing, generic backend behavior,
provenance, codec coverage, and semantic conformance all have an explicit
disposition. There is no default or fallback disposition.

### Program identity and key space

All shard-local Program/compiler identities are dense within their immutable
shard; project relation/shard slots are dense within one sealed Program link.
A Term is compact and deterministic for its shard. Reusing that exact shard
retains its Term namespace across project re-link; rebuilding an equivalent
shard preserves semantic `(shard, local)` identity but does not require the
same packed bits or object identity. Cross-module relations use only explicit
exported/imported boundary Terms. Runtime `Value`
representation and equality are tag-specific and are not covered by this
handle law. Strings are accepted only at source parsing and external
serialization boundaries.

Program still retains authored names, string literals, source text identity,
and exact source spans as immutable payload atoms/ranges for Lua debug and
reflection behavior. They are not semantic keys and are never parsed in an
analysis/backend hot path.

Shard seal canonicalizes every semantically exact Lua table key to one typed
static key value with a dense shard-local atom handle:

- dot field `t.x`, bracket field `t["x"]`, `{x = v}`, and `{["x"] = v}` point
  to the same string-key atom while retaining their source spelling/syntax;
- numeric literals normalize by the runtime's table-key equality law, so an
  integer and its equal integral-float form do not create different slots;
- exact boolean/string/number keys use typed atom variants. Nil and NaN never
  become stored-key atoms, but key validity is operation-specific: an ordinary
  read can observe absence and enter `__index`; an ordinary write can enter
  `__newindex`; only a primitive/raw table commit that requires a storable key
  takes the target Lua contract's invalid-key error outcome. Program preserves
  these alternatives instead of attaching one unconditional error to the key;
- a dynamic key remains only a Program Value. Program never consumes an
  inferred proof. An equality/heap Rule may later refine its own dynamic-key
  partition to the Program's same canonical literal atom or to an exact
  identity class, without formatting or parsing. Lua key equality is one
  closed law: integer and equal integral float coincide, strings compare by
  content, and every other valid non-scalar key compares by object identity.
  Array versus hash placement is never a semantic key class.

An atom's semantic identity is its typed normalized value under the frozen
target key-equality law, not its shard-local or project-link handle. Reusable
shard equations store the local atom plus its canonical typed payload.
Cross-shard boundary Rules compare/substitute that semantic value; project
linking may intern equal atoms for runtime speed but no equation, cache key, or
serialized artifact depends on the link-local representation. Project seal
validates cross-shard key equality and direct indexes; it does not mint a
second semantic atom plane.

A cell is one lexical/global/upvalue storage identity. A lens is one evaluated
member/index location:

```text
base Value
  + exact field atom
  | exact literal key
  | dynamic key Value
```

A read consumes a cell/lens and produces a Value. A write targets a cell/lens
after all required base/key/RHS evaluations have occurred. A deep access is the
structural chain of those read-produced Values and subsequent lenses; it is
never flattened to a textual path. The heap domain may derive exact-key slots,
finite symbolic key partitions, equality classes, array summaries, or an
unknown-key summary. The source Program retains the distinction so the domain
never reparses or reconstructs it.

There is one aggregate write operation, including writes to a currently shaped
record. The Program row distinguishes ordinary assignment from raw assignment
and declares the primitive and typed `__newindex` application/outcome
relations. It does not decide which dynamic alternative is feasible. Heap and
metatable Rules determine raw presence and feasible primitive/dispatch
alternatives under Guard; the backend performs any required runtime check.
For Lua dispatch, a physically declared shaped field whose current value is
nil is absent: it cannot suppress `__index` or `__newindex`. Only a proved raw
or no-dispatch path may keep a compatible present string field in the shaped
representation or treat a missing-field nil primitive write as a no-op.
Otherwise the ordinary metatable application and its
call/effect/escape/outcome edges stay visible. A raw/no-dispatch missing
non-nil or incompatible declared-field value changes the same semantic object
to an ordinary dynamic table.

That representation change preserves every source alias and all public
contents. Program does not acquire a second shape-transition node or identity.
Physical shape hashes, owner-local shape indexes, field offsets, aggregate
headers, tags, indirections, forwarding state, and array/hash placement are
backend artifacts. The runtime representation must make shaped-to-dynamic
widening preserve the observable aggregate identity and every alias without a
global live-reference scan; the backend/runtime design chooses the tag/header/
forwarding mechanism. Compatible private writes
remain direct writes. The target runtime must implement widening without a
global live-reference rewrite; the current target allocate-and-scan implementation
is removed in the Stage 3 runtime cut, not recorded as source semantics.

One canonical Values relation represents Lua result lists: an ordered fixed
prefix plus an optional open tail. Every consumer names that relation and one
of the language's exact adjustments:

- scalar/parenthesized use takes the first result and supplies nil if empty;
- every non-final expression in a list is scalar-adjusted;
- a final open producer may expand;
- a fixed target count truncates or nil-fills;
- an open consumer preserves the final tail.

Assignment, return, arguments, table array tail, vararg, and generic `for` all
use this relation. There is no second `ListShape` descriptor.

Field atoms and literal constants are canonicalized once at shard seal and
validated at project link.

Immutable shards and their sole project link carry the complete semantic
identity (source/reflection/module/provider/import-contract content plus
parser/binder/lowerer/sealer and target-contract digests). Equal retained shards
reuse work directly; eviction/rebuild misses rather than remaps. A shard seals
only wholly local source-control recurrence. Existing typed import,
module-initialization, resumption, and callback relations carry their own
entry/outcome correspondence and recurrence fields; project seal composes those
relations into the canonical heads/interfaces for every exact static
direct-call and module-initialization cycle, including wholly same-shard and
mixed cross-shard cycles, while preserving original Terms. Head selection is
canonical from semantic shard identity and local occurrence, never load order.
Static equations reference that annotation and are rejected if it mismatches.
They never select a second head; only the Solver adds dynamic edges or reforms
an active component. The project link is part of the one Program and does not
re-lower bodies, mint Terms, infer semantics, or create a second IR. Recurrence
fields stay on existing typed Program boundary relations; there is no `Port`
type/plane, summary, or deferred name lookup.

### Manifest persistence

The manifest phase is the canonical persistence boundary for precomputed
modules, not another semantic IR. The two construction paths meet at the same
immutable Program shard:

```text
source → parse → bind → Program shard
stored module artifact → validate/decode ─┘

Program shard → canonical encode → stored module artifact
```

After decode, project link, Rules, Solver, Queries, backend, and runtime cannot
observe which path produced the shard. There is no manifest-specific Term,
type/effect vocabulary, summary plane, engine Rule, backend operation, or
compatibility adapter.

One versioned canonical artifact contains:

- format/schema and target-language-contract digests; parser/binder/lowerer/
  sealer producer versions for the canonical shard core;
- the complete semantic shard identity, source/module/reflection identity,
  dense shard-local Term and atom tables with canonical typed payloads, spans,
  Program rows/direct indexes, explicit boundary/import/export relations,
  external-operation contracts, shard-local source-control `Mu`, and typed
  recurrence fields on every link-owned boundary relation;
- exact dependency identities for imported boundaries, providers/schemas, and
  module initialization;
- static declarations/types/effect schemas and body entry/outcome
  correspondences in their one canonical representations;
- optional derived cache sections: relocatable generic shard bytecode plus
  provenance under its backend/compiler/bytecode-ABI digest, and body-local
  context-parametric equations under their engine/Factor/Rule/widening
  versions. Portable equations contain only shard Terms, shard-local
  source-control `Mu`, and typed operands for existing boundary dependencies;
  they never contain a project SCC/head.

Domain/engine/backend identities never enter the canonical Program-shard core,
Term namespace, project link, or source-built/decoded Program equality.
Producer/dependency identity is scoped to the smallest section that actually
interprets it: backend/ABI for bytecode or physical artifacts;
engine/Factor/Rule/widening for equations. Proof/Query versions apply to
evidence and certificates only in their separate State-bound operational
caches.

Stored bytecode carries only shard-local typed boundary operands and becomes
executable solely through the typed, linear project linker under project-link,
backend, and ABI identity; it performs no name lookup or inference. Source and
decoded shards take that same step. Portable equations likewise contain no
cross-shard head: project binding creates only static equations/candidates.
The Solver alone creates a generation's active graph, dynamic edges,
SCC/`Mu`/WTO, read logs, values, and reverse dependencies. A project cache can
hold only the static binding under the complete project-link and
engine/Factor/Rule identity; mismatch discards and recomputes it. A
State/evidence/certificate/native cache is operational and State-bound. Stored
evidence and certificates must pass the same local-law, post-fixpoint,
constructive-proof, and dependency validation as fresh artifacts. Every absent
or invalid derived section recomputes from the canonical shard, never selects
another semantic path.

A context/input-specific solved State is never a portable module section.
Separately persisted State, evidence, certificate, or native-code operational
caches are discarded on any universal-validity mismatch.

The storage-only format uses canonical typed encodings, ordering, graph
backreferences, lengths, and digests; it forbids process pointers, link slots,
Go types/interfaces/reflection, paths, sentinels, and parsed strings. The core
digest excludes optional sections; each derived section has canonical bytes
under the core plus all producer/dependency identities. The outer directory is
canonical for a selected section set, so decode followed by encode reproduces
the complete container bytes for that set; different valid optional-section
sets may differ without changing shard semantic identity. Program owns its core
codec; a Factor that permits equation caching supplies its own typed element
codec and schema version to the existing composition root, not a second
registry.

Decode is iterative, total, bounds-checked, transactional, and constructs the
ordinary builders only; decoded wire structs never escape the persistence
package. It validates schema/producer/contract/ABI identities,
section digests, canonical ordering, all index/range/kind references, boundary
closure, equation dependencies, lattice-codec versions, and recursive
backreferences. The equation validator reconstructs the canonical
equation/Rule dependency digest from Program plus current Factor/Rule
declarations and accepts only an exact match. The generic-bytecode validator
checks Program provenance, control/value/list/outcome coverage, constants, and
operation lowering against the same backend semantics; safety verification is
additional. Both validators come from the same Program operation/Rule/lowering
declarations as their producers. Otherwise the sole compiler rebuilds and
compares the artifact. Malformed, unverifiable, unknown-version, or
resource-exhausting input publishes nothing; unavailable source in a
precomputed-only deployment fails explicitly. There is no fallback to an old
decoder or weaker analysis. Storage commits atomically; corrupt leftovers are
invalid. Storage addresses the shard core by its semantic digest and each
derived section/container by its own canonical digest. Schema replacement
rebuilds or rejects all old blobs with one live decoder. I/O/hashing remain
cold path, and decoded shards use the same dense local hot-path representation
as source-built shards.

### Program sealing

Sealing is structural and deterministic:

- validate all handles and relation cardinalities;
- resolve lexical references, labels, outcomes, and direct structural calls;
- freeze ordered relation ranges and direct indexes needed by Rules;
- build control predecessors/successors and exact occurrence ownership;
- at shard seal, compute source-control recurrence wholly owned by the shard
  while retaining exact direct-call edges and recurrence fields on existing
  import/module-initialization/resumption/callback boundary relations;
- at project seal, resolve those explicit relations, compute canonical SCC/`Mu`
  heads for every exact static project cycle, and compose the decisions
  re-evaluated on the next visit plus the Program predicate/value parameters
  that remain live, so Guard transport never guesses recurrence lifetime;
- retain unresolved/higher-order applications as exact application sites; the
  Solver, not Program, forms/reforms the combined instance/control SCC when a
  dynamic call/resumption edge is discovered;
- validate packs, lenses, captures, type/effect substitutions, and source
  anchors;
- hash the immutable Program and its module shards.

Every source operation family has a typed Program row keyed by its Occurrence
and body-local dense range. Domain construction iterates the exact typed range
once to instantiate Rules. The engine receives already-instantiated Rules and
never learns a relation kind; Rule application performs no scan. There is no
generic rule-kind/operand-role/topology code plane.

The Stage 1 target-language contract is the sole denominator that generates
and validates the finite executable operation matrix, builtin/provider
bindings, runtime-lifecycle rows, and conformance ledger. The matrix is
explicit and typed. It covers
literal/constant creation; every unary, binary, relational, logical, length,
concat, conversion, and truthiness operation; cell/lens read and write; all
table-constructor field modes; Values adjustment; closure/capture creation;
plain/method/dynamic/external calls; returns and every supported outcome;
branches, loops, labels/gotos; vararg and generic iteration; metatable/raw
operations; coroutine/handler operations; and module/provider initialization.
Any parser, builtin, provider, runtime-lifecycle, or ABI addition must extend
that contract and matrix or fail sealing—backend lowering never falls back to
the AST or a numeric operation code, and runtime scheduling may only introduce
observable lifecycle behavior declared by the contract.

Every operation controlled by the target Lua metatable contract has a closed
typed application relation in that same matrix; “metatable operation” is not a
catch-all row. The relation records primitive eligibility, raw-presence lookup,
the specified operand/fallback order, function or table delegation where the
operation permits it, argument and Values adjustment, normal/error/non-local
outcomes, and raw bypass behavior. This covers `__index` and `__newindex`
chains, `__call`, arithmetic/bitwise/unary/concat/length/comparison fallbacks,
protected metatable access, and every iteration or lifecycle metamethod
actually supported by the frozen language contract. Program declares the
candidate relations; metatable/value/heap Rules select feasible alternatives.
Delegation or callable chains that can recur enter the ordinary structural
application relation and compiled-equation SCC/`Mu`; neither lowering nor a
domain follows them with recursive host-language calls or a depth cap.

One project-sealed external-operation relation maps a provider/import binding
to a dense runtime operation handle, exact calling/Values convention, result
and outcome convention, module-initialization boundary, and every concrete
boundary event permitted by its typed provider contract. Those typed events
cover synchronous callback application; zero/repeated application;
callback/capture retention and later re-entry; continuation retention,
suspension, resumption, cancellation, and handler transfer; and the Values and
outcome correspondence for each event.

The contract is not an unordered event inventory. It seals one finite typed
control/causal relation: each event occurrence has its enabling condition,
normal and non-local successors, Values transport, activation
creation/restoration identity, retention/release ownership transition, and
whether provider return waits for callback completion. It distinguishes
one-shot from repeated resumption and synchronous return from retained
asynchronous re-entry. Repeated/re-entrant event edges carry structural
recurrence and therefore participate in the ordinary compiled-equation
`Mu`/SCC model. This is the exact Program structure used by typestate,
ownership, escape, placement, and effects; no domain reconstructs event order.

The generic backend consumes the same operation handle and calling/lifecycle
ABI. The Solver instantiates structural dependencies from these sealed event
relations; call Rules supply the possible bodies for callback Values, and
type/effect/heap/escape/ownership/placement Rules interpret the same contract.
An opaque external operation is one explicit maximally nondeterministic typed
control relation over the callbacks/Values it receives, including retention,
re-entry, suspension, mutation, and outcomes—not an absent edge or fallback
registry. There is no second intrinsic/provider registry and no backend name
lookup.

Actor/host transfer operations seal only their observable ABI relation:
success value contents at the transfer occurrence, aliasing within one
delivered graph, isolation from later sender mutation, capability preservation
or contract-declared loss, failure/rejection classes, and whether
cross-delivery identity is observable or explicitly unspecified. Retain,
promotion, serialization, move, clone, and COW are backend implementations
permitted only when observationally equivalent to that relation. Domains infer
escape, ownership, portability, mutation, lifetime, and eligibility facts;
backend/runtime lowering chooses the lawful implementation.

Labels target source positions, including empty positions before `end`.
Multiple labels may share a position. Goto carries its resolved
source-position edge and binder proof that it does not enter a live local.
Sealing propagates normal, return, break, and goto, plus only those
throw/yield/resume/cancellation outcomes introduced by a supported
source/provider/runtime operation, through every nested lexical sequence. A
terminal outcome never fabricates the skipped suffix, and normal fallthrough
enters exactly one tail.

For a construct that can actually suspend or handle a non-local outcome,
Program records one typed continuation relation: suspension occurrence,
outgoing Values, resumed input Values, outcome kind, target body/scope,
handler/resumer boundary, and the lexically reachable binding/cell/capture
environment. Effect and placement derive semantic survival from it; backend
liveness derives physical live slots/stack maps after lowering. Ordinary
structured control and goto do not use it.

Each call/module boundary owns one sealed typed correspondence covering
formal/actual Values, captures, receiver, arguments and open tails, every
outcome result pack, imported/exported bindings, type parameters, authored
effect-row variables, and structural activation/allocation sites. These
substitutions are over Program static/schema handles, never resolved inferred
types or effect rows. The correspondence names structural roots, not inferred
reachable heap. Domain boundary Rules consume this same
correspondence and the joint Factor product; no domain reconstructs it.

The supported parser grammar is authoritative. If it has no authored Koka
handler/row syntax, Program does not invent such source syntax: Koka rows are
the effect domain's abstraction of Lua operations and typed provider/module
contracts. Stage 3 first fixes any missing parser evidence required by existing
typed syntax, including asserted-parameter token identity, static annotation
arguments, and the source occurrence referenced by `typeof`.

Program does not perform general SSA conversion and does not fabricate phi
nodes. Expressions already have immutable value identities; mutation is
represented by cell/lens reads and writes. A syntactically immutable binding
may point directly at its value. Backend SSA and register allocation belong to
backend LIR.

Sealing is `O(V+E)` apart from sorting and one-time interning. A Rule never
scans a Program relation or builds a private index: every required relation is
available as a frozen direct range or lookup.

### Lowerer organization

The lowerer is vertical, not one monolithic horizontal package. Its production
children write directly into one unfinished Program builder:

```text
compiler/program/lower
  lexical
  eval
  store
  control
  call
  static
  module
```

The parent only sequences phases and seals. The children share dense AST/binder
handles through the Program's internal builder; they do not exchange shadow
IRs, adapters, string codes, or callbacks that decide language semantics. A
single syntax walk allocates source occurrences and invokes typed builder
operations; each child owns and validates the canonical relation ranges for its
semantic family.

Lowering laws explicitly distinguish declaration/initializer visibility,
ordinary `local f = function` from recursive `local function f`, implicit
method `self`, vararg binding, loop-local lifetimes, and function boundaries
for break/label/goto.

Black-box tests lower source and assert Program semantics. No test inspects Go
files, package imports, directory layout, registrations, or composition shape.

The generated coverage matrix is schema-derived and closed over the current
AST fields, not maintained only as a hand-written node list. For every AST
implementation it takes the finite product of every semantically relevant
enum, boolean, nullable, list-cardinality class, token role, child-node class,
and target-law equivalence class at one constructor step; each product member
must map to a typed Program row or an explicitly justified impossible state.
Unbounded scalar payload domains—identifier text, source strings,
integer/float values, and literal contents—are parameters of those structural
cases, never matrix axes. Generated/property tests quantify over arbitrary
valid payloads and boundary/exceptional representatives for parsing,
normalization, target key equality, numeric overflow/rounding, serialization,
and alpha-renaming.

Arbitrary list length and nesting depth are structural and are covered
inductively, not called payloads or enumerated as an impossible finite product.
Every recursive AST/list schema generates a base law and one law per recursive
constructor. Sequence lowering separately proves empty, prefix-step, and final
element behavior, preserving left-to-right evaluation, lvalue commit order,
scope, non-local tails, and final-open `Values` expansion. Recursive nesting
proves that composing a lawful child row preserves binding, spans, control,
correspondence, and structural `Mu`.

These are executable compositional proof obligations, not sampling claims.
The schema generator uses the exact canonical Program row declarations and
builders to construct partial rows whose recursive child positions are typed
symbolic holes. The checker for every base/constructor law validates
composition through the same generated row contracts used by ordinary
lowering. It may not define a fragment IR, alternate builder, operation
semantics, or second semantic switch. It must discharge row assembly, operand
correspondence, order, scope, tails, indexes, and `Mu` preservation for
arbitrary lawful substitutions into those holes; a missing or failed
obligation makes generation/sealing/conformance fail. Structural induction over
those checked constructors establishes the result for every finite source
tree. Generated property/fuzz tests still exercise arbitrary heterogeneous
sequences and depths, but are adversarial evidence rather than the universal
proof. Thus the one-step structural denominator is finite and exact without
presenting a finite sample as 100%. The following human-readable list is a
minimum digest, not the source of coverage:

- expressions: true/false/nil/number/string, vararg, identifier, dot/bracket
  access, table, plain/method/generic call, `and`/`or`, every relational,
  concat, arithmetic/bitwise and unary operator, function literal, cast, and
  non-nil assertion; both cast syntaxes, adjusted/unadjusted call/vararg,
  qualified methods, and simple/qualified source names are separate cases;
- statements: assignment, annotated local assignment, call statement, `do`,
  `while`, `repeat`, `if`/`elseif`/empty or present `else`, numeric and generic
  `for`, named/method function definition, return, break, label, goto, type
  alias, and interface declaration; numeric-for default/explicit step,
  generic-for binding arity/open iterator tail, and every function-name/
  receiver form are separate cases;
- static forms: annotation, primitive, optional, union, intersection, array,
  map, record/optional field, type parameter/constraint, function/variadic/
  asserting return, reference, generic application, typed literal, metatype,
  self, tuple, `typeof`, `keyof`, indexed access, and conditional type;
  interface extends/fields/optional fields/methods, qualified type-reference
  paths, every generic/type-parameter binding site, and annotations on
  primitive, array element, array, record field, local, parameter, and vararg
  are separate generated cases; readonly array/map/record, literal
  boolean/string/integer/float kind and target-law class, named/anonymous function
  parameters, variadic position, nil/empty/singleton/pack returns, and
  `asserts x` versus `asserts x is T` are separate field products;
- use contexts: condition, scalar, parenthesized scalar, non-final list, final
  expanding list, fixed/open assignment, call argument/result, return, table
  array/named/dynamic field, lvalue base/key/commit, numeric/generic loop
  header, annotation/static argument, nested closure/capture, module/provider,
  and every normal/non-local lexical tail; table bracket-literal versus dynamic
  key and final-open versus non-final array field are distinct cases.

Adding or changing an AST implementation or semantic field without adding its
operation row and every lawful field-product/use-context case makes generation
or seal testing fail. Coverage is measured against the reflected/generated AST
schema denominator, never against the cases the lowerer happened to register.

## Formal engine

The engine receives an immutable Program plus declared Factors, Rules, and
Queries. It owns no Program-like inventory and no Lua relation.

### Guard

Guard is the only public path predicate. Decisions and edges are Program
handles; the engine canonicalizes their predicates privately as a disjoint
decision representation. Conjunction, disjunction, conflict, entailment,
substitution, and alternative selection use an exact reduced symbolic decision
representation with structural sharing. Each compiled `Mu` declares which
decisions are re-evaluated and which predicate parameters are live across its
interface. Its Guard transport existentially discharges the former and
renames/widens the latter before confluence at the head. A cached body uses
canonical body-local Guards. Boundary Rules discharge or alpha-rename them
into an activation namespace when applying the formal result; instantiated
Guard identity includes Program decision provenance and substitution, so
unrelated bodies cannot alias while equal callers can still share the formal
solution. No hot operation sorts slices or allocates guard lists.

Guard partitions are joint-state alternatives. They are not repeated
independently inside every Factor key.

### Factor

A Factor owns:

- its typed key order/hash;
- element bottom/top, equality, order, join, widening, and optional narrowing;
- its semantic default and canonical sparse-storage law;
- immutable sparse storage and interning.

Body transport is not a set of independent Factor actions. Domain-owned
boundary Rules consume the Program's single typed correspondence and execute
over the same joint guarded coordinate as ordinary Rules. They declare every Factor
read/write, stabilize with reductions, and obey identity, composition,
alpha-renaming, monotonicity, guard preservation, and freshness laws. Adding a
Factor adds its owning boundary Rules; it never enlarges a universal summary
struct or creates a parallel carrier.

The heterogeneous hot state is a flat vector of small private root handles.
Typed Factor arenas resolve those handles. It is not `[]any`, and joining a
point does not box or reflect on domain values.

### Rule

A Rule declares exact Factor reads, writes, decisions, revocations, and the
Program occurrences at which it applies. Reads are logged at
`(Factor, dependency unit, Guard)` granularity. The dependency unit is the
Factor key unless the owning Factor supplies the finer stable-token closure law
defined above.

Typed writes distinguish replacement/strong update from domain join/weak
update. Program supplies only the candidate write identity and structural
freshness origin. Strong replacement is judged pointwise in the
concretization: in every represented concrete state, the write must target the
one current storage denoted by that abstract location. Binder-proven
non-captured, non-escaping invocation locals have this structural licence even
inside loops and recursive bodies. They live in the current formal activation;
suspended caller locals cross the call boundary as formal continuation data and
are restored after return, so recurrence never aliases them with the callee's
current local. Captured/upvalue/global/heap locations and any location whose
activation identity is stored or shared require the owning domain's
must-uniqueness proof. Revoking that proof restores weak update.
Heap/lens replacement likewise requires the owning
heap/equality/ownership Factor to prove current must-uniqueness. Without that
proof the owning Rule performs the lawful weak update. The engine never
guesses and Program never acts as an alias oracle. Reading an unstored key logs
the Factor default plus the storage presence/version, so a first write
invalidates the reader.

The Rule is invoked separately for each joint leaf. Its private typed read
interface contains all declared Factors from that one product and never a
per-Factor join across alternatives. This is the operational law that
preserves cross-domain correlation.

Rule application is deterministic and pure with respect to semantics. Reusable
scratch and memo tables may change operational state, but a Rule may not:

- scan a global Program relation;
- mutate semantic state outside its declared typed writes;
- call the Solver recursively;
- decode generic codes or strings;
- decide a sibling domain's judgment;
- use a Query.

A Rule may declare a dependency on another Program body after a call-domain
Rule proves a candidate. This is a generic equation dependency, not call
semantics in the engine. Entry and outcome-exit transport use domain boundary
Rules over the Program correspondence.

### Solver

The Solver compiles each Program body and its Rule instances once into an
entry-independent guarded equation system over the same coordinates/carrier used by
the whole analysis:

```text
E_body(formals, captures, imports, type/effect variables)
  → the exact outcome coordinates declared for this body by Program
```

This equation system is the reusable symbolic library artifact. It retains
formal boundary variables and exact recursive dependencies; it is not a
pre-solved exit tuple and not a universal closed relation.

At a call boundary the Solver:

1. resolves the candidate body through domain Rules;
2. runs joint domain boundary Rules over the Program correspondence;
3. validates a cached formal specialization against only the boundary reads that the
   body actually observed;
4. reuses the result when that read projection is identical;
5. otherwise evaluates only invalidated equations and records their exact read
   projection;
6. runs the corresponding guarded outcome Rules back into the caller.

Outcome application obeys the interprocedural valid-path law: a body outcome
is instantiated only through the same Program call/resume/handler
correspondence that supplied that formal application. Equal callers may share
the symbolic computation, but their outcome substitutions and successor
locations never join through a global callee-exit fact. Tail calls carry no
return correspondence; throw/yield/cancel/resume outcomes use only their
Program-declared handler/resumer correspondence. Recursive summaries therefore
match calls and returns without a call-string coordinate, reconstructed CFG,
or unbounded context identity.

Boundary transport is lazy and memoized by the joint input roots, Program
correspondence, formal Guard, and structural activation-renaming law. Body-local keys are not
eagerly copied through a deep call chain. Interned composed renamings are
path-compressed to direct dense translations on first demand, and only demanded
Factor keys/results are materialized. Reusing a child therefore neither walks
its internal points nor replays the caller chain.

Before cache lookup, domain boundary Rules normalize actual identities to the
child's canonical formals while retaining the exact alias/equality pattern and
externally observable identities. Isomorphic caller boundaries may share the
formal computation; outcome Rules rename mutations/results back to their own
actual objects. Non-isomorphic alias, heap, effect, type, or ownership inputs
remain distinct projections.

Closed sub-equations are partially evaluated once. Equal callers share one
formal specialization even at different call sites. The result contains
symbolic body-local allocations, mutable cells, closure environments, and
continuation-resident cells. Abstract activation is the canonical structural
tuple `(kind, Program creation occurrence, structural invocation coordinate)`.
An acyclic sibling call has a distinct coordinate; recurrent execution
revisits the same stable creation/invocation tuple while the active equation
SCC carries `Mu`. This normalization happens before Guard
substitution, boundary-key translation, cache identity, allocation identity,
or any Factor key. It has no capacity policy and cannot mint one identity per
dynamic iteration. Program's activation relation determines which structural
identity is created or restored:

Invocation-local Cell storage is scoped to the current formal body
application, not interned as one global `(Cell, I)` heap location. A recursive
call transports the caller's live locals as suspended continuation formals and
alpha-renames the callee's current locals; return restores the former. `Mu`
closes the recursive input/outcome relation without merging current-frame
locals with suspended ancestor frames. Only captured or otherwise reified
Cells enter an environment/heap identity governed by alias and uniqueness
Factors.

- a new ordinary/direct/callback/closure invocation creates its structural
  call activation; closure creation separately creates its structural captured
  environment; coroutine creation/first entry and first module initialization
  create their corresponding structural activation;
- a closure invocation retains the environment captured by that closure while
  receiving fresh invocation-local cells;
- coroutine resume and handler re-entry restore the continuation's existing
  activation instead of fabricating a new one. Their boundary Rule is a typed
  two-input merge: continuation-resident Cells, captures, body-local Values,
  and live local Guard parameters come from the suspended continuation;
  globals, shared/actor heap, module/provider state, and other world Factors
  come from the resumer's current leaf. The Program continuation relation
  declares this ownership per subject, and the two Guard namespaces compose
  by the same alpha-renaming/correlation laws as calls. No stale world heap is
  restored and no continuation-local correlation is dropped;
- module cache identity ensures initialization is not repeated, including
  partial initialization through an import cycle.

Aliases within one abstract activation are preserved. Repeated execution at a
recurrent site revisits the same structural coordinate and its owning Factors
join/widen at `Mu`; therefore it cannot allocate an unbounded sequence of Guard
atoms, renaming maps, Factor keys, or cache identities. Because that coordinate
may denote multiple concrete activations, singleton/must privileges require a
separate owning-domain proof and are revoked when the proof no longer holds.

A caller difference that no Rule reads is absent from the cache key. A
semantically relevant callback, reachable boundary, type argument, effect row,
or heap/context abstraction is present, so precision is never traded for an
unsound cache hit.

Recursive direct, mutual, higher-order, and callback dependencies enter the
Solver's finite structural invocation graph. Dynamic edge insertion can add
only a candidate body from the sealed Program or the opaque external edge; it
incrementally forms/merges SCCs. If insertion changes the equation or incoming
contribution of any cyclic SCC, whether or not its membership changes, the
Solver restarts the complete affected cyclic SCC from canonical `Init` and
current external-boundary contributions under the new graph and stabilizes it
by explicit `Mu`/WTO work. A prior widened Factor or Guard value never seeds
the new widening; independently validated pure transfer caches may be reused.
Every reverse dependent of the restarted component is invalidated. For every
newly active or changed SCC the engine derives its recurrence interface
generically from the compiled equation graph: decisions owned wholly inside
the SCC are re-evaluated and discharged on recurrence; live parameters are the
exact boundary dependency units read from outside the SCC. The same
substitution, widening, and termination laws used by sealed control SCCs apply.
There is no Go recursive evaluation and no depth cutoff.

Argument or heap values never select or create an invocation coordinate.
Every structural coordinate has one boundary equation whose incoming guarded
Factors are joined by the ordinary lattice laws. Context precision lives in
that joint guarded state and in domain abstractions; ascending recursive input
is widened at the active SCC's `Mu`. Read-projected specialization is only a
memoized evaluation of the same symbolic body equation. It may share work for
equal inputs but cannot merge, split, admit, or redirect semantic coordinates.
Demand closure follows every structural incoming dependency of a reached
coordinate; a newly discovered dynamic edge invalidates and extends that
equation before publication.

A dynamic dispatch set is open until completeness is proven. An open set
always contributes the typed opaque boundary—unknown effects, escape,
mutation, typestate, placement and outcomes—so no negative/must proof can be
published prematurely.

Structurally complete calls use their closed transfer from the first
iteration. Completeness or must-alias discovered by analysis is a separate
must-proof and cannot remove opaque/weak may facts during the ascending phase.
After the affected may/SCC solution stabilizes, that proof may trigger a
dependency-tracked reduction/narrowing phase whose owning operators have a
well-founded termination proof. It reevaluates every affected boundary,
update, conclusion, and evidence coordinate to stability and rechecks a
post-fixpoint. Discovery of a new candidate or alias revokes the proof,
invalidates certificates, restores the sound open/weak contribution, and
restarts widening for the affected component. No definitive proof is
published before the final stable epoch. If no termination-proven refinement
exists, the sound open/weak result remains; the engine never forces
completeness to terminate.

The phase sequence has a lexicographic termination measure, not a retry limit.
Let `U` be the sealed finite candidate universe and `E` the monotonically
growing active-edge set of the live generation. The outer component is the
strictly shrinking finite complement `U \ E`. An edge insertion cancels descent, revokes every
dependent must-proof/certificate, restores the affected sound ascending
contributions, reforms the deterministic condensation, and restarts only its
reverse closure. With the edge set fixed, each condensation component reaches
its widening-stable post-fixpoint in topological order. Only then may its
dependency-tracked reduction/narrowing closure descend under the combined
well-founded ranks declared by the owning domains; proof additions and every
downstream consequence are included in that rank. Revocation invalidates the
entire dependent descent closure rather than leaving a locally refined result.
A whole-epoch post-fixpoint verification succeeds before any exact diagnostic
or certificate publishes. Guarded infeasibility may hide an edge contribution
but cannot delete its dependency, so restoration never requires rediscovery
and cannot oscillate.

All reusable semantic cache records—shards, equations, Rule transfers,
specializations, active edges, evidence, certificates, and emitted
artifacts—obey one dependency-minimal validity law. Their identity contains
every applicable producer and Factor/Rule/widening/proof version, target
contract (and ABI where physical), exact boundary/provider/module dependencies,
and revisions. A shard record carries its immutable shard/Term namespace.
Every per-body/per-coordinate record additionally carries its exact owning
Program shard plus body/occurrence/boundary coordinate and its relevant
normalized Guard/substitution/read projection. Linked/physical records carry
their project namespace.

A specialization additionally uses its body, the exact imported-boundary shard
digests it observed—never the whole-project seal—schemas, alias/freshness
pattern, and widening semantics; it is never keyed by call site, whole caller
State, path, or diagnostic policy. Any mismatch misses before it can affect
scheduling, State, diagnostics, certificates, or emitted code; a dependency
may be omitted only under a tested independence law. Eviction/rebuild changes
work only and never remaps handles. An evictable adaptive candidate index uses
a declaration signature to select candidate traces, then interned
`(coordinate, Factor, dependency-unit, Guard, presence/revision)` tokens and
the exact projection for validation; a new read/dependency invalidates its
reverse slice. Index eviction is deterministic and changes work only.

### Scheduling and publication

- A changed Factor key wakes only Rule instances that actually read that key
  under a compatible Guard, plus necessary control successors.
- A changed decision wakes only readers of that decision.
- A split, alpha-renaming, or canonical rewrite of the joint Guard cover wakes
  correlation-sensitive readers even when each projected Factor value is
  unchanged.
- A cached Rule result is validated by the universal semantic identity above
  plus its recorded read projection.
- Unchanged re-solve performs zero transfers and zero allocations.
- The worklist carries compact joint-state handles, not one interface value per
  Factor.
- Persistent structures share unchanged roots.

Read logs use fixed-width interned tokens and per-coordinate append-only,
deduplicated buffers. Warm validation hashes immutable Factor-root/key
versions; it does not allocate one object per read. Cache/evidence retention is
generation-bounded: live States and active specializations pin shared roots,
while unreachable traces and proof-term nodes are reclaimed without changing
semantic identity.

State is an opaque immutable publication generation, not a semantic cache
record. It has no singular owning occurrence/read projection and no key derived
from whole-State contents. Publication stores one shared joint-state handle for
each requested coordinate; Factor maps remain immutable. Body specializations
publish only requested entries/outcomes/evidence plus the incremental
coordinates required for reuse.

State validity binds the Program-link seal, referenced shard Term namespaces,
target-language contract, engine/Factor/Rule semantic versions, exact
imported-boundary and provider/schema dependencies, registered-root set,
boundary/input generation, and identities of the validated lower-level
artifacts that produced its joint Factors and active graph. `Ask` and
`Solver.Extend` first validate that identity. A Program link, Term namespace,
target contract, engine/Factor/Rule version, boundary/provider dependency, or
input-generation mismatch is rejected and requires a fresh Solve that may
reuse only independently validated lower-level caches, never the old State
roots, active edges, or schedule.

Root growth is not such a mismatch. It is permitted only through
`Solver.Extend`: the prior State is validated under its existing root-set
identity, the new State receives the union-root identity, and verified prior
joint roots, active edges, dependencies, and schedule are retained except for
the exact added and causally invalidated/restarted closure described below.

Query roots are declared before Solve as part of each Query: exact output
families, Factor dependencies, Program occurrence/body/outcome selectors, and
whether precision metadata is required. Solver computes their finite backward
Rule/body dependency closure. When a rooted slice discovers a dynamic
call/resumption edge, membership grows monotonically through that edge and is
finite because the sealed Program has finitely many structural
call/body/resumption relations; the opaque boundary is seeded before the new
coordinates run. An undemanded coordinate is operationally uncomputed, not semantic
top/default. State rejects an Ask for a root that was not included; it never
performs hidden solving. Multiple registered Queries over one State perform no
solving. The complete oracle registers every diagnostic/native observation
root.

Additional demand is explicit: `Solver.Extend(prior State, additional roots)`
returns a new immutable State generation, reuses the prior caches/dependency
graph, and evaluates the monotone added backward closure plus the exact
invalidation/restart closure caused when that work activates an edge, changes a
read projection, or revokes a proof that contributes to an already-computed
coordinate. If a cyclic SCC's equation or incoming contribution changes, its
whole canonical restart and reverse-dependent closure are included even when
they contain coordinates demanded by the prior roots. Every untouched prior
coordinate performs zero work. The result is projection-equivalent to a fresh
Solve over the union of roots. Ask remains pure and never calls Extend
implicitly; this adds no new public lifecycle noun.

Query may select, filter, order, and render converged facts. It may read a
declared Factor at a Program occurrence or body boundary. It may not:

- scan all points to infer a missing result;
- compute a closure, reachability, ownership, placement, type, effect, or call
  judgment;
- parse strings or inspect Rule instances;
- call the Solver or mutate State.

If a result requires recursion, is consumed by another Rule or backend
correctness, or reconstructs a fact not present in State, it is a Factor/Rule
judgment, not a Query.

Each semantic domain therefore writes typed violation/proof evidence at the
occurrence where its judgment is made. A diagnostic Query does not compare
types, rediscover an unsafe use, or infer trust; it renders and applies advisory
publication policy to those converged domain facts.

## Domain ownership

Domains declare Factors first; then primitive and relational-reduction Rules
are bound. A reduction may read any number of Factors from the same joint leaf
but writes only the one Factor that owns its conclusion. There is no fixed
semantic import ladder.

### Control reachability

The control child package owns one occurrence/outcome reachability Factor.
Its semantic default is unreachable; a feasible incoming Program edge writes
reachable under that joint Guard, and dependency-tracked contradiction pruning
removes that contribution. Because undemanded is operationally distinct from
the Factor default, a registered Query can project unreachable only after the
reachability root was solved. Dead-code diagnostics and backend block
certificates read this Factor; Query never computes a control closure, and the
engine contains no reachability judgment.

### Value and static type

Program supplies literals, operations, declarations, annotations, type syntax,
value/list shape, bindings, and predicates. Domain Factors own runtime
possibilities, finite reference sets, truthiness, nilability, type
instantiation, subtype/refinement results, and typed evidence.

Recursive source types use explicit recursive declaration identity and
domain-owned fixed-point laws. They are never expanded by depth.

### Numeric and equality

Numeric owns intervals, residues, affine/difference relations, thresholds,
widening, and narrowing. Equality owns equivalence/alias classes and
congruence. Information exchange with value, heap, and predicates is by
declared relational Rules owned in domain child packages. A relational
numeric/equality element is keyed by its stable Program-derived variable pack;
closure changes the whole pack and invalidates it atomically.

### Heap and key space

Heap owns abstract allocation identity, slots/locations, mutation epochs,
containment, alias reachability, metatable effects, weak-reference strength,
and shape eligibility.
`Cell` always means the Program's exact lexical/global/upvalue storage; a heap
slot is an allocation-plus-key abstraction and never a Program Cell.

The Heap Factor's stable key is the allocation/aggregate root. Inside that
root's lattice element, its state-dependent partition distinguishes:

- exact field atom;
- exact canonical boolean/string/number key atom;
- finite symbolic key set;
- equality-class key;
- array/range partition;
- unknown dynamic key.

Deep lenses remain structural through substitution across arbitrarily nested
libraries. Finite Program allocation/key handles use exact powersets.
Symbolic numeric/string/reference partitions use their domain join and
termination-proven widening at `Mu`; they never rise to a summary because a set
cardinality limit was reached. No string path or fixed semantic depth appears.
Partition join transports slot contents through the sound common partition;
partition refinement/coarsening never changes the enclosing Factor key or
escapes the owning heap lattice.

Weak tables and finalization are part of the frozen target Lua semantics, not
GC implementation details. A table edge is strong, weak-key, weak-value, or
the target contract's ephemeron relation according to the possible `__mode`
state. Heap/reachability/ownership reductions interpret those edges: weak
references do not become strong merely because analysis needs a witness, and
an ephemeron value is retained only under the contract's key-reachability law.
Mode changes retain their specified delayed/collection-boundary alternatives.
Unknown mode preserves all feasible strengths under Guard and records the
uncertainty; it does not silently choose strong or weak.

Program declares a typed runtime-lifecycle application relation at exactly the
finite operations where the target may collect or finalize, including explicit
collection and state shutdown. It carries finalizer eligibility, ordering,
once-only status, error/non-local behavior, resurrection, and weak-entry
removal timing from the frozen Lua/runtime ABI. Call/effect/heap/ownership
Rules interpret the same relation, and possible finalizer calls participate in
the normal application SCC/`Mu`. Collector scheduling and implementation
remain runtime-private. This preserves lifetime and diagnostic precision
without inventing a second GC graph in the engine.

An allocation root at a recurrent coordinate may denote arbitrarily many
concrete objects, so finalization state is never one boolean on that root.
The owning heap/lifecycle element soundly partitions represented members among
fresh, eligible, finalizing, finalized, and resurrected phases (or an
equivalent target-law partition), with zero/one/unbounded multiplicity and
identity correlation sufficient to enforce once-only finalization per concrete
object. A strong phase transition requires the existing singleton/must
identity proof. Otherwise a transition preserves the possible remaining
source members while adding the target phase; a finalized member never becomes
eligible again merely because it resurrected, while a later allocation at the
same structural root adds a distinct fresh member. Phase multiplicities and
relations use a termination-proven lattice widening at `Mu`, never a count cap.

### Effects

There is one typed Koka-style effect system.

Program carries authored effect syntax, label declarations, operation
parameters, handlers, and row variables only where the source/provider schema
has them; otherwise it carries exact Lua effectful operations and boundary
contracts. The effect Factor carries inferred canonical rows with:

- dense typed label/operation handles and typed Program parameters;
- open or closed tails;
- row-variable substitution at generic, call, callback, and module boundaries;
- duplicate-label semantics where elimination requires it;
- handler masking and normal/throw/yield/resume/control outcomes;
- an explicit unknown open tail distinct from the empty row.

Rows denote unordered possible effect capabilities/operations, not execution
traces. One Effect Factor element is the reduced product of:

- an immutable canonical solved equation/substitution over the finite Program
  row variables at that structural activation; and
- the canonical may-effect row after applying that substitution.

This is one Factor and one authority, not a constraint side table or parallel
effect plane. A row term is empty, one row-variable representative, or an
extension `⟨label | row⟩`; label order is canonical but duplicates are
preserved. Exact generic/call/callback/module/handler equations are solved by
principal Koka-style row unification with duplicate-label semantics,
occurs-check, and deterministic representative choice. Repeated uses of the
same variable share one representative; binding it to the empty row removes
the tail after substitution. Boundary transport composes and normalizes the
substitution before cache identity or handler elimination. An inconsistent
exact equation is a typed contradiction, never an unknown-open shortcut.

Control-flow alternatives remain separate under Guard. When a may confluence
really requires a row LUB, it first applies the solved substitutions, then
computes the canonical least common row/generalization with a deterministic
coordinate-local row representative and records the principal equations that
relate the input tails to it. Closed alternatives join label multiplicities by
maximum and join payloads. The normalized operation is
associative/commutative/idempotent up to row equivalence and must prove the
least-upper-bound law; it never replaces equality constraints with a set of
unrelated tail variables. A genuinely opaque boundary alone introduces the
distinguished unknown-open tail.

Order is concretization inclusion. Per-label may multiplicity is an upper
bound, never additive execution count. It is exact only in a join-free sealed
row whose derivation records exactness. Recurrence reuses the same row equation,
so execution count never increments multiplicity; any recursive row equation
stabilizes through the Effect Factor's declared `Mu` widening. Typed payload
Factors apply their own widening at `Mu`. A handler unifies the action row with
`⟨label | ρ⟩` and returns substituted `ρ`; on a may-joined upper bound it
removes one known occurrence while preserving upper-bound status, and an
unknown-open tail stays open.
Control order, resumption, and protocol sequences live in Program outcome coordinates
and typestate/evidence paths, not in the row. Row substitution, handler
transfer, join, widening, and boundary laws are tested through direct, mutual,
callback, module, and coroutine cycles.

There is no `Content any`, reflection-based label identity, label string,
parallel operational-effects record, or effect handoff embedded in a call
summary. Escape, placement, typestate, and diagnostics read the effect Factor
through typed Rules.

Typed provider operations may carry formal value/type/effect parameters and
postcondition operation handles. Each owning domain interprets its typed
operation with Rules; effect rows do not become an `any` payload container for
heap/type/placement conclusions.

### Calls and higher-order flow

The call domain owns callee sets, dispatch completeness, opaque/imported
boundary behavior, and candidate evidence. It does not own a mega-summary of
value/equality/heap/escape fields.

Once a candidate is proven, the Solver specializes that body's generic
equation artifact. A callee set explicitly carries open/complete status. The
open case always retains the opaque typed boundary contribution; only a
complete set licenses negative/must proofs. Higher-order callback applications
remain formal residual dependencies until a boundary correspondence supplies
the callback. Supplying it invalidates only equations that read it.

### Escape, ownership, typestate, and placement facts

Escape, ownership, transfer, suspension, residence, footprint, and typestate
are semantic Factors with their own lattices and Rules. They retain maximum
per-Guard precision about:

- containment, aliasing, and escape through capture/store/return/send/global
  boundaries;
- owner, lifetime, last use, capture/callback retention, suspension survival,
  and actor/module/global reachability;
- borrow/move/share/send obligations and transfer outcomes;
- mutation/publication order, identity observability, contents at each transfer
  occurrence, non-interference by later mutation, and whether replacement
  would be observable;
- graph portability, cycles, capabilities, descendant reachability, and
  immutable/shared facts;
- exact or abstract symbolic allocation count, object/graph footprint,
  element-count/capacity bounds, overflow, and uncertainty. Physical byte size
  and alignment are derived by target lowering from these facts plus its ABI;
- typestate transitions, invalidation, and every semantic blocker.

These Factors never prescribe physical region/allocation/layout/root/COW policy
or a placement plan. Queries project their converged facts/evidence; backend
and runtime choose a guarded lawful representation, or the generic correct
operation when proof is absent. Pointer encoding, safepoints, stack maps,
allocation failure, and resource policy remain unconditional runtime work.

Heap, ownership, escape, lifetime, and transfer Rules interpret each finite
Program publish/send occurrence directly. Under its Guard they derive the
reachable aliases, contents relevant to the transfer contract, mutation before
and after that occurrence, exact or unknown changed lenses, descendant
immutability and portability, ownership obligations, exceptional outcomes, and
uncertainty. Unknown aliases, dynamic keys/metatables, capabilities, or cycles
remain explicit blockers.

There is no Snapshot/published-object/runtime-version/shadow-ownership plane:
Program order and the ordinary Factor value at each occurrence describe each
observable send. No abstract object is minted per iteration or delivery, and
deliveries have no identity relation unless the ABI makes one observable. The
generic runtime operation guarantees that a receiver observes the sent contents
and later sender mutation cannot change them, including partial construction,
failure, discard, cancellation, death, provider error, and disallowed cycles.
Analysis only supplies facts that certify a cheaper observationally equivalent
move/seal/share/path-copy implementation; it never chooses it. The one public
aggregate operation remains correct without such proof. Frontier COW, roots,
proxies, forwarding, cloning, and reclamation are private runtime mechanisms;
any use preserves aliases, immutable-child reuse, failure/release, and prior
published values without a global live-reference scan. Ordinary private/local
writes, including compatible shaped-field writes, remain direct. Local aliases
observe ordinary writes; remote/published values do not. A no-write proof may
select a cheaper immutable representation. Solver, Rule, State, and Query
contain no copy-policy heuristic or runtime allocation algorithm.

Ownership abstracts obligations, not a shadow runtime refcount table. It tracks
exact zero/one ownership when provable and an interval with an unbounded upper
endpoint under recurrent/multiple sends. Only exact ownership can erase a
retain/release; otherwise runtime reference counting remains. The interval
widening at `Mu` proves convergence without a cardinality cap.

Footprint/ownership facts include overflow, exceptional and non-local outcomes,
partial initialization, nested ownership, callback/provider re-entry, and
recurrence. Unknown trip count or footprint remains unknown; no guessed
reserve appears in a Factor. Runtime limits and allocation failure are concrete
Program/runtime outcomes, never Solver budgets. Backend lowering alone decides
generic growth, reserve checks, or hoisting and may not weaken safepoint repair.

Target-runtime conformance is one explicit backend/runtime gate. It proves that target
lowering interprets the semantic escape/lifetime/ownership/mutation/footprint
facts without performing a second analysis over LIR. Its required memory
contract is:

- actor-arena object Values are absolute tagged pointers valid between
  safepoints. Every live such Value at a relocating safepoint is represented in
  a runtime-recognized root or backend stack map and is reloaded after the
  runtime relocation protocol. No offset identity or universal stable object
  handle is introduced;
- a thread Value retains the target ABI's distinct generation-stamped identity while
  its resolved block may move. Shared, registry, and external tags obey their
  own contracts and are not spuriously rebased as actor-arena pointers;
- a raw/unboxed pointer may exist only in a bounded sequence proved by backend
  construction to contain no relocating safepoint. It cannot cross an
  allocation-capable call, deoptimization/transfer door, callback, suspension,
  or native exit;
- actor/shared transfer preserves the ABI alias/isolation/failure contract and
  balances the target's ownership obligations; host/native retention never
  keeps an unregistered actor-arena raw pointer.

Collector algorithm, root-container layout/lifecycle, scanning order, and
shared reclamation implementation are runtime-private. Conformance tests force
relocation, growth, suspension, callback, transfer, and failure to verify the
contract.

Until those runtime conformance laws pass, the backend must use generic
allocation, precise conservative roots/stack maps, and conservative ownership
actions regardless of analysis precision.

Evidence is a non-semantic sidecar keyed to the exact interned owning-Factor
conclusion, Guard, assumptions, and version. Each evidence node names its
deriving Rule, Program occurrence or boundary correspondence, and typed premise
evidence edges for every Factor conclusion used by that derivation. A refused
certificate/optimization conclusion retains the explicit blocking
conclusions, uncertainty, and precision-loss facts, so a Query can explain why
proof was unavailable rather than reconstructing the judgment. The owning
conclusion—not presentation policy—records whether it is a must theorem, a may
violation, or a result weakened by abstraction/widening; evidence preserves
that status through every premise edge. Evidence cannot
influence lattice equality or convergence. Join, Guard normalization,
widening, narrowing, revocation, and boundary transport keep/rebuild only
evidence valid for the resulting conclusion; exact proof publication is
refused otherwise.

Validity is constructive, not a version-only check. Typed evidence
constructors mirror Rule derivation, Guard branch injection, Guard-cover join,
Factor join/reduction, boundary substitution, widening/narrowing, and the
existing compiled-equation `Mu` backreference. A must conclusion under Guard
`g` is publishable only when its premise evidence covers every feasible leaf
of the canonical cover of `g`; normalization carries a checked equivalence
between the old and new covers. A may conclusion retains at least one feasible
abstract branch injection but is not thereby a concrete witness. Join,
reduction, and boundary constructors name all premises they semantically use.
Widening or incomplete coverage must propagate the owning conclusion's
uncertainty/precision-loss status and cannot construct an exact certificate.
Final post-fixpoint and version validation then checks that this complete
derivation still refers to current conclusions; it cannot repair missing Guard
coverage.

The sidecar is an interned immutable finite proof-term graph built with the
Factor conclusion. Ordinary justification nodes are acyclic. A conclusion that
depends on an equation SCC uses a typed backreference to that exact
compiled-equation `Mu` head, validated against its final post-fixpoint,
conclusion, assumptions, and Factor versions. Evidence introduces no
proof-specific fixed-point binder, arbitrary cycle, or unrolling cutoff. A
proof through nested libraries shares the child's evidence and adds boundary
substitution edges; it does not rescan child points or rebuild the call chain.
Rendering traverses the finite graph iteratively and never uses Go recursion.
It can therefore present a source-anchored abstract derivation path, boundary
substitutions, recursive `Mu` step, and semantic blocker while leaving choice
of wording/severity to diagnostic policy. It may call that path a concrete
witness only when a concrete execution or a domain completeness theorem
establishes realizability; an overapproximating may-path is never reported as
an executable counterexample.

Backend-certificate and diagnostic Queries only project already-converged
residence, lifetime, ownership, mutation/order, footprint, typestate, blocker, and
evidence conclusions; there is no separate placement-plan Query. Projection
performs no allocation-by-point scan, transitive closure, owner search,
suspension classification, or fallback inference. Backend/runtime lowering
owns the target decision.

## Backend boundary

Program replaces analysis CFG/WIR, not backend LIR.

The backend receives Program plus optional typed certificates and owns:

- physical control blocks and backend CFG;
- scalarization/SSA local to lowering;
- register allocation, captures/upvalues, instruction selection, constants,
  block layout, bytecode PCs, and serialization;
- MIR, ABI, native registers, semantics-neutral inlining heuristics,
  safepoints, deoptimization, and
  native labels.

Semantic specialization consumes typed certificates for call target, type,
numeric range, key/shape, alias/effect, escape/ownership, placement, typestate,
and suspension. It may choose a generic correct opcode when a proof is absent.
It may not reconstruct a semantic proof from LIR patterns, path strings,
source AST, or analysis CFG points.

Every certificate is either statically invariant or carries a concrete runtime
guard, invalidation dependency, proof revision, and generic/deoptimization
fallback. It obeys the universal cache-validity law: identity includes the
owning Program-link seal/shard Term namespaces and semantic shard/boundary digests, target
contract and backend/runtime ABI, Program occurrence, owning Factor/Rule
semantic versions, Guard/abstract activation, and proof revision. Physical shape IDs, field
offsets, registry indexes, and byte offsets remain backend/runtime artifacts.
Metatable mutation, module loading, callbacks, shared-heap transitions, and
shape invalidation cannot leave a certificate silently live.

JIT guard removal follows one proof order:

1. an unconditional Factor theorem removes the guard entirely;
2. a theorem entailed by the current Program Guard removes repeated checks only
   in its dominated guarded region;
3. a revision-stable theorem hoists one runtime semantic/resource guard and attaches
   every invalidation dependency plus deoptimization state;
4. otherwise the generic operation or local guard remains.

This applies uniformly to tag/nil/type, callee, result arity, shape/metatable,
field/key/bounds, alias/ownership, actor/shared residence, immutable/sealed/COW
state, refcount ownership, suspension, provider
lifecycle, module revision, and allocation capacity. A proof that omits one
possible invalidator is not a weaker certificate; it is no certificate.

Placement certificates may remove allocation, promotion/clone,
retain/release, write-barrier, or bounds/capacity
work only when their exact ownership, lifetime, initialization, relocation,
and failure obligations are covered. They may never weaken precise
safepoint/root/stack-map repair. A raw address may span only the backend's
bounded non-calling, non-relocating instruction sequence; it cannot cross an
allocation-capable call, callback, host/provider transition, deoptimization
door, suspension, or native exit. Shared ownership still requires proof for
refcount elision.

Every guard that can deopt/transfer to the interpreter or reach a safepoint,
and every allocation-capable call, yield/resume, deoptimization door, and
interpreter-exit boundary, has one backend recovery record rooted in Program
provenance: exact live Program Values/Cells,
their physical representation or materialization recipe, ownership/release
obligations, continuation/suspension identity, and safepoint
root/reload rule. Backend liveness supplies the conservative physical live
set; certificates may refine representation but never omit recovery truth.
Native code crosses such a boundary with values in the precise tag-specific
root/stack-map representation: actor-arena Values are repaired absolute tagged
pointers, thread Values retain their stamped thread identity, and no
unregistered actor-heap raw pointer survives. There is no second LIR-derived
deopt or placement fact taxonomy. A purely local native branch to a generic
native sequence needs no recovery record unless it can deopt, transfer, or
reach a safepoint.

Backend lowering records provenance as ranges/multimaps from Program
occurrences/calls/suspensions to zero or more LIR instructions and emitted PCs,
plus reverse origin for diagnostics/deoptimization. Expansion, elimination,
fusion, and inlining retain or combine provenance; they do not pretend identity
is one-to-one. No CFG-point identity survives.

Bytecode remains executable truth. Disabling analysis certificates changes
performance only. Backend liveness and runtime representation always produce a
conservative GC root/stack map without analysis; placement/shape certificates
may only remove work behind their validity guards.

## Complexity and performance laws

The system controls complexity by its mathematics, never by abandoning work at
a capacity:

- Program lowering/sealing: linear in source relations apart from sorting and
  one-time interning;
- Guards: exact reduced symbolic functions inside each recurrence region plus
  termination-proven recurrence transport at `Mu`; worst-case exponential size
  is measured and attacked by sharing and demand, never cardinality collapse;
- Program-handle reference, callee, allocation, and exact-key sets: exact
  finite powersets;
- numeric, symbolic-key, equality, and heap chains: domain widening with a
  termination proof, applied only at explicit `Mu`;
- invocation coordinates: the finite structural Program/candidate graph;
  argument values never create coordinates;
- specialization traces: evictable operational caches whose
  retention changes work only;
- evidence: hash-consed finite proof terms with typed backreferences to the
  existing compiled-equation `Mu` heads,
  rendered on demand;
- no depth bound represents a cycle as acyclic.

The analysis engine introduces no parallel-solver architecture. Its speed
comes from demand closure, exact invalidation, symbolic sharing, reusable body
equations, and zero-work cache hits. Actor execution, JIT/runtime scheduling,
and their parallelism belong to the backend/runtime design and do not enlarge
the formal engine vocabulary.

No semantic algorithm has a cardinality cap, admission quota, context budget,
guard budget, iteration cutoff, or “first N then top” rule. A domain may
produce top/unknown only as its declared abstraction, join, or
termination-proven widening result.

Finite machine resources are an operational concern, never an abstract
operator. An explicit cancellation, host timeout, allocation failure, or
process/resource limit may end a solve only as `incomplete`, outside every
Factor lattice. The solve generation is transactional: an incomplete run
publishes no State, Query result, diagnostic, certificate, oracle numerator,
evidence graph, or semantic-result cache entry. Previously committed immutable
artifacts remain valid, but no uncommitted conclusion from the failed
generation becomes reusable truth. Retrying with more resources executes the
same equations and can change only incomplete to a completed semantic result;
there is no reduced-precision resource mode.

Dynamic candidate activation, the active-edge set, derived SCC/WTO/`Mu`
schedule, reverse dependencies, and dynamic read logs are also generation-local
semantic work. Abort rolls all of them back to the last committed State; only
the sealed candidate universe and input-independent compiled skeleton survive.
A successful State commits its active graph together with the exact semantic
read projection logged by the call/dispatch Rule that activated each edge:
the responsible Rule identity and semantic version; Factor conclusion handles,
Guard, dependency tokens, and versions; the application/candidate-body shard
identities; the sealed project-link namespace; and every imported boundary,
provider/schema, and target-contract semantic digest that Rule interpreted.
`Solver.Extend` over that same State may reuse it. A changed Program or
boundary/input generation revalidates those read projections first and
excludes invalid edges before forming SCCs; the responsible Rule recomputes
them and may reactivate the edge. It may reuse only edges whose complete
semantic dependencies—including Rule/domain/contract versions—are unchanged.
Diagnostic evidence may reference this
record but never controls activation or scheduling. Within one live generation edge
activation remains monotone. Thus cancellation or incremental reuse cannot
leave a ghost edge that changes widening, precision, or diagnostics.

Hot-path requirements:

- dense integer Program handles;
- packed canonical Guards;
- flat vectors of small Factor-root handles;
- typed arenas and monomorphized Factor operations;
- no string construction/parsing, reflection, `any`, interface decoding,
  parser, source-symbol interner lookup, or global relation scan; typed
  Factor-root hash-consing remains an intentional computed-table operation;
- no point-by-Factor State publication;
- no whole-State cache key;
- exact read logging and reverse invalidation;
- reusable scratch and zero-allocation cache hits;
- no Go recursion for program recursion.

Required measurements on one pinned corpus and machine:

- parse, bind, lower, seal, manifest encode/validate/decode, engine compile,
  solve, query, backend lower, and bytecode time reported separately;
- p50, p95, maximum, total wall time, allocations, and peak RSS;
- cold and warm solve;
- source-built versus stored-artifact module load, link, first Query, and
  execution;
- one-line edit in one module: shard relink, invalidated equations/transfers,
  allocations, and retained untouched-shard cache work;
- one, ten, and one hundred equal-projection callers;
- distinct projections that differ in one value, heap key, callback, type row,
  and effect row;
- deep-library placement/effect proof substitution;
- actor-local/shared placement and preallocation workloads, including direct
  shared construction, seal/clone/COW, retain/release, actor transfer,
  coroutine suspension, and forced relocation;
- branch-partition and recursive-context stress;
- second Query performs no solving.

The frozen target-language contract also owns per-family scale workloads,
including adversarial Guard/product correlation, dynamic application/SCC
growth, heap-key partitions, recursive effects, and publication/lifecycle
cycles. Each reports input scale, completed/incomplete status, wall time,
allocations, and peak RSS. Performance envelopes are release acceptance
criteria, not semantic cutoffs: exceeding one is a visible performance failure
requiring optimization or more resources, never permission to collapse a
Factor or publish a partial answer.

No performance gate may hide missing semantic work. A regression is accepted
only with a measured semantic gain and explicit approval.

## Semantic gates

There is no test named or described as the full oracle unless it enumerates the
complete checked-in expectation ledger.

The honest analysis report has separate, explicit counts for:

- source parse/bind/lower/seal;
- convergence;
- expected diagnostics;
- missing diagnostics;
- unexpected diagnostics;
- severity/trust/span mismatches;
- native/provider facts;
- runtime/compiler conformance;
- concrete-execution containment;
- incomplete/resource-failed roots;
- performance.

A narrow fixture-diagnostic test is deleted, not retained as a convenient
green signal.

Only completed roots contribute semantic numerators. Every incomplete root
remains in the denominator and makes the relevant oracle/performance gate red;
no report may omit it, substitute cached partial observations, or relabel it as
an expected diagnostic.

Tests are semantic:

- lattice, default/presence, order, widening, and narrowing laws per Factor;
- identity/composition/alpha-renaming/freshness laws for joint boundary Rules;
- recursive/higher-order calls with equal callee inputs share computation but
  return only through their originating correspondences; tail calls,
  throw/handler, and yield/resume cannot create invalid cross-caller paths;
- Rule monotonicity and deterministic read-projection reuse;
- demanded/full equivalence across independent equal-projection callers and
  a newly discovered recursive/dynamic structural edge;
- late dynamic-edge activation after an old component widened versus the same
  edge active early, covering both an edge that merges SCCs and a non-merging
  incoming/internal edge that changes an existing cyclic equation: the whole
  affected cyclic SCC canonically restarts, and a deliberately
  history-sensitive lawful widening produces the same rooted demanded/full
  projection and final active graph;
- Guard correlation and exact canonical-reduction equivalence;
- acyclic decision-death projection drops only subjects absent from every
  registered potential observable dependency; relational-pack and
  partitioned-root domains exercise exact cleanup Rules and refusal-to-drop
  fallback. Guard merges only equal resulting products and matches the
  undiscarded requested projection. Rules execute once for equal read
  projections rather than once per irrelevant Guard leaf;
- loop, recursive-call, resume, and handler recurrence in which the same source
  decision is true on one visit and false on the next; the prior visit is
  discharged at `Mu` without losing live invariant predicates;
- domain-proven pruning of contradictory nested guards, cross-domain
  contradictions after reduction, restoration after invalidation, and zero
  evidence/diagnostics from unreachable leaves;
- explicit `Mu`, direct/mutual/higher-order recursion, and no Go recursion;
- mixed loop/goto/handler/resumption SCCs and dynamic demand growth;
- a dynamically discovered higher-order/callback SCC derives its internal
  decision-discharge and boundary-live interface from the equation graph and
  converges under the same `Mu` law as a sealed control SCC;
- open-to-complete dispatch and weak-to-strong update only through the stable
  must-proof narrowing epoch, including proof revocation;
- fresh allocations, mutable locals, capture environments, and
  continuation-resident cells across equal calls, callbacks, resumes, modules,
  and recursion;
- recursive non-captured frame locals update strongly without overwriting a
  suspended ancestor frame; captured/upvalue/global/heap writes remain weak
  until their owning domain proves uniqueness;
- resume restores continuation-local Cells/Guard correlation while reading
  post-yield global/heap/shared world state from the resumer;
- unbounded looped calls, closure/coroutine creation, and internal branches
  reuse their structural recurrence coordinates, terminate through Factor
  widening at `Mu`, and never allocate identities per dynamic iteration;
- adversarial recursive chains cannot terminate because of a cap, cutoff,
  depth test, cache eviction, or forced top;
- black-box Program lowering for every AST form in every scalar/list/tail,
  lvalue, branch, loop, call, table, return, type, effect, and module context;
- the complete finite Program operation matrix and external-operation ABI;
- external-operation ABI scenarios for synchronous callback, zero/repeated
  callback, retained/asynchronous re-entry, provider-induced
  suspension/resume/cancellation, callback capture retention, and a provider
  cycle entering the ordinary `Mu` SCC;
- ordered provider scenarios for acquire → callback → release, provider result
  only after callback return, one-shot versus repeated resume, and retained
  asynchronous re-entry;
- metamorphic evaluation-order and substitution invariance;
- alpha-renaming/source-format invariance only for programs that cannot observe
  names, source text, or locations through Lua debug/reflection APIs (otherwise
  compare through an explicit name/location translation);
- library reuse and deep proof substitution;
- source-built and stored-module paths produce the same canonical shard
  denotation, project relations, generic bytecode behavior, diagnostics, and
  certificates; shard-core and individual derived-section bytes/digests are
  deterministic across processes, and decode→encode is byte-identical for the
  same selected section set;
- the same stored shard bytecode linked into two projects with different
  provider/import operation slots resolves each shard-local typed boundary to
  the correct project operation; the stored bytes never embed or accidentally
  reuse either project's dense slot;
- recursive type/equation/`Mu` artifact graphs round-trip without host
  recursion; malformed lengths/tags/indexes/backreferences, corruption,
  truncation, version/contract/ABI mismatch, and cancellation publish nothing,
  while absence of a valid derived cache section recomputes the same result;
  structurally valid bytecode/equation sections with the wrong Program
  translation/provenance are rejected and rebuilt before execution/solve;
- source-built and decoded body equations produce identical static linked
  equations and candidate universes; changed roots or boundary inputs construct
  independent active graphs, and a dynamic edge activated in only one
  generation re-forms only that generation's SCC/`Mu`/WTO while each result
  matches a fresh Solve for its own roots and inputs;
- changing one module shard permits complete-semantic-identity cache hits for
  untouched shards' compiled equations, read logs, and validated
  specializations; cache eviction may rebuild them, and a changed imported
  boundary invalidates exactly its reverse dependency slice;
- changing the target-language contract digest invalidates/rebuilds affected
  Program shards, project seal, compiled equations, Rule results,
  specializations, active-edge/evidence records, backend certificates, and
  backend/JIT artifacts before any old semantic or physical result can
  publish;
- explicit `Solver.Extend` is projection-equivalent to a fresh Solve over the
  root union, including when new demand activates a non-merging incoming edge
  into an old cyclic SCC and therefore restarts that SCC plus its old-root
  reverse dependents; coordinates outside the causal restart/invalidation and
  added backward closures perform zero work, while Ask over an unregistered
  root performs no work;
- `Ask`/`Extend` reject a State after Program re-link, shard-Term-namespace,
  target-contract, producer-version, provider/schema, or boundary/input
  mismatch; the required new Solve cannot inherit old roots, active edges, or
  schedules, though separately validated lower-level caches may hit;
- cancellation during dynamic dispatch activation and during an SCC merge
  publishes no active edge/schedule/dependency residue; retry from the last
  committed State is byte-equivalent to a fresh solve with the same
  Program/inputs/roots;
- finite, checkable recursive placement/effect/escape proof terms reference
  the existing compiled-equation `Mu` head and never introduce a proof-local
  binder, depth-cut, or recursive unrolling;
- placement laws for actor-local versus shared-at-birth versus
  seal-on-transfer, pinned external values, nested portability/cycles,
  COW/last-mutation, shared-child retain/release, last-use release, and
  per-Guard alternative residence;
- a loop that mutates one deep field and sends successive values to one and
  many actors: each receiver observes the contents at its own send occurrence,
  and mutating an unchanged actor-local child after send cannot change a value
  delivered by an earlier send.
  When lowering selects certified frontier COW, only the proved frontier is
  rebuilt and only immutable shared descendants are reused; otherwise the one
  generic primitive takes its safe clone/serialize/reject behavior;
- a received shared aggregate aliased through multiple locals, table slots,
  upvalues, closures, and repeated references preserves its internal aliasing
  across an ordinary write while the previously published value stays
  unchanged.
  Separate actor deliveries have no cross-delivery identity requirement; a
  proved read-only receive may use a cheaper representation;
- shaped-record writes cover ordinary versus raw assignment, function/table
  `__newindex` dispatch and its effects/outcomes, compatible direct field
  update, nil-as-absence for ordinary dispatch, a proved raw/no-dispatch
  missing-field nil no-op, and
  identity-preserving widening to an ordinary table for raw/no-dispatch
  missing non-nil or incompatible values; every pre-existing alias observes
  the same object and the implementation performs no global live-reference
  scan;
- every supported metatable-controlled operation covers primitive success,
  each specified fallback/delegation order, callable/table chains, raw bypass,
  non-local outcomes, and recursive dispatch through the ordinary application
  SCC/`Mu`; nil/NaN reads, ordinary writes, and primitive/raw commits exercise
  their distinct presence/dispatch/error laws;
- strong, weak-key, weak-value, and ephemeron reachability; uncertain and
  changed `__mode`; finalizer eligibility/order/once-only behavior,
  resurrection, weak-entry removal timing, error behavior, explicit
  collection, and state shutdown all match the frozen target Lua/runtime
  contract;
- a recurrent site allocates multiple finalizable objects, collection
  finalizes each concrete object at most once, and a finalizer allocates
  another finalizable object; the lifecycle distribution remains sound and
  converges through the ordinary application/heap SCC `Mu` without an
  instance or iteration cap;
- effect rows test principal unification of shared and distinct formal tails,
  empty-tail elimination, substitution composition at generic/call/handler
  boundaries, canonical least common row at may confluence, unknown-open
  propagation only from opaque boundaries, duplicate-label handler
  elimination, upper-bound multiplicity, and exactness loss/restoration;
- exact/abstract symbolic footprint and count facts for tables, records,
  closures, capture environments, coroutine stacks, channels, actor heaps, and
  transferred graphs; backend preallocation choices cover overflow, failure,
  partial initialization, non-local exit, and unknown-trip-count fallback;
- JIT removal/hoisting of type, shape, residence, ownership, refcount, and
  capacity guards, with forced invalidation at metatable, mutation,
  callback/provider, suspension, actor transfer, allocation, and module
  boundaries; safepoint root/stack-map correctness remains unconditional;
- forced actor/thread growth and global relocation during native/host callback:
  all live actor-arena absolute tagged Values are rooted, rewritten, and
  reloaded; the tag-specific thread identity still resolves; other tag classes
  are not spuriously rebased; and no semantic certificate can
  authorize a raw address across the relocating boundary;
- backend/interpreter semantic equivalence.

The oracle also has a counted concrete-containment lane independent of the
checked-in diagnostic expectations. The generic backend instruments demanded
Program occurrences and records observed values, tags/types, table keys and
mutations, control/effect outcomes, calls, sends, and allocation/residence
events.

Each event carries a test-only typed trace key made solely from existing
Program handles: the Program occurrence/location; the exact structural
incoming application/callback/resumption/module relation; the selected Program
body or opaque-target handle and its entry/root case; the outcome
correspondence; and concrete outcomes of the Program decisions live at that
coordinate. The selected target is obtained from the executed closure,
metamethod, module, or provider provenance and must be one of the sealed
candidate pairs; it is not inferred from observed argument values.

The harness applies the Program's ordinary alpha-renaming, boundary
correspondence, structural activation, and recurrence substitution to obtain
exactly one Solver coordinate and one canonical Guard leaf. The event must
belong to the concretization of the converged owning Factor conclusion at that
exact coordinate/leaf. Zero or multiple mappings, an inactive relation, an
unsealed target pair, an unrepresentable decision valuation, or an absent
conclusion is a counted oracle failure; there is no global-coordinate,
joined-Guard, or “any matching leaf” fallback. The trace key exists only in
instrumentation/test provenance and creates no production Program, engine, or
runtime semantic plane.

Backend instrumentation receives only the demanded Program occurrence handles
and emits those Program handles, observed concrete data, and execution
provenance: Program-link seal, target-language contract, runtime/backend ABI,
and boundary/input execution identity. It never receives or emits State,
Factor, Solver-coordinate, Guard, or Query-root identity.

After execution, the test harness—not backend/runtime—pairs that immutable
trace with one successful completed State and the observation-root set
registered before its Solve. The harness owns this comparison-batch
provenance, validates exact Program/contract/ABI/input agreement, and rejects
the batch unless every event maps to a registered root in that State. It never
solves on demand, compares against another generation, or exposes State
identity to the executable path.

Bounded exhaustive small programs and metamorphic generated programs
supplement fixture executions. Dedicated cases distinguish equal-looking
callers, recursive visits, callback re-entry, and suspension/resumption whose
local and world Factors come from different sources. This cannot prove
soundness, but it detects a wrong abstract model even when the expectation
ledger was authored from the same wrong implementation.

No test may assert Go package composition, imports, filenames, directories,
registrations, authority ladders, adapters, or production implementation
shape.

Stage statements such as “no old import remains” are destructive-cut audits,
never semantic gates or checked-in tests. At the cut, an explicit one-time
dependency/reference inventory records the exact zero residue in the migration
ledger; the audit code is not retained in production or the test suite.

## Flash migration constitution

Prohibited:

- adapters, bridges, forwarding packages, compatibility aliases;
- `Legacy`, `Compat`, `Old`, `New`, or `V2` production paths;
- dual Program, binder, lowerer, Solve, summary, effect, key, or diagnostic
  authority;
- feature switches between implementations;
- a shadow production run;
- restoring structural tests because the old implementation passed them;
- leaving a replaced path in the import graph for later removal.

Retain only after re-deriving it against this design: persistent pointwise
Factor storage/structural sharing, lattice and termination-proven widening
laws, explicit-stack WTO/cancellation mechanics, and semantic scenarios for
Lua evaluation, types, effects, calls, heap, and placement. Reject as
production design: the former engine judgment layer, admission/authority
ladders, engine inventory/relation/frame/code vocabularies, CFG/WIR
reconstruction, point/program Scope, fixed call-summary payloads,
string/reflection/`any` fact planes, per-domain Program scans, Query or LIR/JIT
semantic inference, composition tests, and every adapter or parallel path.

The previous implementation may exist only in an inert `_reference` or
`_attempts` tree that Go ignores and no production/test package imports.
Compiler errors after a destructive cut are the finite migration ledger.
Partial work remains on its cutover branch until the stage's semantic exit is
met.

Every stage begins by deleting/quarantining the authority it replaces and ends
with one named implementation. There is never a bridge stage.

## Ordered implementation

### Stage 0 — quarantine and binding decisions

- Verify every `_reference`/`_attempts` tree is inert and absent from the Go
  import graph.
- Record which formal-core laws/domain lattices survive re-derivation against
  this design.
- Classify old tests into semantic scenarios or structural/transitional
  assertions; only semantic scenarios survive.
- Freeze this design after two consecutive adversarial passes find no new
  defect.

Exit: one binding design, one inert reference tree, no code implementation.

### Stage 1 — honest baseline

- Land the governance commits that remove the false fixture oracle,
  admission-composition guards, and false-green gates.
- Freeze one versioned target-language contract from the actual parser,
  runtime ABI, standard libraries, typed syntax, and Wippy actor/provider
  extensions. It enumerates every grammar form, evaluation rule, operator and
  metamethod fallback, result adjustment, non-local outcome, reflection/debug
  observation, GC/lifecycle behavior, and external operation. It also records
  excluded facilities: `debug.setlocal`, `debug.setupvalue`,
  `debug.setmetatable`, `load`, and equivalent runtime source/binary-code
  loading entrypoints are unavailable, not opaque operations. Sealed
  module/provider resolution remains the only code-loading path. A vague
  “supported Lua” subset is not an acceptable denominator.
- Produce the complete explicit semantic/native/performance ledgers.
- Freeze corpus, toolchain, machine, and measurement commands.

Exit: every reported numerator has its real denominator; each language/runtime
row has an owner and conformance scenario; no narrow gate can be mistaken for
system completion.

### Stage 2 — destructive language-spine cut

- Move the analysis binder, CFG builder, WIR, analysis front, old formal/direct
  engine, engine Program, generic relation/frame vocabularies, semantic
  Queries, and their structural tests outside the live graph.
- Quarantine all live analysis entrypoints/consumers in the same cut. This
  explicitly includes the typed compile/runtime API, proto type injection,
  runtime type validation/introspection, their dependent tests, and every root
  package that imports the old analysis manifest/type stack. The old
  manifest/type representation is not re-homed behind a compatibility package.
- Keep only the untyped AST-direct compiler/VM slice whose transitive import
  closure is independent of analysis, because no canonical Program exists yet.
  It is removed at the beginning of Stage 3, not adapted. If that slice cannot
  be separated by deletion alone, the entire old compiler is quarantined and
  Stage 2 validates only the standalone untyped VM/runtime substrate.

Exit: the surviving analysis-independent, untyped VM/runtime substrate builds
and its applicable unchanged suite is green; typed compilation, typed runtime
introspection, and analysis are explicitly unavailable with a finite ledger.
No removed package is restored, no manifest/type bridge exists, and no second
Program exists.

### Stage 3 — complete Program and executable generic backend

- Assign one owner to the Program schema/identity/seal/project-link spine, one
  lowering integrator to binding and construction order, one backend integrator
  to the generic LIR control/value ABI, one persistence owner to the generated
  artifact schema, and one runtime owner to the memory/root contract. Parallel
  lanes consume these frozen contracts and may not redefine them.
- If Stage 2 retained an analysis-independent AST-direct compiler slice,
  remove it before adding the canonical `compiler/program` package/path.
- Remove every Stage 1 excluded runtime/library entrypoint in this cut,
  including debug mutation and dynamic code loading. No hidden host/provider
  alias may re-expose one; conformance proves the operations are unreachable.
- First flash-cut the runtime prerequisites of the canonical Program:
  replace shaped-record widening and shared-to-mutable writes with one
  observable aggregate contract that preserves within-delivery aliases and
  transfer isolation without scanning live references; audit/fix actor
  relocation roots, open-upvalue/thread-block relocation, shared
  publication/child ownership, and safepoint repair. Tag/header/root/forwarding
  layout remains runtime-private. Superseded scan-based implementations are
  deleted in this cut; there is no runtime adapter or compatibility mode.
- Implement binder and vertical AST-to-Program lowering by executable
  grammar-family slices. Each slice lands its complete Program rows, generic
  LIR/bytecode lowering, and interpreter/runtime conformance subset before the
  next family begins. Unsupported families remain explicitly unavailable;
  they never route through the removed compiler.
- Before grammar-family fanout, land one retained two-shard executable spine
  that exercises lexical binding and mutation, exact table-key/lens read and
  write, a branch, direct recursion with explicit `Mu`, call and module
  correspondence, and multiple-result adjustment. It must pass source →
  Program → generic LIR → bytecode → VM, deterministic seal, and the generated
  internal schema round-trip without an AST/backend fallback, generic operation
  row, alternate builder, local codec, registry, or adapter. This is a prefix
  of the final target matrix, not a toy implementation or published partial
  artifact format.
- After that spine passes, fan out only complete vertical tranches. A tranche
  consists atomically of its target-contract rows, typed Program rows and
  writer calls, seal validation/direct indexes, generic backend lowering,
  provenance, and black-box semantic cases. A row, builder helper, or backend
  handler may not merge alone. If a lane discovers a missing shared identity or
  operand, the affected wave stops, the spine owner changes the one canonical
  contract, and every dependent lane rebases; no local bridge is accepted.
- Seal all direct indexes, structural Guards, control recurrence,
  application/correspondence relations, type/effect/module substitutions,
  continuations, typed external-operation boundary events, module
  initialization, immutable content-addressed module shards, and their one
  project link.
- Complete the generated grammar/context matrix and adversarial black-box
  tests before any engine integration.
- Implement the final Program → backend LIR → bytecode path using generic
  correct operations only; implement backend control, register allocation,
  closures/upvalues, list adjustment, provenance, serialization, and VM entry.
- Implement the one canonical manifest encoder/validator/decoder after the
  Program schema is complete. Source-built and decoded shards enter the same
  project linker and generic backend; store generic bytecode only under its
  exact target ABI/compiler identity. Remove/reject every old manifest schema
  in this cut and rebuild stored modules atomically—no dual decoder.
- Run the complete interpreter/runtime/coroutine/debug/error/cache conformance
  suite through both source-built and precomputed-module paths.

Exit:

- every source in the parser, compiler, runtime, and fixture corpora binds,
  lowers, and seals;
- 100% of the frozen target-language contract has a Program representation and
  executable generic behavior;
- every source control/direct-call cycle is explicit; dynamic application
  structure is available for Solver structural-invocation SCCs;
- determinism, reflection-scoped alpha-renaming, list adjustment, evaluation
  order, static-key/lens equivalence, outcome/continuation, module-cycle,
  type/effect, and linear-cost laws pass;
- a complete-semantic-identity shard cache hit reuses validated work; eviction
  or a miss rebuilds the shard with identical semantics, and changed
  shard/boundary dependencies invalidate their recorded reverse slice;
- forced actor/thread growth, relocation, suspension, callback, transfer, and
  failure scenarios satisfy the tag-specific root/reload and ownership
  contract before analysis can optimize them; tests assert observable safety,
  never a root-container implementation;
- shaped-record ordinary/raw/metatable writes preserve alias identity through
  widening and perform no global live-reference scan;
- shared aggregate aliases in locals, heap slots, and upvalues all observe an
  actor-local write while the previously published value remains unchanged;
  proved read-only receives may use a cheaper runtime representation;
- canonical module artifacts round-trip deterministically, reject corruption
  transactionally, and execute/link identically to source-built shards;
- 100% runtime behavior executes Program → LIR → bytecode and the old compiler,
  CFG, and WIR are absent;
- no Solver/domain package is imported.

This is the executable semantic feedback gate. Analysis never grows on an
unexecuted source model.

### Stage 4 — guarded sparse carrier

- Verify the quarantined engine has no live import before creating the sole new
  engine package in an empty live namespace. No old engine package, type, or
  test is restored as scaffolding.
- Implement Guard, joint sparse Factor fibers, Rule access control, exact read
  logging, strong/weak updates, dependency-tracked unreachable leaves, reverse
  invalidation, deterministic WTO and an explicit `Mu` for every compiled
  equation SCC, recurrence-scoped Guard transport, widening/narrowing, demand
  roots, immutable State publication, transactional incomplete/cancellation
  behavior with no partial State/result/evidence cache publication, and typed
  root arenas.
- Remove Scope, `[]any`, point-by-Factor State copies, global dirty sweeps, and
  language-shaped engine relations.

Exit: the mandatory synthetic suite includes a stable-pack relational Factor,
a stable-root Factor with a state-dependent key partition and partition
widening, a two-input suspend/resume boundary, and a dependency-restorable
reachability Factor. The relational and partitioned domains each provide one
exact lifetime-cleanup Rule and one retained-subject case. Together they prove
lattice/product/partition/scheduling/publication laws including
default/presence, exact Guard normalization, widening and precision loss;
zero-allocation unchanged solve; no engine package imports a domain.

### Stage 5 — parametric bodies and library reuse

- Compile one equation system per Program body.
- Treat occurrence, entry, and every outcome exit as coordinates of the same
  carrier.
- Instantiate domain-owned joint boundary Rules in synthetic law tests; the
  engine does not generate a universal transport.
- Implement exact read-projected specialization, partial evaluation, reverse
  dependency invalidation, adaptive lookup, structural activation
  normalization before identity use, module artifact keys, and shared
  evidence.
- Add the context-parametric compiled-equation cache section to the same
  canonical module artifact schema using each declared Factor's typed codec.
  It contains only body-local equations, local source-control `Mu`, and typed
  boundary dependencies. Loading it constructs the identical body equation
  artifact as fresh engine compilation; after project seal both enter the same
  static equation binding/candidate construction. The Solver independently
  constructs the State-generation active graph and SCC/`Mu`/WTO. Absence or
  version mismatch recomputes and never invokes another summary or manifest
  analysis path.
- Stabilize direct, mutual, indirect, higher-order, and callback invocation
  cycles through incremental structural SCC/`Mu`/WTO.

Exit:

- equal-projection callers share one formal body solution while distinct
  applications retain sound allocation freshness;
- a changed irrelevant caller fact executes zero child equations;
- a relevant one-key change executes only its reverse slice;
- recursive and higher-order tests use no Go recursion;
- open dispatch contributes the opaque boundary until completeness is proven;
- deep-library synthetic value/heap/effect/evidence transport is exact;
- no fixed central call-summary payload exists.

### Stage 6 — domains, by dependency leaf

All old judgments remain in the inert reference tree throughout these waves.
Only semantic laws re-derived against the canonical model may be ported; no
old type/package is moved back into the live graph. A wave ends with lattice,
joint-boundary, Rule, reduction, scenario, and performance laws; no Query
supplies semantics.

1. control reachability, value, static type, equality, numeric, local effect
   operations, callee-set lattice, and conservative opaque/open boundary;
2. heap, alias/key partitions, metatable, containment, shape eligibility, and
   their boundary Rules;
3. call/callback/module body dependencies and complete Koka effect inference as
   one strongly connected fixed-point cluster;
4. escape, ownership, typestate, suspension, residence/mutation/footprint facts,
   and all cross-domain evidence/reductions, including the complete target
   actor/shared transfer and ownership semantics. GC relocation and root repair
   remain an unconditional backend/runtime law rather than a placement Factor.

Wave 4 cannot start if the Stage 3 target-runtime memory law is red. Its first
gate verifies that no LIR-local escape scan, runtime-call placement guess, or
shared-lifetime inference was ported as a semantic authority. Any such
surviving target-backend heuristic is deleted before certificate integration;
generic backend allocation/ownership remains correct until formal
certificates enable each optimization.

Before a wave, complete its Program-supply/Factor-demand table. A missing
operand is a Program seal or Rule declaration failure, never a scan or fallback
path.

Exit: the complete corpus converges with all semantic Factors inside the frozen
measured performance envelope; composition contains declarations only.

### Stage 7 — Queries and honest oracle

- Implement diagnostic, module/provider, native, and typed
  backend-certificate Queries as pure projections of converged Factors and
  evidence.
- Apply advisory diagnostic policy only at Query publication.
- Run the complete ledger and add real diagnostics or fix semantic Factors
  until every checked-in expectation is met.

Exit: the complete oracle reports its full denominator and is green; repeated
runs are byte-identical; no Query reconstructs semantics.

Analysis is complete here. Backend work cannot weaken this gate.

### Stage 8 — certified backend specialization

- Let the existing generic Program/LIR backend consume revisioned typed
  certificates.
- Add guarded specialized operations, invalidation dependencies, generic
  fallback, placement/preallocation lowering, retain/release optimization,
  safepoint/root integration, guard erasure/hoisting, and differential tests; semantic
  inference remains absent from LIR.
- Integrate any certificate-selected copy optimization inside the one public
  aggregate publication/write operation. Its generic behavior remains
  sufficient for correctness; a target may add frontier COW internally. No
  public or semantic COW operation is added.

Exit: every specialization can be disabled independently without changing
runtime behavior; invalidation and provenance laws pass.

### Stage 9 — JIT

- Lower bytecode/LIR to MIR/native code using revisioned typed certificates.
- Implement guards, deoptimization, ABI, safepoints, and stack maps.
- Differentially verify ordinary and optimized execution.

Exit: disabling all certificates changes performance only.

### Stage 10 — demolition

- Verify zero imports/references to inert attempts and removed authorities.
- Remove inactive vocabularies, stale architecture documents, and reference
  trees after explicit approval.
- Publish the final vocabulary, formal laws, complexity bounds, oracle ledger,
  and performance digest.

Exit: one Program, one formal engine, one domain path, one Query path, one
backend, and no transitional mechanism.
