# Symbolic Relation Retirement Contract

## Decision

Symbolic relations are the destination, not an optional fast path. Once the
coverage and equivalence gates below pass, the analyzer has one
interprocedural model:

1. compile each lexical function to a symbolic relation;
2. solve relation dependencies and recursion once;
3. specialize the frozen relation at lexical call sites; and
4. materialize results for diagnostics without re-solving callees per caller
   context.

A known lexical call must never mean "try a relation, then solve the callee
body if it fails." Failure to compile or specialize a known lexical call is a
pre-publication error. Dynamic, imported, native, and externally described
calls remain explicit boundary providers; they are not a legacy lexical
fallback.

## Architectural authority

The following components are the final authorities:

- `analysis/check/fixpoint/transformer` owns symbolic function compilation,
  relation composition, and immutable relation vocabulary. Its local WTO and
  `SolveRelationCells` scheduler are migration oracles, not final scheduling
  authorities.
- `analysis/semantic/program` owns loop and call/resource-SCC structure;
  `analysis/engine/region` is the sole execution boundary and delegates generic
  WTO mechanics to `analysis/engine/solve`.
- `analysis/check/body` remains lexical preparation and a differential oracle
  during migration. Final diagnostics and summaries are projected from guarded
  observation/evidence terms on stabilized evaluated roots; production does
  not replay a concrete body to materialize them.
- `analysis/check/fixpoint/summary` remains the normalized, versioned module
  boundary and cache artifact vocabulary. It is not an equation engine.
- `program/internal/relationcall` specializes known lexical relations.
- `program/internal/callresult` retains only explicit dynamic, imported,
  native, and externally described call lowering.

The 17-axis product domain, axis registry, transfer semantics, observations,
diagnostics, manifests, and oracle remain authoritative. Relation composition
changes how interprocedural work is scheduled and reused, not what the axes
mean.

## Ordered migration and deletion

Deletion happens in this order. A later step must not be used to hide missing
coverage in an earlier one.

### 1. Make the symbolic vocabulary total

Represent parameters, captures, globals, heap reads and writes, allocations,
guards, correlated returns, methods, varargs, generics, callbacks, protected
calls, and suspension. Every reachable known lexical function must publish a
relation, including recursive SCCs.

Add a tripwire that rejects any known lexical call reaching the external call
provider. Unsupported symbolic operations reject the program transaction;
they do not select another solver.

### 2. Make relation preparation mandatory

Build and freeze the relation catalog in every `RunBoundChunk` and
`RunBoundFunction`. Replace the activation vocabulary:

- `prepareInactiveRelationCatalog` becomes the normal relation-program
  constructor;
- `inactiveRelationResolverFactory` becomes the normal resolver factory; and
- exact-leaf and certified-context activation slices disappear once the full
  catalog is certified.

Then remove these `Config` fields:

- `enableRelationActivation`
- `relationCatalogAudit`
- `relationSnapshotAudit`

Delete `relation_activation.go`, `relation_owner_cache_policy.go`, their
activation-policy tests, `relationActiveV1ResolutionMarker`, and
`composeRelationActiveResolutionDigest`. Keep `semanticProgramAudit`, which is
a general semantic test hook rather than migration routing.

### 3. Remove fallback composition

In `checkConfigWithSummaries`, replace
`relationcall.Exclusive(relation, legacy)` with explicit call classification:

- known lexical call: relation resolver only;
- dynamic/imported/native/external call: boundary provider only.

Delete `relationcall.Exclusive` and its fallback tests. A catalogued lexical
route that cannot bind or specialize is an error before any result or cache
artifact is published.

### 4. Remove the concrete outer summary fixed point

Replace the `query.Function` construction and `query.Run` calls in
`program.go` with semantic program cells executed by `engine/region`, and a
single normalized summary/observation projection per stabilized lexical
relation. `transformer.SolveRelationCells` remains only until region parity is
complete, then is deleted with the duplicate transformer scheduler.

Delete:

- `analysis/check/fixpoint/query`
- `program/query_order.go` and its tests
- `chunkFunction`, `boundFunction`, and `solveSummaryPrepared`

The generic intraprocedural solver remains. The removed query loop is the
duplicate interprocedural mechanism that currently invokes whole CFG solves
as its transfer function.

### 5. Remove concrete call contexts

Once symbolic parameters, captures, guards, heap terms, and effects cover the
same semantics, remove concrete entry-state specialization:

- `context_index.go`
- `context_semantic_key.go`
- `context_seeding.go`
- `call_context_entry.go`
- `call_context_refresh.go`
- `callback_phase_context.go`
- `materialization_context_queue.go`
- their context-specific tests

