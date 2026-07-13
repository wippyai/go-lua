# Final engine architecture v4: guarded core plus complete checker/service surfaces

Status: final POC and migration contract. This adds repository ownership to v3; it does not weaken guarded precision, termination, or deletion gates.

## 1. Decision and honest limit

The existing domains, 17 lanes, reducers, summaries, judgments, manifests and oracle remain. One declarative transaction language, one neutral program IR and one region scheduler replace repeated body solving and duplicate interprocedural machinery.

For arbitrary non-distributive transfer `F`, exact independent-context precision, finitely bounded contexts and near-zero evaluation cannot all be guaranteed. The engine retains guarded application/provenance disjuncts; only certified merges claim equivalence. Versioned guarded widening is sound but may change theoretical precision. Shipping requires no fixture/oracle/frozen-corpus quality regression, reviewed diagnostic diffs, zero failed/skipped units and measured corpus improvement. No Top, deadline skip or legacy fallback is accepted.

## 2. Acyclic layers and ownership

```text
semantic/schema       IDs, descriptors, inventories, ordinals, codecs
semantic/primitive    declarative microprograms
semantic/transaction  FrozenTransaction, capabilities, routes/outcomes/resources
semantic/program      sole neutral blocks/WTO/policies/observations/canonical codec
lua/transferfacts     sole Lua/WIR adapter -> semantic/program
engine/state          concrete carrier + generated local glue
engine/slotbank       private kind+index SlotRef; typed/parametric banks
engine/region         sole scheduler; consumes semantic/program
engine/concrete       concrete transaction backend
engine/circuit        guarded circuit backend; never imports concrete
engine/config         exact runtime composition/seal
check/projection      high-level semantic product graph
check/service         sole host/session/invalidation/publication/query boundary
artifact/{key,codec,cas} generic typed storage only
```

`semantic/program` owns regions, blocks, WTO nodes, widen/narrow policy, transactions, mixed routes, resources, outcomes and observation slots. Observation terms form a guarded annotation sidecar: they publish atomically with semantic cells but never participate in semantic row equality, widening or SCC convergence. Neither backend nor `check` defines another region model.

## 3. Exact inventories, universes and runtime ABI

Reviewed `BaseInventory` (`analysis/semantic/inventory/builtins.yaml`) drives a hermetic type-checking generator. State fields/lane bits/unexported accessors are emitted inside `analysis/engine/state/zz_generated_*.go`; backend dispatch is emitted in backend packages. CI regenerates and compiles synthetic lane add/remove cases. Marker/init/import discovery is forbidden.

Runtime user-lattice instances form a canonical sorted `ExtensionInventory`. Every instance contributes descriptor, codec, immutable config, projection and instantiated leaf coverage; registration order cannot alter bytes/ordinals. Coverage expands carrier/component/subcomponent to leaves plus reducer closure and validates `primitive/effect/output/observer × leaf`; no carrier verdict covers children.

```text
DescriptorUniverseID = H(base inventory, extension inventory,
                         leaf descriptors, coverage, schemas/codecs)
RuntimeUniverseID = H(DescriptorUniverseID, exact backend/projection bindings,
                      AnalyzerSemanticBuildID)
```

`schema.Freeze` owns the first; `config.SealRuntime` proves exact descriptor/implementation/config bijections and owns the second. Circuit/evaluated/analysis keys require RuntimeUniverseID.

Transactions use verifier-created private handles. `slotbank.SlotRef{kind,index}` has private fields; generated built-in typed banks and one packed verified-extension bank validate kinds. `ScopedBinding` exposes only transaction handles, never raw State, Universe, arena, ordinal or unrestricted callee provider.

## 4. Canonical lexical lowering: evolve, do not duplicate

`analysis/lua/transferfacts/**` evolves as the **sole Lua/WIR semantic adapter**. It produces `semantic/program.Program`; `analysis/ir/lexicalprogram` is a façade/assembly namespace, not a second WIR interpreter.

- `engine/factflow/**`: retain immutable syntax-free descriptors while used; migrate each descriptor into primitive/program schema; delete DTOs when census reaches zero.
- `engine/operationplan/**`: retain dense lexical plan as staging input; generated neutral blocks/transactions replace it descriptor-by-descriptor; delete only at zero census.
- `engine/factquery/**` and `engine/visibility/**`: retain as lexical dominance/definition preprocessing below `check`; they are not runtime scheduling/context machinery.
- `engine/sourcevalue/**`, `sourceprojection/**`, `typenarrow/**`: census every semantic rule. Move executable meaning to declarative primitive/intrinsic programs; retain pure lexical descriptor construction; delete duplicate runtime evaluators after differential closure.

