# Route runtime atomic cut map

Status: dependency map, 2026-07-18. This is a deletion/wiring plan, not a
compatibility plan. The invariant is that production has exactly one executable
interprocedural engine before and after the cut. The formal evaluator must not
be wired as an option, fallback, audit path, or second `Solve` method.

## Current executable spine

There is one precise route-owned spine today:

```text
program.runPreparedRelationProgram
  -> transformer.FreezeRelationProgram
  -> (*RelationProgram).Solve(bodyID, concrete entry State)
  -> freezeRelationInvocationPlan
  -> newRelationForestCoordinateScheduler
  -> newCoordinateDirtyScheduler / coordinateDirtyScheduler.solve
  -> freezeRelationForestView
  -> StabilizedRelationView.ProjectApplications
  -> program.publishRelationProgram
  -> ExecutionFactory.PublishResult once per application route
```

The formal destination is present only as static inventory:
`RelationProgram.formalRegion` is built by
`freezeFormalRelationRegionInventory`, and owns sealed cells, influences and a
`solve.WTOPlan`. It has no evaluator or production caller. That is the correct
pre-cut state: the route runtime remains the sole executable engine until one
atomic replacement.

## Keep: immutable semantics and generic mechanics

These are not route execution and must survive the cut.

### Relation syntax and compilation

- `transformer/relation.go`: `Relation`, `Builder`, immutable descriptor and
  projection payloads.
- `transformer/world_program.go`: frozen compiler IR and publication syntax.
- `transformer/reduced_relation.go`: `relationCode`, `relationNode`,
  `boundaryStep`, `boundaryOutcomeTuple`, `relationApplyRef`, and the reduction
  from `WorldProgram`. This is the callable immutable syntax.
- `transformer/relation_semantic_transport.go`: immutable prefix/terminal
  transaction vocabulary. Its route-executor consumers disappear, but the
  semantic payloads remain inputs to formal factor transfer.
- `transformer/relation_code_closure.go` and
  `transformer/relation_environment_closure.go`: static term closure and
  lexical environment closure. Remove any dependency on linked runtime frames,
  but retain the closure proof.
- Compiler and freezer surface: `compiler*.go`, `compiler_prepare.go`,
  `structural_freezer.go`, `cfg_rows.go`, `cfg_wto_tape.go`, `terms.go`,
  `expression_terms.go`, `effect_terms.go`, `effect_catalog.go`,
  `observation*.go`, `certificate.go`, `capability.go`, descriptor/guard/value
  algebra, and direct lexical declaration/call-boundary syntax.
- `transformer/relation_program.go`, narrowed to an immutable lexical catalog:
  stable body-to-`relationVar` mapping, sealed `relationCode`, complete call
  topology, recursive SCC metadata, definitions, formal root schemas and the
  formal region inventory. It must no longer own concrete application domains,
  execution callbacks, linked frames, route allocation authorities or
  caller/callee `keyspace` transport.

### Formal carrier and schedule

- `transformer/formal_slot_space.go`: collision-free lexical formal slots.
- `transformer/formal_relation_region_inventory.go`:
  `formalRelationCell`, `formalRelationInfluence`, frozen cell universe and WTO
  plan. Evaluation may not discover another cell or edge.
- The one registered formal product carrier and exact IN/MID/OUT composition
  algebra described by `formal_relation_reconciliation.md`.
- `transformer/decision_diagram.go` and
  `transformer/decision_pair_partition.go`: payload-independent decision DAG
  algebra. Keep this kernel; delete the concrete `State`/route wrapper around
  it.
- `analysis/engine/region` and `analysis/engine/solve`, especially
  `region.RunPrepared`, `solve.EquationSystem`, `solve.WTOPlan` and the
  uncapped equality-based ascent/narrowing logic. The formal evaluator is an
  adapter to these generic mechanics, not a new scheduler.

### Preparation and public result semantics

- `program/program.go`, `program/prepared_bodies.go`, `program/program_keys.go`
  and static portions of `program/relation_program_input.go` remain the driver
  and preparation boundary.
- `body.ExecutionFactory.PublishResult` and
  `body.StabilizedResultCoordinates` remain the public read-model constructor,
  but receive one stabilized lexical result per body, not one result per route.
- `analysis/check/fixpoint/summary` remains normalized artifact vocabulary, not
  an equation engine.

## Delete: route-owned executable state

The following symbols/files form one implementation and must be removed in the
same source cut that wires formal evaluation.

### Invocation expansion and per-route forest