Remove `programKeys.contexts`, `callContextRef`, `functionExpressionRef`,
`contextKeyFunc`, all context loops in `RunBound*`,
`materializeDiscoveredContexts`, and `contextResultsByFunction`. Remove
`keyedFunction.entryState`, `entryKeys`, `hasEntryState`, and
`relationContextEntry`.

The concrete entry-state parts of `call_argument_seeding.go`,
`capture_seeding.go`, `definition_capture_entry.go`, `param_inference.go`, and
`method_receiver_context.go` can then be deleted. Static type, escape, and
metatable proof collectors remain until their facts have an explicit symbolic
producer; file deletion must not discard those producers accidentally.

### 6. Replace re-solve caches with an artifact cache

Delete machinery whose only purpose is caching or regionally replaying a
concrete body-to-summary application:

- `summary_solve_cache.go`
- `retained_summary_application.go`
- `retained_summary_solve.go`
- `point_summary_dependencies.go`
- their tests
- `relationTrackedSummaryReader`
- `installRelationInputDigests`

Remove `Config.SummaryCache`, `Config.CacheProfile`, and the old summary-cache
counters. Replace service ownership in `analysis/check/service/solve.go` with
an immutable, analyzer-versioned relation/transformer artifact cache usable by
both lint and compile. Its key must include source and binding identity, axis
registry/schema identity, semantic configuration, and dependency artifact
identities.

### 7. Replace materialization with evaluated-root projection

Compile guarded observation/evidence terms into a separate annotation
semilattice attached to canonical semantic rows. Observation payload is not
part of semantic row equality, widening, or call-SCC convergence: equivalent
semantic rows explicitly union their guarded annotations. After semantic SCC
stabilization, run one deterministic, finite observation closure and project
diagnostics, read models and summaries from the frozen evaluated-root snapshot.
Semantic rows and their annotation sidecar publish atomically, but they do not
share convergence identity. Recursive provenance must be SCC-normalized; an
unbounded dynamic call-stack hash is forbidden. Delete discovered-context
solves, retained handoff and final concrete body replay.

In `materialization_cache.go`, delete `materializedSolveCache`,
`trackingSummaryReader`, dependency-universe comparison, retained-handoff state
and concrete result attachment. Retain only immutable evaluated-root projection
helpers used by diagnostics and compiler DTO construction.

Finally remove activation census adapters, duplicate producer/consumer
identity indexes, compatibility APIs, and superseded POC tests. Preserve
lattice-law, collision, cancellation, race, determinism, soundness, and
semantic differential tests.

## Hard retirement gates

### Coverage

- Every reachable known lexical function and call in all fixtures and the
  frozen 2,210-entry Kickside corpus has a certified relation.
- All pathological families are included.
- There are zero contextual compiler rejections and zero known-lexical
  fallback attempts.
- Unsupported syntax fails before publication; no file is deadline-skipped or
  silently unchecked.

### Semantic equivalence

- Diagnostics and manifests are byte-identical.
- Normalized summaries and every registered axis oracle are equivalent.
- Result versions remain identical until the deliberate artifact-schema flip.
- `scripts/wall.sh`, fixtures, curated oracle, soundness, lattice laws,
  architecture audits, fuzz corpus, race tests, and vet pass.
- Cancellation publishes no relation, summary, or cache artifact.
- Repeated and parallel runs are deterministic.

### Version and cache

- The sole-path flip bumps `summary/schema_version.go` and the analyzer artifact
  version once, deliberately invalidating old cache entries.
- The final cache stores immutable symbolic artifacts, not concrete caller
  applications.
- The temporary `rel-act1` generation fence is gone after the schema flip.

### Work counters

Before their removal, the legacy counters must prove they are dead:

- `SummaryBodySolves == 0`
- `SummaryBodySolvesAfterDependencyChange == 0`
- `MaterializedContextSolves == 0`
- `MaxContextCount == 0`
- `RelationActivationFallbacks == 0`
- materialized body solves are no greater than the lexical body count

Add an architectural test that fails if a known lexical call can invoke
`body.SolvePrepared` transitively.

### Performance

- Full cold Kickside lint completes with every entry checked in at most 40
  seconds on the frozen benchmark host.
- The selected pathological Lua file completes in under one second.
- A warm run approaches immutable artifact loading cost.
- Peak RSS does not regress from the frozen baseline.
- Promotion measures total preparation, admission, transformer compilation,
  prepass, evaluation, projection, and rejected-owner work. A reduction in
  summary/materialization counters alone is insufficient.
- Relation plans are immutable preparation/cache artifacts. Production must
  not rebuild a broad per-run catalog merely to admit a narrow owner slice.

Only after all four gate groups pass is the legacy code deleted. The deletion
is part of the migration definition of done, not optional follow-up cleanup.

## Next measured slice: parameter plus immutable global