Every old fact/opcode has exactly one final classification: retained lexical producer, moved primitive/transaction descriptor, observation compiler, boundary provider, or deletion. Two lowerers for one meaning are forbidden.

## 5. Transaction and call-semantics migration

Primitive authority is declarative microcode. A necessary native intrinsic has one pure generated typed concrete specialization; circuit IR only reifies `IntrinsicCall` and invokes that same authority at specialization. Independent concrete/symbolic meanings are forbidden.

`analysis/engine/factapply/**` is the concrete interpretation during migration. Its ordered node/edge updates, index mutation, allocation/rekey, protected typestate, presence closure and user-lattice operations are converted file-by-file into `FrozenTransaction`s with structurally derived reducer-expanded capabilities. Once the shared concrete backend executes those same programs and all differentials pass, duplicate `factapply` execution code is reduced/deleted; descriptor helpers may remain below the adapter.

Call ownership:

- `effectlowering/**`: retained owner for signature/native external-boundary primitive programs and canonical effect derivation.
- `callboundary/**`: retained canonical boundary/output schema until a deliberate schema migration; it is not reimplemented by circuit code.
- `callpayload/**`, `calloutcome/**`, `callproducer/**`: each producer maps to a boundary-provider program or FrozenTransaction; parallel payload application paths are deleted after census closure.
- `fixpoint/program/internal/callresult/**`: allocation/return/outcome materialization becomes transaction specialization/projection, then deletes as call execution authority.
- `.../projectsummary/**`: existing normalized summary schema/transactions are retained projection authority under `check/projection`; any solver coupling deletes.
- `.../relationcall/**`: known-call routing becomes guarded mixed-target Apply; exclusive/fallback shims delete.

Mixed calls carry guarded known lexical alternatives plus explicit unknown/native residue and snapshot-content completeness proof. Outcome-local overlays preserve normal/raised/suspended/nonreturning worlds; explicit commit points and reducer positions define rollback/survival. Alias predicates relate params, shared/distinct capture cells, globals and heap roots; allocation templates atomically rekey all identity-bearing leaves.

## 6. Scheduler disposition

`engine/region` becomes the single worklist/WTO/revision/restart/widen/narrow/observation/cancellation kernel.

- `engine/solve/**`: retain only generic lattice/WTO utilities actually delegated to by region; move the sole scheduler there or delete duplicates.
- `solve/concreteflow/**`: delete after shared concrete backend parity; no second dense scheduler remains.
- `engine/transfer/**`: semantic operations migrate to primitive/transaction programs; scheduling/session/checkpoint machinery deletes.
- body WTO/workplan/observation replay: migrate plans to `semantic/program`; body scheduling and replay delete.
- `fixpoint/query/**` and fixpoint program query scheduling/prepass/context/materialization: delete after SCC authority; no compatibility adapter may schedule work.

The standalone body API invokes the shared concrete region kernel. Only input/output adapters survive.

## 7. Guarded bindings and nested fixed points

A versioned finite `BindingPartitionPolicy` maps Apply/target/finite provenance/alias classes to finite cells. Each cell stores guarded disjuncts `{application guard, provenance guard, binding}`. Certified exact merges must cover all reachable primitives, reducers, outcomes and observers. At the disjunct bound, versioned `PrecisionPolicy` performs sound guarded widening while retaining union application/provenance guards.

Finite descriptors are mechanically verified: enumerated carriers exhaustively model-check order/join/widen/rank; infinite carriers supply verifier-checkable well-founded measures whose widening steps consume bounded ascent budget. Violations are internal failures naming the component. Safety-budget exhaustion aborts publication.

Nested schedule:

1. outer call/resource SCC rounds visit members canonically;
2. each member runs inner LoopMu WTO with current callee approximants;
3. any callee binding/output/route/resource growth recomputes dependent loops from canonical entry—no checkpoint reuse initially;
4. loop widening owns point equations; call-SCC widening owns guarded bindings/interprocedural outcome equations;
5. after product stabilization, joint narrowing uses stabilized predecessors;
6. growth during narrowing discards that round and restarts from the widened outer solution.

