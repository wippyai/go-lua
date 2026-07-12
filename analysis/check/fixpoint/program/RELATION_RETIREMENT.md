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

Make guarded observation/evidence terms part of the atomic relation row and
project diagnostics, read models and summaries directly from the frozen
evaluated-root snapshot. Delete discovered-context solves, retained handoff and
final concrete body replay.

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

Only after all four gate groups pass is the legacy code deleted. The deletion
is part of the migration definition of done, not optional follow-up cleanup.