The first production-value proof is the frozen threads `is_str` function at
lines 31–33 (body `16010901263544322178`). It currently accounts for 10 body
solves and 70 transfers across phases and exercises a parameter plus immutable
stdlib global `type`. The larger immutable-global family accounts for 160 of
271 solves and 7,205 of 11,882 transfers, so this boundary is the measured next
step rather than another call-free leaf optimization.

Land it in three independently gated pieces:

1. publish exact ordered global roots, bind `RootGlobal`, and seal a complete
   call-surface artifact whose sites are lexical, content-bound external, or
   rejected;
2. close only the existing scalar expression-condition/refinement vocabulary
   required by `type(value) == "string" and value ~= ""`; and
3. publish certified parameter/capture/global contextual summaries as pinned
   equations for an acyclic singleton transaction, retaining concrete
   materialization until observation projection is complete.

The first slice does not claim recursive SCC support. Any binding,
specialization, content-identity, coverage, or call-surface miss rejects the
whole owner before query publication. Acceptance requires every discovered
contextual summary equation for this owner to be discharged, strictly lower
total summary work, and exact all-lane states, summaries, diagnostics,
manifests, observation products, and `ResultVersion` across legacy, repeated,
and strict runs.

Commit `54f4b86f0` met that private strict gate for the frozen `is_str` owner.
Four independently identity-fenced full contexts are concretely validated and
retained; caller-only ambient path facts are ignored only by the symbolic
candidate, never removed from the validation or diagnostic Result. Repeated
legacy/strict differentials are byte-identical and the owner moves from 10
body solves/70 transfers to 6/42 with contexts/omissions/reuses = 4/4/4. The
whole frozen file moves from 271/11,882 to 267/11,854, which is too small to
change wall time outside noise. This is a semantic-reuse proof, not a
production promotion.

The next private slice closes statement-form sealed Lua type predicates and
prepares the direct-call boundary without activating it. The plan now owns an
immutable, owner/CFG-width-fenced call census built independently from WIR;
stable local targets use a binder-owned O(1) index, while method, dynamic,
mutable, ambiguous and unresolved calls remain explicit rejected sites. The
frozen graph `str` function then moves from 36 body solves/288 transfers to
14/112, with contexts/omissions/reuses = 19/19/38, zero misses/fallbacks, and
exact products and diagnostics. Its relation reconstructs the sealed
`lua_type(param) == literal` expression DAG instead of treating the operand as
a boolean. Syntactic return aliases live in a separate relation projection
semilattice, so they survive SCC joins without contaminating row feasibility,
ReturnFlow, or parameter-preservation proofs.

The closed direct-call fanout gate is now implemented privately. Exact lexical
CallSurface identities are resolved to generation-local relation cells, and
only outgoing-closed acyclic producer regions activate. Direct-callee-only
capture values may remain in the concrete validation carrier only when their
exact Lua function identity matches the binder's immutable local-function
identity; they never enter the symbolic Shape. Every admitted lexical base is
then solved once in dependency order, every summary read must already be
present and belong to a strictly earlier producer rank (apart from independent
zero-boundary pins), and every specialized context is concretely revalidated
in the same order before atomic publication. This preserves diagnostic
Results and byte-identical ResultVersion lineage while omitting both base and
context equations.

The three-function shared-leaf fanout regression owns 3 producers and omits
and reuses all 6 base/context equations with exact summaries, products,
diagnostics, manifests, and recursive ResultVersion parity. On the frozen
oracles, graph.str improves from 36/288 to 12/96 solves/transfers with
contexts/omissions/reuses = 19/21/42, and threads `is_str` improves from 10/70
to 5/35 with 4/5/5; the whole threads file moves from 271/11,882 to
266/11,847. There are zero misses or fallbacks. This is still private strict
evidence and does not claim a default wall-time improvement.

Exact early-return type-guard evidence is now preserved by the transformer
compiler (commit `b4fabba3e`). The compiler derives the active CFG edge,
respects canonical branch truth polarity, and proves only the exact
string/number/nil refinement implied by that edge; alternate predecessors,
wrong polarity, mismatched paths, unsupported truthiness, and ambiguous active
edges fail closed.

On top of that evidence, the private strict path now seals the canonical
`string.gsub` method boundary for the frozen `trim` owner. Admission requires
an exact unversioned receiver root, binder-wide immutability, an edge-dominating
string refinement, no intervening root assignment, a complete call-surface
descriptor match, and a pure context-independent static scalar signature.
Dynamic or descriptor-drifted methods, effects, generics, composite returns,
and open multi-return forwarding remain legacy. The frozen threads oracle moves
from 271 solves/11,882 transfers to 264/11,829 with
contexts/omissions/reuses = 5/7/7. Diagnostics, summaries, every registered
product oracle, and repeated-run digests are byte-identical. Wall time remains
within measurement noise, so this remains private capability evidence and is
not a default-path performance claim.