Checkpoint reuse requires a later equivalence proof. After semantic stabilization,
a separate finite observation closure unions guarded annotations for equivalent
semantic rows and performs SCC-normalized recursive provenance. Dynamic
call-stack hashes must not grow the observer state without bound. Current
versioned presentation dedup consumes that stabilized observer sidecar.

## 8. Checker projection seam: products, not engine policy

`analysis/check/projection` consumes sealed evaluated roots/observations and is the sole high-level product assembler. It produces:

- normalized summaries and call-boundary products;
- stable public `check/readmodel/**` and internal projection DTOs;
- obligation evidence and `obligation/pass` judgments;
- diagnostic semantic evidence (rendering remains `diagnostics/**`);
- `placementplan/**` compiler licenses;
- `exportmanifest/**` globals/effects/callbacks/object/type products; and
- compiler-facing readmodel/identity DTOs.

`AnalysisArtifactKey` commits individual schema/product digests for public/internal readmodel, summary/boundary, obligation/judgment, diagnostic evidence, placement, export manifest and compiler DTO—not a generic observer hash.

Body-owned occurrence semantics do **not** move into projection decisions. `observation_seal`, send safety, advice guards/loops/split-birth discriminants and `result_*_readmodel` semantic producers migrate into lexical observation compilers/primitive roots. `check/internal/readmodel` remains projection-only. Projection normalizes sealed meaning; it never reconstructs transfer/occurrence semantics.

Deletion of body/program imports from readmodel, obligations, diagnostics, placement and exportmanifest is blocked until every product is byte-differential and its observation/schema census is exact.

## 9. Artifact keys and atomic product graph

```text
CircuitArtifactKey = H(RuntimeUniverseID, sorted member-local lexical keys,
    internal call/resource graph, external typed content IDs, negative proofs,
    BindingPartitionPolicy, PrecisionPolicy, nested schedule, observation schema)

EvaluatedRootArtifactKey = H(CircuitArtifactKey, canonical root seeds,
    entry/alias/capture/global/heap world, keyspace and allocation provenance,
    exact dependency content IDs)

AnalysisArtifactKey = H(RuntimeUniverseID, profile/policies, canonical root set,
    sorted EvaluatedRoot typed digests, SCC condensation/resource graph,
    complete dependency vector, all check/projection product schema digests)
```

Internal SCC members use handles derived from CircuitArtifactKey, never recursive member digests. External/negative proofs use authoritative content IDs, not mutable generations.

Generic `artifact/**` stores typed domain-separated envelopes only. Typed Analysis graph assembly belongs to `check/projection` plus service. Changed circuits/roots/products build in one service-owned scratch transaction; validate dependency closure, negative authorities, observation/source-map bijections and every product digest; atomically publish one immutable Analysis envelope/snapshot. Lint and Compiler projections consume that typed analysis digest.

## 10. Service and LSP ownership

`analysis/check/service/**` remains the sole embedding/composition/publication layer:

```text
embedding UnitInput + resolution/content snapshot
 -> transferfacts/semantic program + config.Runtime
 -> circuit/evaluated CAS scratch transaction
 -> check/projection product graph
 -> immutable service snapshot/query APIs
```

Migrate `solve.go`, `session.go`, `snapshot.go`, `debug_map.go`, `semantic_projection.go` and query APIs individually. Artifact content IDs compose with `analysis/embedding/**` document/resolution identities; document versions are publication metadata only. Service owns invalidation, sessions, transient release, runtime construction, atomic generation swap, debug/source-map bijection and public semantic queries.

`analysis/lsp/**` imports service/embedding/diagnostic public surfaces only. Architecture gates forbid LSP imports of engine, config, circuit or CAS.

## 11. Exact census and gates

Generate a checked-in surface census with one row per file/exported semantic symbol across transferfacts, factflow, operationplan, factquery, visibility, sourcevalue/sourceprojection/typenarrow, factapply, effect/call families, body occurrence producers, schedulers, fixpoint internals and all projection/service consumers. Columns: current owner, final owner, primitive/observation/product ID, schema, differential, retained/moved/deleted status. Missing or multiple ownership blocks flip.

Architecture import gates enforce: lexical producers below check; no second WIR lowerer; semantic/schema/program backend-neutral; projection cannot import body/query/schedulers; service is sole CAS/runtime publisher; LSP service-only; shipped program analysis cannot reach legacy schedulers/body solve.