- Delete `transformer/relation_invocation_plan.go` entirely:
  `relationInvocationRef`, `relationInvocation`, `relationInvocationEdge`,
  namespaces/resources, `relationInvocationPlan`,
  `freezeRelationInvocationPlan` and its recursive-route expansion.
- Delete `transformer/relation_forest_coordinate_scheduler.go` entirely:
  `relationForestRuntime`, `relationForestCoordinateBuilder`, call cuts,
  per-invocation allocation/seeding and `newRelationForestCoordinateScheduler`.
- Delete `transformer/relation_forest_definition_equations.go` entirely. A
  lexical definition is a formal relation equation/resource dependency, not a
  route-owned concrete feedback world.
- Delete `transformer/relation_code_executor.go` entirely:
  `relationCodeRuntime`, its loop table and all lowering from immutable code to
  dirty concrete coordinates.
- Delete `transformer/relation_boundary_transport.go` entirely:
  `relationInvocationRoute`, `relationSemanticRoute`, concrete boundary-root
  extraction/canonicalization and root carriers. Exact formal substitution is
  its replacement, not a wrapper around it.
- Delete runtime-only linking from `transformer/relation_program.go`:
  `relationProgramBody` concrete-domain/authority fields,
  `linkedRelationFrame` and every `linkedFrame*` type,
  `linkFrozenFrames`, allocation/existential authorities, call receiver lenses,
  route authority and concrete inbound/outbound rebase helpers. Preserve only
  the immutable formal call binding implied by `relationApplyRef` and the
  target formal schema.

### Dirty scheduler and guarded concrete world

- Delete `transformer/coordinate_dirty_scheduler.go`, including
  `dirtyCoordinate*`, `coordinateTransform`, `coordinateDirtyScheduler`, its
  private queue, contribution admission and joint narrowing. Generic WTO is
  the sole scheduler after the cut.
- Delete `transformer/coordinate_contribution_fold.go`; incoming formal
  contributions are owned by the generic equation system/product join.
- Delete `transformer/coordinate_block_transform.go` and
  `transformer/coordinate_sparse_plan.go` as executable coordinate machinery.
  Any useful closed selector/factor dependency declaration must be consumed by
  the formal relation compiler before these files go; their guarded route
  evaluator and component roots must not survive.
- Delete the other scheduler-bound helpers:
  `coordinate_selection_contract.go`, `coordinate_value_access.go`,
  `effect_coordinate_access.go`, `branch_factor_topology.go`,
  `relation_action_guard_authority.go`,
  `relation_application_guard_plan.go`,
  `relation_guard_forest_vocabulary.go`, and `relation_guard_decision.go`.
  Static guard substitution belongs in formal composition; route-rank/lifetime
  authority does not.
- Delete `transformer/guarded_world.go` and
  `transformer/guarded_world_existential.go`: `semanticRoute`,
  `semanticWorld`, `guardedStateKernel`, `guardedStateAuthority`, and
  `guardedWorldArena` are the concrete route carrier.
- Delete the concrete guarded transfer layer once each transaction is handled
  by its registered formal factor operation:
  `guarded_boundary_input_factor.go`, `guarded_boundary_output.go`,
  `guarded_boundary_receiver.go`, `guarded_boundary_root_sidecar.go`,
  `guarded_branch_factor_plan.go`, `guarded_call_outcome_sidecar.go`,
  `guarded_call_return_presence.go`, `guarded_choice_refinement.go`,
  `guarded_coordinate_factor.go`, `guarded_coordinate_family.go`,
  `guarded_diagnostic_publication.go`, `guarded_dynamic_value_decision.go`,
  `guarded_environment_write.go`, `guarded_object_constructor.go`,
  `guarded_object_value_decision.go`, `guarded_return_terminal.go`,
  `guarded_root_assignment.go`, `guarded_sparse_presence.go`, and
  `guarded_value_decision.go`.
- Delete `boundary_application_executor.go`. Split the few immutable semantic
  payload definitions out before deletion; do not retain
  `executeNonCallBoundaryPrefixStep`, `applyResolvedBoundaryEffect`,
  `stabilizedCell` or any `State` application bridge.
- Route-specific diagnostic transport files
  (`boundary_diagnostic_projection.go`, `boundary_diagnostic_transfer.go`,
  `boundary_member_call_diagnostic.go`, `boundary_param_contract.go`) must be
  reduced to immutable observation/contract terms or deleted. They may not
  retain `linkedRelationFrame`/`relationProgramBody` execution.