POC uses frozen threads SCC rooted at `M.catch_up_projection`, plus mutual-recursion/loop, alias+capture+recursive-allocation and actor/protected-outcome microcases. Compare actual merged/widened production cells and every high-level product against individual legacy contexts. Report residual transfers/CPU per guarded cell versus old solve, disjunct growth/merges/widens, loop/call rounds, observation and projection cost, product bytes, RSS and full cold/warm Kickside.

Before commitment: no quality regression, reviewed diagnostics, product byte parity, materially lower real work, zero failures. Final counters: zero prepass/summary/materialization/body solves and zero known-call fallback.

## 12. Deletion and WIP

After exact census/import/product gates, delete legacy query/context/materialization, concreteflow/transfer schedulers, body worklist/WTO/replay, solve caches, activation shims, duplicate callresult/payload application, hand-written State catalog and any factapply execution path superseded by shared transactions. Retain visibility/factquery and lexical descriptor helpers only where the census names them.

Uncommitted identity and allocation/WTO-row prototypes remain quarantined. Salvage tests/codec evidence only. Identity becomes typed Circuit/EvaluatedRoot/Analysis keys; allocation returns as atomic transaction/template; WTO lives in the shared region kernel. Committed relation/WTO work remains migration oracle/baseline, not final authority.

## 13. Measured baseline and performance budget

The current accepted end-to-end baseline is a cold standalone lint of Kickside
from `/home/wolfy-j/kickside/kickside/app`, using the reconstructed canonical
binary. Commit `7ac918e28` processed all 1,891 then-present entries in **2:14.07
wall**, consumed **1,202.71 user-CPU seconds** plus 33.14 system seconds at 921%
aggregate CPU, and peaked at **1,222,588 KiB RSS**. It reported 2,365 errors and
38 warnings with zero deadline-failed or unchecked entries. Corpus entry and
diagnostic counts are not stable while Kickside is being edited, so performance
comparisons must intersect physical sources and record the input snapshot.

The first strict relation phase-collapse experiment at `61d42a154` was rejected
by this gate. It processed the then-present 1,889 entries in **3:07.06 wall**,
used **1,510.16 user-CPU seconds** plus 44.84 system seconds at 831% CPU, and
peaked at 1,191,952 KiB RSS. Among 1,885 comparable timed entries, 1,812 slowed
and their summed durations grew by roughly 428 seconds. The mechanism preserved
semantic and `ResultVersion` parity, but globally enabled context certificates,
compiled transformer plans for owners later rejected, and repeatedly rescanned
call surfaces before admitting only a small call-free slice. Commit `d06381635`
therefore returned it to an internal gate. This is a design measurement, not a
production optimization.

The important shape is not broad parser or loader cost. About 1,300 entries can
finish in roughly the first second, after which a small heavy tail dominates.
The frozen four-offender sample performed 1,691 body solves and 64,671 transfers
for 305 lexical bodies whose once-per-body floor is 11,297 transfers. About 82%
of that work is structurally removable. Its phases were:

| Phase | Body solves | Transfers |
| --- | ---: | ---: |
| prepass | 460 | 19,072 |
| summary | 619 | 24,407 |
| materialize | 612 | 21,192 |

The frozen threads offender currently costs about 1.17–1.21 seconds: parse and
bind about 2.6 ms, preparation about 51 ms, prepass about 250 ms, summary about
385 ms, and materialization about 528 ms. It performs 271 solves and 11,882
transfers for 41 lexical bodies with a once-per-body floor of 2,206 transfers.
This evidence is why the design removes repeated body solving rather than
removing axes or weakening semantics.

Acceptance targets are empirical and conjunctive:

- no solve-failed, deadline-skipped, or silently unchecked unit;
- fixture, oracle, lattice, soundness and frozen-corpus gates remain green;
- no unreviewed diagnostic or manifest change;
- pathological production files approach **less than one second**;
- full cold Kickside reaches **30–40 seconds** on the reference machine;
- warm lint and compilation approach typed artifact load plus projection cost;
- memory is bounded by the live working set and immutable retained artifacts,
  not the sum of all transient entry states.

The 30–40 second objective is not claimed by design arithmetic alone. Each
migration stage must publish cold/warm wall, CPU, RSS, allocation, solve,
transfer, fixed-point and diagnostic results from the real Kickside corpus.
Counts that exclude relation preparation, prepass solves, transformer
compilation, admission scans, or rejected owners are not promotion evidence.

## 14. Frozen validation corpus and commands