### Route publication and views

- Delete `transformer/relation_forest_publication.go` entirely.
- Delete `transformer/relation_coordinate_view.go` entirely:
  `StabilizedRelationView`, `StabilizedApplicationKind`,
  `StabilizedApplicationCoordinates`, `ProjectApplications`, `JoinState` and
  the old `(*RelationProgram).Solve(ctx, bodyID, entry state.State)`.
- Delete `transformer/stabilized_route_view.go` entirely:
  `StabilizedCoordinate`, `StabilizedRouteView`, `StabilizedLeafView` and
  independent concrete-coordinate projection.
- Delete or rewrite route-shaped profiling in `transformer/engine_profile.go`
  and `transformer/sparse_projection_trace.go`. Preserve one observational
  profiler, but make its identities formal cell/factor IDs; no invocation,
  dirty-edge or route counters remain.

## Exact production callers to migrate

### Program driver

`program/relation_program_execution.go` is the only production caller of the
exported route view:

- `runPreparedRelationProgram` calls `RelationProgram.Solve`, reads route/body
  work counters and calls `ProjectApplications`.
- `publishRelationProgram` historically indexed stabilized coordinates by
  route and published one child result per route; canonical publication now
  owns exactly one result per lexical body.
- Delete `relationApplicationResult`,
  `relationApplicationLineageInput`, `relationRuntimeApplicationRoutes`,
  `stabilizedApplicationResultVersion`, route maps and `RuntimeRooted`.
- Replace them with one evaluated lexical publication per body: formal result,
  observation sidecar, route-free semantic fingerprint and the
  input-boundary-closed proof. Call-site observations already own caller-edge
  diagnostics; they do not require a child `body.Result`.
- `newRelationProgramExecutionFactories` may remain only as one publication
  authority per prepared body. It must not supply a concrete domain/callback
  bundle to interprocedural evaluation.
- In `program/stats.go`, keep `FunctionalSummaryBodyStats` and the 1/10/100
  scaling contract, but source it from formal cells/equations. Delete
  `ApplicationRoutes` and `ExecutionEnvironments` from `program.Stats`; retain
  only cheap formal Apply count versus body-owned work.

### Body publication/lineage

- `body/result_publication.go`: remove
  `ResultPublicationConfig.ApplicationDependencies` and the corresponding
  parameters after route-free fingerprint authority exists.
- `body/result_accessors.go`: delete `ApplicationDependency` after its lineage
  consumers migrate. The per-route child-result field and accessors are already
  gone.
- `body/result_version.go`: delete
  `computeResultVersionLineageWithApplications`,
  `canonicalApplicationDependencies`, `sameApplicationDependencyEdge` and
  `writeApplicationDependencies`. The lexical formal artifact fingerprint is
  lineage; a caller route is not.
- Diagnostics and manifests traverse `FunctionResults` directly under
  single-body observation ownership; no duplicate-result suppression remains.
- `body/call_argument_trust.go`:
  `CallerOwnedRootParameterContract` reads the formal input contract proof
  directly.

No other production package references the retired stabilized route views.

## Tests: migrate semantic assertions, delete implementation assertions

### Rewrite against the sole formal evaluator

- Program semantic/publication tests:
  `relation_program_compound_call_test.go`,
  `relation_program_definition_input_test.go`,
  `relation_program_execution_test.go`,
  `relation_program_guarded_return_test.go`,
  `relation_program_param_outcome_test.go`, `result_version_test.go`, and the
  call-context assertions in `program_test.go`.
- Preserve `relation_program_scaling_test.go` as the primary gate, but make its
  body cells/equations/evaluations come from `formalRelationRegionInventory`
  and `region` stats. For 1/10/100 callers those three counts must be identical;
  only formal Apply substitutions may scale.
- Transformer end-to-end semantic tests currently calling `Solve`,
  `ProjectApplications` or `newRelationForestProgramScheduler`:
  `boundary_diagnostic_producer_test.go`,
  `boundary_path_obligation_projection_test.go`,
  `boundary_typed_semantics_test.go`, `branch_proof_term_test.go`,
  `compiler_branch_relation_transaction_test.go`, `dynamic_read_test.go`,
  `guarded_root_assignment_object_primary_test.go`,
  `irreducible_multi_entry_red_test.go`,
  `relation_call_outcome_projection_red_test.go`,
  `relation_external_call_producer_test.go`,
  `relation_program_allocation_transaction_test.go`,
  execution assertions in `relation_program_test.go`, and
  `structural_topology_test.go`.

These tests retain their semantic expectations (registered axes, guards,
effects, observations, recursion, allocation identity, cancellation), but
assert one lexical formal result instead of route coordinates.

### Delete with the route implementation

- `relation_coordinate_scheduler_test.go`
- `coordinate_dirty_scheduler_test.go`
- `relation_forest_definition_equations_test.go`
- `relation_forest_publication_test.go`
- `relation_forest_recursive_test.go` route/cut assertions
- `relation_invocation_plan_test.go`
- `relation_guard_forest_vocabulary_test.go`
- route-owned portions of `guarded_world_test.go`,
  `guarded_coordinate_factor_test.go`, `guarded_cross_axis_dependency_test.go`,
  `guarded_sparse_presence_test.go`, `guarded_choice_refinement_test.go`,
  `guarded_dynamic_read_sparse_test.go`, `guarded_relation_step_test.go`,
  `guarded_boundary_receiver_test.go`, and
  `guarded_boundary_output_order_test.go`.

Retain any general ROBDD algebra law by moving it to tests of
`decision_diagram.go`; do not preserve a route-world fixture merely to keep a
test compiling.

## The two required publication authorities

Neither authority exists today; `rg` finds no `InputBoundaryClosed` producer.
The current `stabilizedApplicationResultVersion` is a 64-bit digest of concrete
per-route states, so it is explicitly not the replacement.

### Route-free semantic fingerprint

The fingerprint is one full-width, canonical artifact identity per lexical
body. It covers:

1. body identity and relation/artifact schema version;
2. canonical registered formal product factors after stabilization;
3. guarded observation/evidence sidecar;
4. explicit external/import/native artifact identities; and
5. lexical callee relation artifact identities in canonical relation-variable
   order (SCC-normalized, never recursively hashed through a call stack).

Hash bytes are an index; collision-safe equality is canonical bytes/structure.
Do not widen `state.SemanticFingerprint`'s `uint64`, and do not include root
route, call occurrence, invocation namespace, scheduler edge or caller state.

### `InputBoundaryClosed`

This is a typed proof minted only by exact final projection/elimination. It
certifies that every retained dependency is one of:

- an `IN` formal root from the lexical body's sealed input schema;
- an `OUT` result/effect/observation root;
- a concrete constant/identity; or
- an explicit external artifact dependency.

No `MID`, target-local coordinate, unresolved environment term, caller
`keyspace` key, route/invocation identity, concrete entry `State`, or
allocation template without its formal authority may survive. The proof must
own the exact canonical payload it certified; a boolean set by the driver is
not authority.

Only a publication carrying both this proof and the route-free fingerprint may
be passed to `ExecutionFactory.PublishResult` or the summary artifact builder.
Until then, deleting `ApplicationDependency` would erase real lineage
information; bridging it with zero versions or a synthetic route is forbidden.

## Smallest sequence with no parallel executable engine

1. **Non-executable foundation (current phase).** Finish the formal carrier,
   IN/MID/OUT substitution, `InputBoundaryClosed`, route-free fingerprint
   encoder, and frozen inventory/WTO validation. These may be constructed and
   law-tested, but there is no formal `Evaluate` entry point and production
   still has exactly the existing route engine.
2. **Prepare one atomic source change.** In one change that is allowed to be
   temporarily non-compiling while assembled:
   - add the sole formal transfer adapter from `relationCode` cells to
     `region.RunPrepared`;
   - replace old `RelationProgram.Solve` with the formal evaluator (do not keep
     the old signature or another exported method);
   - rewrite `runPreparedRelationProgram`/publication to consume one lexical
     result per body and require both publication authorities;
   - migrate the semantic tests listed above;
   - delete every route runtime/view/scheduler file and symbol listed above;
   - delete application lineage and call-context fields in the same final
     compile once the two authorities are wired.
3. **Compile only the final side of the cut.** There is no build tag, config,
   environment switch, audit dual-run, fallback resolver or compatibility
   adapter. If formal transfer coverage is incomplete, the cut does not land;
   it does not call the deleted engine.
4. **Gate the sole engine.** Run the 1/10/100 scaling test, transformer/program
   tests and then the full unskipped oracle. Only after the oracle is green run
   cold Kickside under `GOMEMLIMIT=3GiB` and the 4 GiB RSS fuse.

The compilation boundary is therefore singular: before the atomic change only
the route runtime can evaluate; after it only the formal WTO/region runtime can
evaluate. Static formal schemas before the cut are not an executable second
engine.