Kickside is an external validation corpus and must never be edited to make the
analyzer pass or benchmark better. The primary pathological contract is the
threads SCC rooted at the real `M.catch_up_projection`; its frozen source lives
at `/tmp/frozen-threads-contract.lua` for isolated reproduction. The frozen
four-offender set and the three semantic microcases in section 11 remain
mandatory throughout migration.

Representative end-to-end command:

```bash
cd /home/wolfy-j/kickside/kickside/app
/home/wolfy-j/wippy-canonical lint -c --cache-reset --timings
```

The benchmark record must include the exact go-lua commit, binary digest,
runtime adapter commit, machine/worker count, cache state, input root digest,
wall/user/system time, peak RSS, allocation profile, per-entry timings,
deadline/failure list, diagnostics digest and manifest/product digests. A result
without those fields is directional evidence, not a release gate.

The repository gate remains the complete build/vet/test/fixture/meta/oracle/
soundness/lattice/architecture/fuzz suite. The exact commands should be read
from the repository gate scripts at execution time rather than copied into this
document and allowed to drift.

## 15. Migration program and stop/go gates

This is an incremental replacement at the execution seam, not a rewrite of the
domains. Every stage is independently reviewable and keeps one semantic
authority.

### Stage A — freeze truth and ownership

1. Check in the generated semantic surface census and make missing or duplicate
   ownership a CI failure.
2. Freeze the threads/four-offender evidence, full Kickside benchmark envelope,
   and diagnostic/manifest/product digests.
3. Land `BaseInventory`, `ExtensionInventory`, universe IDs, generated State
   glue, synthetic add/remove-axis compile tests and import architecture gates.
4. Quarantine current identity/allocation prototypes. Salvage only tests and
   deterministic codec evidence after review; do not merge their execution
   architecture.

**Gate:** inventory changes are deterministic, axes can be added/removed without
editing scheduler logic, every current semantic producer has one disposition,
and all existing gates remain green.

### Stage B — prove one primitive and one scheduler

1. Implement capability-scoped `FrozenTransaction` and the four representative
   modules: a scalar built-in, reducer-coupled built-in, parametric user lattice,
   and identity/allocation-bearing module.
2. Execute the same primitive microprogram through the legacy differential
   harness, shared concrete backend and circuit reification/specialization.
3. Introduce the sole region kernel and run the existing concrete body API on
   it without changing public products.
4. Measure primitive bytes, transactions, nodes, allocations and region work.

**Gate:** byte/semantic differential parity, reducer closure, rollback/outcome
parity, no independent symbolic meaning, and materially no scheduler regression.

### Stage C — guarded interprocedural POC

1. Build guarded Apply, target residue, provenance/alias partitions, LoopMu and
   CallSCCMu over neutral programs.
2. Implement relational captures/world roots and atomic allocation rekey.
3. Prove finite partition/widening contracts and explicit exact-table/
   reduced-circuit certificates.
4. Run mutual recursion plus loop, alias/capture/recursive allocation, and
   actor/protected-outcome microcases.
5. Run the frozen threads SCC. Compare each production merged/widened cell and
   all observations/products against every corresponding legacy context.

**Gate:** total execution, zero fallback/skip, no quality regression, bounded
cells, and a substantial reduction in residual transfers and CPU. If threads is
not materially faster, stop and diagnose the model; do not broaden migration.

### Stage D — authoritative SCCs and typed artifacts

1. Move eligible SCCs to the circuit authority only when their census rows,
   transactions, observations and projections are complete.
2. Publish Circuit, EvaluatedRoot and Analysis artifacts in the single typed CAS
   with atomic graph publication.
3. Migrate high-level products through `check/projection`; retain service as the
   sole session/invalidation/publication/query owner.
4. Expand by measured offender family, rerunning full cold Kickside after every
   meaningful optimization pass.

**Gate:** authoritative SCCs have no legacy scheduling reachability; artifact
misses reproduce byte-identical products; artifact hits perform no semantic
solve; full diagnostics remain complete.

### Stage E — compiler v2 handoff and final deletion

1. Export the immutable, versioned CompilerProjection from the same Analysis
   artifact before transient release.
2. Let v2 lowering/codegen consume that DTO and publish separately keyed
   CodegenArtifact and AdmissionCertificate nodes.
3. Validate the integration on the v2 tree at `100.70.10.28` / `wippy2` without
   giving runtime generations CAS ownership semantics.
4. After the exact zero-census/import/product gates, delete every legacy path
   listed in section 12. The final system has no opt-in legacy analyzer path.

**Gate:** a valid Analysis artifact is shared by lint and compile; compilation
does not pay semantic analysis again. Policy-only lint changes reuse analysis;
compiler/VM/ABI changes invalidate codegen but not analysis; admission-policy
changes invalidate admission only. Existing live actor generations retain their
normal CodeGraph lifetime and are never freed by CAS eviction.

## 16. v2 single-cache contract

“One cache” means one physical typed content-addressed store and one canonical
semantic solve, not one unversioned blob. The durable graph is:

```text
semantic inputs -> circuit -> evaluated roots -> analysis artifact
                                             |-> lint projection
                                             |-> compiler projection -> codegen -> admission
```

The analyzer key excludes diagnostic wording, severity, display labels and
document publication versions. It includes exact source/import/runtime semantic
content, RuntimeUniverseID, policies that change analysis meaning, schemas and
the analyzer semantic build. Compiler projection is an immutable DTO with
stable body/operation/value/allocation/call identities, facts, licenses,
bindings and debug anchors; it cannot expose mutable checker state.

Codegen keys additionally fence lowering/transforms, runtime-module ABI,
optimizer/compiler build, VM Proto ABI, backend and operational-map schemas.
Admission remains a separate proof keyed by codegen plus the complete host
policy. Runtime artifact generations reference CAS digests but continue to own
live executable lifetimes, dependency generations and retained actor resources.

The more detailed integration contract remains
[`v2_analysis_artifact_cache.md`](v2_analysis_artifact_cache.md); this document
is authoritative where the guarded engine architecture is more recent.

## 17. Current repository state and handoff discipline

The sound committed baseline includes immutable capture certification, exact
guarded relation contexts and closed local-function use proofs. These commits
are migration evidence and regression oracles, not permission to preserve
duplicate final execution paths.

The current worktree also contains uncommitted canonical-identity,
literal-allocation and WTO-row experiments. They are deliberately not approved
for commit. Do not bulk-clean or commit them: distinguish agent WIP from the two
pre-existing WTO-row prototypes, preserve useful red/codec tests, and migrate
only evidence that satisfies this document’s ownership rules.

Implementation commits must be small enough to review one semantic ownership
move, must state which census rows close, and must include their differential and
performance evidence. Do not push a partial mixed-authority design to the v2
consumer. The compiler-facing integration is delivered as one reviewed, clean
commit once its producer schemas and cache contract are complete, so the v2
side can pick it up without inheriting experimental history.

## 18. Non-negotiable definition of done

The work is complete only when all of the following hold simultaneously:

- all required actor-model axes and their current oracle semantics remain;
- adding/removing an axis is inventory/schema work, not scheduler surgery;
- each primitive has one executable meaning and every leaf has exact coverage;
- each program region is scheduled by one kernel;
- calls compose guarded reusable circuits instead of re-solving callee bodies;
- lint and compile share the same immutable Analysis artifact;
- no legacy analyzer path, shortcut cache, opt-in semantic mode or silent
  deadline fallback remains;
- every unit is checked with reviewed diagnostics and products;
- cold and warm Kickside targets are measured from the real corpus; and
- legacy machinery is physically deleted only after mechanical closure proves
  it has no semantic or consumer ownership left.

## 19. PromptMap architecture decision (2026-07-12)

A wide PromptMap audit of the program, query, summary, transformer, body,
region, circuit, operation-plan and semantic-program surfaces confirmed that
the product decomposition is not the performance defect. The multiplicative
path is orchestration: prepass, whole-body summary equations, concrete
parameter-feedback re-solves and final materialization all invoke the body
solver.

Parameter substitution therefore belongs at the symbolic Apply edge. One
immutable relation is compiled per lexical function using distinct parameter,
capture, global, result and heap-template roots. Known calls compose those
relations with `ComposeDirectCallRows`; call/resource SCCs converge relations;
summaries and guarded diagnostic evidence are projected once from stabilized
evaluated roots. Dense path keys remain structural until evaluated-root
specialization.

The regional parameter-feedback prototype is retained only as a correctness
oracle: on the representative scanner family it removed 15 false positives but
added about 18 percent wall time because three concrete summary rounds were
still required. It is not production authority. The immediate production
slice is whole-function admission for acyclic, fixed-arity direct lexical
functions with exact parameter roots, supported guards/path reads and exact
allocations. Unsupported functions remain wholly on the old authority during
migration; admitted functions may not fall back internally. The old prepass,
feedback, query-body-solve and rematerialization authorities are deleted after
the symbolic census reaches zero.
