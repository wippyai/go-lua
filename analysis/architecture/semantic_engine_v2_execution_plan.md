# Final engine architecture v4: guarded core plus complete checker/service surfaces

> **SUPERSEDED MIGRATION CONTRACT — 2026-07-14**
>
> This document's staged side-engine/legacy-differential migration is not an
> authorized implementation strategy. It produced parallel orchestration,
> duplicate vocabulary, a false-green oracle signal, and delayed deletion.
> Retain the domain inventories and final ownership analysis below as design
> evidence only.
>
> The non-negotiable production invariant is:
>
> - one analysis engine, one semantic fixed point, and one publication path;
> - no shadow/evaluated/strict/staged/legacy production implementations;
> - no fallback from unsupported new machinery to an older solver;
> - a foundational replacement deletes the superseded orchestration in the
>   same reconciliation, rather than carrying two paths until a later flip;
> - every discovered curated fixture is a hard gate over that sole path; fixture
>   manifests cannot exempt semantic failures from the oracle;
> - the codebase and vocabulary must shrink during reconciliation;
> - arbitrary depth, row, iteration, time, or node caps are not semantic
>   termination mechanisms.
>
> Temporary comparison tooling may live only in tests and may compare the sole
> candidate implementation with a frozen baseline. It must never become a
> second production execution path or make a rejected candidate appear green.

Status: superseded as a migration contract; pending one-engine reconciliation.

## 1. Decision and honest limit

The existing domains, 17 lanes, reducers, summaries, judgments, manifests and oracle remain. One declarative transaction language, one neutral program IR and one region scheduler replace repeated body solving and duplicate interprocedural machinery.

For arbitrary non-distributive transfer `F`, exact independent-context precision,
finitely enumerated concrete contexts and near-zero evaluation cannot all be
guaranteed. The final engine therefore does not enumerate or cap concrete
contexts: it compiles one guarded symbolic relation per lexical function and
uses lexical call-frame and loop/call-SCC mu binders. Only the ordinary abstract
domains own widening. No provenance-stack approximation, disjunct-count budget
or precision-policy fallback is part of semantic termination. Shipping requires
no fixture/oracle/frozen-corpus quality regression, reviewed diagnostic diffs,
zero failed/skipped units and measured corpus improvement. No Top exhaustion,
deadline skip or legacy fallback is accepted.

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

## 7. Guarded symbolic relations and nested fixed points

Each lexical function first freezes one typed `WorldProgram`: lexical CFG
topology, ordered closed transactions, structural choices, observation sites
and call occurrences. This is input IR for the sole symbolic solve, not the
object applied at call sites. One joint solve per call/resource SCC reduces its
member programs to immutable guarded symbolic `Relation`s. A Relation is the
normalized parametric boundary-world transformer; its roots distinguish
parameters, captures, globals, results and allocation templates.

A lexical call frame stores only an `ApplyRef` to a callee Relation variable
plus the inbound/outbound boundary lens, guard, correlated result selectors and
call-site allocation alpha-renaming. Applying it never enters, clones or solves
the callee WorldProgram. This is the performance boundary: function-body
transfers execute during the one SCC reduction, independent of caller contexts;
call sites compose the already-reduced boundary relation. Lexical `LoopMu` and
call-SCC mu binders are the only recursion vocabulary.

The reduced relation uses hash-consed value/path/guard/world terms. Choice and
phi/select nodes remain circuit structure rather than DNF row enumeration.
Circuit nodes are finite because they are derived from lexical CFG points,
facts, call occurrences and WTO components; recursive edges revisit binders.
There is no disjunct-count, node-count, call-depth or context budget.
Termination of the one symbolic reduction is owned by the registered abstract
domains' lattice laws and widening/narrowing, not by an orchestration cap.

Nested schedule:

1. outer call/resource SCC rounds visit member WorldPrograms canonically;
2. each member reduces its inner LoopMu WTO with current symbolic callee
   Relation approximants;
3. any callee binding/output/route/resource growth recomputes dependent loops from canonical entry—no checkpoint reuse initially;
4. loop widening owns point equations; call-SCC widening owns guarded bindings/interprocedural outcome equations;
5. after product stabilization, joint narrowing uses stabilized predecessors;
6. growth during narrowing discards that round and restarts from the widened outer solution.

Checkpoint reuse requires a later equivalence proof. Call application itself
never uses checkpoints because it does not execute point programs. After semantic
stabilization, a separate finite observation closure unions guarded annotations
for equivalent symbolic outcomes and uses lexical mu backedges for recursive
occurrences. No dynamic call-stack hash or provenance string enters semantic or
observer identity. Presentation dedup consumes that stabilized observation
sidecar.

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
    symbolic binder schema, nested schedule, observation schema)

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

The staged call-free planner then removed the broad tax: it performs cheap
owner filtering before transformer preparation and remains off by default. On
the frozen threads contract it scanned 41 lexical owners, compiled and
activated zero, preserved all 271 body solves/11,882 transfers and exact
diagnostic/summary products, and ran within ordinary noise of the roughly
two-second legacy result. This proves the staged funnel is cheap, but also that
call-free leaves cannot move the pathological workload. The next value-bearing
slice is immutable symbolic plans for parameterized, captured, or repeatedly
contextual owners, admitted through a complete call graph/SCC transaction.

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

**Gate:** total execution, zero fallback/skip, no quality regression, a finite
static circuit with no dynamic node growth after freeze, and a substantial
reduction in residual transfers and CPU. If threads is not materially faster,
stop and diagnose the model; do not broaden migration.

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

The committed baseline contains reusable capture, identity, state-boundary and
cycle-exact domain foundations. The replacement relation engine is uncommitted
and incomplete: its structural root, call-frame and loop-mu terms compile, but
its frozen program is not yet the production executor and does not yet satisfy
the semantic oracle. The old program solver remains executable only until the
atomic cutover; passing results from that solver are not evidence for the
replacement.

The worktree contains quarantined WTO-row prototypes. They are not the final
representation and must not influence production termination or authority.
Preserve their useful counterexamples while replacing row/DNF propagation with
one relation-owned hash-consed program circuit; then delete the prototypes,
their row budgets and their APIs.

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
capture, global, result and heap-template roots. A relation owns one
`ProgramRoot` into a hash-consed circuit of typed ordered world transactions,
shared choices, lexical call frames, returns and loop-mu references. Known
calls bind the callee relation variable; they do not unfold a body or enumerate
caller contexts. Call/resource SCCs and loop WTO components are nested parts of
the sole semantic fixed point. Summaries and guarded diagnostic evidence are
projected once from stabilized roots. Dense path keys remain structural until
that projection.

Production publication is rooted at the program entry, not at one
declared-contract root per lexical body. The stabilized transaction owns a
structurally shared observer forest whose nodes bind `{callee cell, call
occurrence, guard, complete boundary environment}`. Reachable diagnostic
instances are keyed by body owner plus cycle-normalized invocation route;
declared contracts create validation-only instances according to the existing
reporting policy and never replace reachable caller worlds. Recursive observer
backedges are finite mu/closure nodes committed atomically with their relation
SCC. Final projection evaluates this frozen forest once; it does not solve a
body or start a second semantic fixpoint.

The compiled projection inventory is the `callOutcome=true` superset. Call
outcomes carry every registered outcome lane (including suspension, normal
return facts, heap/placement, obligations/refinements, presence relations and
exposures); narrower consumers may project the stabilized superset but may not
change compilation or trigger recomputation. A body-only root lookup is not a
production API because caller-entry context is part of its semantic identity.

The regional parameter-feedback prototype is evidence only and is not a
production authority. There is no per-function admission split: the cutover
occurs only when the replacement handles the complete registered semantic
surface. In the same cutover, the old prepass, feedback, query-body-solve,
summary and rematerialization authorities are deleted. No unsupported function
may silently select the old engine.

## 20. Semantic recursion-cap disposition (2026-07-15)

Depth limits are not repaired inside authorities scheduled for deletion. The
following ownership map is part of the one-engine cutover:

- `fixpoint/program/materialization_flow.go` deletes with replay and
  rematerialization. Its recursive type helper is not migrated from that file.
- `RetainedBudget`, `RetainedSystem`, retained regional updates and checkpoint
  publication delete with the old program/body/transfer retained-summary path.
  Budget exhaustion currently discards retained work and falls back to a full
  body solve; it is neither cache eviction nor part of the tuple-mu kernel and
  must not be moved or uncapped into the replacement.
- Parameter-obligation projection retains its meaning but moves from
  `fixpoint/program/internal/projectsummary` to `check/projection`. It becomes a
  demand-driven least fixed point over finite source/path dependencies keyed by
  wanted semantic identity and capture mode; `product.Join` accumulates facts.
- Record/member and array/dynamic-index proofs move from `check/body` into
  shared `lua/typeprojection` / `lua/typeprojection/indexproof` queries. May and
  existential queries use least fixed points. Must and universal queries use
  greatest fixed points conjoined with a separately computed least-fixed-point
  productivity proof, so an unproductive pure cycle proves nothing.
- Concat, static-member, non-nil, advice and nilable-assignment occurrence
  walkers become finite iterative lexical observation compilers. AST size, not
  an arbitrary recursion budget, proves termination.
- Expression and assignment labels are presentation only. Structural
  occurrence/path identities own grouping and deduplication; renderers may have
  an explicitly named display-byte policy but presentation truncation can never
  alter semantic identity or analysis.

No `DefaultRecursionDepth` replacement may be implemented in old program/body
orchestration merely to make a raw cap census green. A retained query must move
to its final shared owner with an exact finite graph algorithm; a superseded
query must disappear with its owner.

## 21. Sole relation carrier and call cost model (2026-07-15)

Let `D` be the complete registered `state.State` lattice and `K` the sealed
equation/route/outcome inventory. Guarded control distributes exactly over that
finite product:

```text
GuardedState = ROBDD(GuardAtom, D)
World        = ordered K -> GuardedState
R_f          : BoundaryInput_f -> World_f
```

This is the canonical transpose
`GuardAssignment -> (K -> D) ~= K -> (GuardAssignment -> D)`, not an
approximation. Every coordinate root uses one shared decision kernel, global
atom order and interned State-terminal table, so common atoms retain exact
cross-coordinate correlation without copying a complete `K` vector into each
terminal. The BDD is finite shared control structure, not an SMT solver. Edge-owned typed
transactions refine State and turn infeasible leaves into State Bottom. Binary
lattice operations align ordered guard atoms with memoized BDD Apply and use the
complete State lattice at leaves, pointwise over dirty coordinates.
`ITE(a,x,x)` reduces to `x`; there is no row/DNF enumeration or semantic node
budget.

For call/resource SCC `C`, the sole semantic equation is a simultaneous tuple
over its function-entry cells and every reachable lexical `LoopMu` cell:

```text
R_C = mu_widen <X_function, X_loop> .
    F_C(<X_function, X_loop>, ApplyRef, frame)
```

External dependencies are already-stabilized relations. Internal calls refer
to the vector-mu variable; they never unfold a callee. Flattening the lexical
loop equations into the same tuple is the constructive fixed-point
decomposition, not a change in semantics. In particular, no member evaluation
may run an inner loop-to-stability process that restarts after a callee grows.
Widening occurs only at lexical-loop and call-SCC feedback vertices over
complete worlds. Narrowing starts after the whole product stabilizes, continues
to equality without a pass count, and any growth abandons narrowing and resumes
ascent from the widened solution.

The frozen dependency graph is at `(relation node, invocation route, outcome or
loop-head coordinate)` granularity. A changed coordinate schedules only its
reverse-reachable acyclic slice; rescanning a member's whole relation circuit
on every tuple round is the old body-solve multiplier and is forbidden.

The cost/ownership boundary is strict:

- once per lexical function, freeze its typed WorldProgram;
- once per dependency revision, reduce affected member programs jointly to
  normalized parametric Relations;
- once per static call occurrence, freeze only an `ApplyRef` and boundary lens;
- at root specialization, demand-evaluate stabilized Relation nodes memoized by
  relation node and immutable frame environment.

No call application may enter `WorldProgram.root`, run a point worklist, clone
or substitute a callee DAG, or solve a callee loop. Doing so would recreate
`callee CFG cost x call applications`, the original multiplicative defect.
Common callees must reduce to small boundary circuits; specialization cost is
proportional to demanded boundary/output terms.

A call frame owns one atomic transaction:

```text
Project caller roots
  -> inbound Rebase/Apply
  -> ApplyRef(callee Relation, lazy environment)
  -> separately frozen outbound Project/Rebase/Apply
```

Inbound and outbound maps are not inverses. The frame owns params, captures,
globals, result selectors, mutated boundary roots, reachable heap templates,
route ownership and injective call-site allocation renaming. Ignored-result
calls still apply the world transaction. Every result/effect selector reads the
same frame-owned outcome, and each guarded outcome is applied atomically before
choice join.

`summary.Summary`, diagnostics, manifests and observations are projections from
the stabilized/narrowed relation. They are never SCC payloads and never trigger
materialization or another solve. `BoundaryArtifact` is an ephemeral exact
transport value; it is never joined, widened or stored as a cell lattice.

## 22. Semantic test salvage required before cutover

Row cardinality, Relation-lattice, shadow-evaluator and migration-differential
tests delete with those representations. Their semantic scenarios do not. The
sole-engine acceptance suite must prove, through reduced Relation Apply and its
single projection path:

1. self and mutual recursion with base cases; base-less recursion has no normal
   continuation; nested/scoped returns never cross; SCC/member/input
   permutations have canonical bytes; cancellation publishes nothing;
2. correlated multi-return calls, nonreturning callees, caller-prefix effects,
   f(x,x), two-formals-to-one-actual coalescing, param/capture/global mutation,
   inbound/outbound rebasing and lexical allocation freshness;
3. ordered noncommuting effects, shared result/allocation identity, exact lane
   authority and all-17-lane boundary round trips;
4. one stabilized world for point/edge/return observations and every projected
   diagnostic/product, including recursive owner locality and cancellation;
5. canonical Relation identity covering every reachable term, effect, ApplyRef,
   guard, mu binder and output field, independent of construction order and
   hash collisions, with no post-Freeze growth;
6. guarded diamond/loop circuits with O(CFG + typed facts) retained nodes and no
   DNF rows or cardinality cap; and
7. exact lowering for concat, signatures, adjusted multi-return, suspension and
   static/generic indexing, with unsupported proof surfaces rejected
   transactionally rather than converted to a contextual/fallback Relation.

The allocation-call regression remains red until executable Apply lands; it
must not be deleted merely to make the replacement package green.

## 23. Forest-wide call authority and relation variables

The complete static call authority already exists before interprocedural
analysis. `body.sealPreparedCallSurface` enumerates WIR calls independently and
`operationplan.SealCallSurface` freezes the exact classified point set. A
lexical target carries its stable lexical body identity. No later fact scan,
summary key, context key or generation-local cell may rediscover or replace
that identity.

Immediately after the complete `body.StaticForest` is prepared, and before any
transformer arena is opened:

1. collect every nonzero stable lexical body identity;
2. sort the full-width identities lexicographically and assign dense nonzero
   `relationVar` identifiers;
3. resolve every sealed call-surface point through that table and freeze its
   independently ordered inbound/outbound boundary lenses, result selectors
   and allocation namespace;
4. compute call/resource SCCs over relation variables and give each recursive
   component a canonical tuple-mu binder; and
5. freeze all member WorldPrograms and jointly reduce them before sealing any
   member relation arena.

An ApplyRef stores the frame only; the frame owns its target relation variable,
so target identity is never duplicated. Call application binds a lazy
environment and applies a stabilized boundary transaction. It does not copy a
callee term DAG, enter a callee WorldProgram or construct syntax after sealing.

Consequently `DirectCallCatalog`, generation `CellRef`, `RelationView`,
`RelationCell`, `PreparedEquation`, `DirectEquation`, lazy
`ensureFrozen(catalog)` selection and their relation snapshots are migration
machinery, not final architecture. They delete when the forest-wide freeze
transaction lands. The early body/operation-plan call-surface producers remain.

## 24. Tuple-mu application identity and guarded World

Concrete application tabulation is not the replacement. In particular, neither
`BindingCursor` values nor a one-edge key such as
`(target relationVar, caller relationVar, callFrameTerm)` may identify a fixed
point cell. The first recreates an unbounded context universe as abstract
arguments grow; the second merges distinct callers after one lexical edge and
silently loses correlation. Both designs are rejected and must not remain as
fallbacks or dormant alternatives.

The frozen structural identities are:

- `relationVar`, assigned from full stable lexical body identity;
- `ApplyRef`, identified by its owner relation and frozen call frame (the frame
  owns its target and complete boundary lens);
- `callSCCMu`, with canonically ordered members, internal ApplyRefs, feedback
  heads and resource edges; and
- lexical `LoopMu` binders nested inside their owning relation.

An application namespace extends only along the acyclic SCC-condensation graph.
An internal SCC edge resolves to the active `(callSCCMu, entry namespace)`
session rather than extending the route. A mu cell is exactly one member of
that session. Consequently separate external applications remain distinct,
while recursion is cycle-normalized to a finite structural equation set;
abstract values are lattice payload and never identity.

The payload is the complete guarded world, not one unguarded `state.State`. `K` is the
finite cross-product of a structural invocation route and a typed equation,
loop-head or outcome coordinate.
For a member of an active call-SCC namespace, invocation routes are exactly the
namespace root (when that member is the entry) plus the frozen internal
ApplyRefs that target that member. Two static call sites remain distinct routes;
recursive reuse of one static call site reuses one route and cannot grow `K`.

```text
GuardedState = reduced ordered multi-terminal BDD(GuardAtom, state.State)
World        = ordered (invocation route x typed coordinate) inventory -> GuardedState
```

Normal Bottom does not erase exceptional, suspended, protected or nonreturning
routes. Truthy and Falsy are exact complementary atoms. The existing cap-free
observation ROBDD kernel is promoted to the sole ordered-decision owner and
generalized to intern World terminals; observation coverage consumes the same
arena. There is no second BDD vocabulary, DNF expansion, node budget or
Top-on-exhaustion behavior. All coordinate roots share that kernel and atom
order; no terminal stores or scans a complete route vector. Atom identity
includes the structural application namespace, invocation route, feedback
lifetime scope and substituted value term, so separate applications and
successive dynamic visits do not accidentally correlate predicates. Truthy
and Falsy edges for one scoped occurrence use opposite edges of the same atom.

A finite invocation route is reused at lexical-loop iterations and recursive
call depths; its Boolean choices are not. Immediately before a feedback
contribution enters a reused coordinate, the carrier existentially quantifies
the ranks whose lifetime ends at that boundary. Existential quantification is
the exact State-lattice operation `exists a. F = F[a=false] Join F[a=true]`.
A loop boundary forgets atoms structurally inside that `LoopMu` for the active
invocation route (including nested-loop scopes); a call-SCC feedback boundary
forgets every atom owned by the target invocation route being reused. Caller
and sibling-route atoms remain live. The operation is an iterative, memoized
ROBDD traversal with no semantic budget and does not create iteration/depth
identities.

The initial sealed outcome inventory contains normal continuation, suspension,
protected exceptional continuation and nonreturning termination. Protected
normal typestate travels on the normal State; its explicit feasibility bit and
the exceptional snapshot are represented by their typed routes. These are not
invented aliases for the resource-typestate State lane: route feasibility is
part of control semantics, while the lane carries the corresponding resource
store.

All mu coordinates therefore retain the one registered `state.State` lattice;
there is no parallel Summary or outcome lattice. Total circuit authority proves
`SuspensionKnown`; feasibility of the typed routes projects `MaySuspend` and
protected `HasNormal`/`HasExceptional`, while their States project the exact
resource snapshots and effects. Results, summary facts, observations and
diagnostics are static selectors over stabilized coordinates and never become
another fixed-point payload.

Each linked frame freezes its inbound and outbound root relations, caller and
target keyspaces, result selectors, route map, existential namespace and exact
allocation plan. Its invocation route owns that allocation plan. Lexical
allocation templates are substituted by the owning route before its State can
enter a leaf Join/Widen; a target reached through two ApplyRefs therefore gets
two finite allocation routes without creating two value-keyed cells. Boundary
transport is performed independently for each guarded leaf. `BoundaryArtifact`
remains ephemeral and is never a cell value, join operand or identity.

The namespace-root route owns the same kind of allocation authority without
pretending to be a call frame. Its concrete allocation identity is the typed,
injective image of `(root-entry route, lexical AllocationTemplate)`. Apply
routes use `(ApplyRef route, lexical AllocationTemplate)`. No synthetic caller,
point, solve generation or entry digest may stand in for the root route, and no
template spelling may reach a GuardedState terminal on either route kind.

SCC ascent is scheduled over the canonical equation tuple: join every incoming
coordinate contribution and widen only the dirty GuardedState coordinates at
designated lexical-loop or call-SCC feedback heads. After equality,
joint narrowing continues to equality without a pass count. Any growth during
narrowing abandons that narrowing round and resumes ascent from the widened
tuple. Reverse dependencies are frozen from structural code; no callback may
discover a new equation during evaluation. Cancellation or error publishes no
semantic root or observation artifact.

## 25. Frozen application guard substitution

Owner-local Guard and ValueTerm identifiers are not application variables. A
callee decision diagram cannot be copied into a caller diagram with its lexical
atom numbers unchanged, and a sealed caller Arena cannot be reopened to import
the callee DAG. Either operation would lose correlation or create runtime syntax.

Each linked ApplyRef therefore owns one immutable substitution circuit, frozen
after both lexical arenas and the complete frame schema exist. It maps every
callee param/capture/global root, guard atom, result selector and effect target
through the frame's caller Value/Path expressions. Application construction
normalizes those circuits into a finite `(tuple namespace, structural
invocation route, feedback lifetime scope, substituted atom)` inventory before
tuple ascent begins. Two callee formals bound to the same caller expression
within one frame and lifetime therefore produce the same application atom
(`f(x,x)`), while two static ApplyRefs remain distinct even inside one call-SCC
namespace. Same-site recursion reuses the finite rank inventory but
existentially closes the prior depth before that rank is reused.

The substitution circuit is structural and acyclic outside tuple-mu references.
It never evaluates abstract values, opens an Arena, copies a callee term DAG per
round, or discovers equations. GuardedWorld transport uses it to re-key decision
variables and applies the same frame-owned boundary lens independently to every
State terminal of the affected coordinate. Missing substitution for any reachable guard/value/path/effect is a
transaction error; there is no callback, scalar Apply, or unrelated-atom
fallback.

A lexical `Choice` is pointwise over invocation routes. It conditions only the
route/outcome coordinate whose substitution produced the application atom;
using one owner-local guard to select an entire route-product `World` would
incorrectly correlate distinct calls and is forbidden.

## 26. Production cut and semantic burn-down (2026-07-15)

The production cut is complete. `RunBoundChunk` and `RunBoundFunction` invoke
only `runPreparedRelationProgram`: forest freeze, one tuple-mu solve, and
coordinate publication. The former query fixpoint, abstract-context discovery
prepass, summary-body cache, retained handoff, materialization replay,
`program/internal/callresult`, and their service plumbing have been physically
deleted. There is no feature flag or fallback. The fixture oracle therefore
cannot be declared green until the canonical path itself passes it.

The first full public `fixpoint/program` run after that cut compiles and exposes
the remaining semantic work in these producer families, in current priority
order:

1. signature-call result/effect producers and contextual relation composition;
2. exact adjusted/scalar/assignment source-term provenance;
3. branch-condition, contextual path-evidence, and cast-certificate production;
4. structural-freezer ownership for writes to lexical symbols;
5. direct-call result target kinds and generic/channel result transport;
6. capture/global lexical terms and heap forwarding;
7. publication mismatches: false-edge feasibility, escape effects, guarded N5
   returns, function-result uniqueness, and lexical allocation identities.

Every red is repaired at its producing authority. No family may be addressed by
reintroducing a body solve, replaying source syntax after stabilization,
skipping a fixture, imposing a semantic budget, or retaining an alternate
implementation. After each producer family, run the focused public tests; after
several families, run the full program suite and zero-skip oracle. Run cold
Kickside only once the oracle is canonical and green, then optimize the measured
remaining work toward the 30--40 second target.

### 26.1 Reconciliation checkpoint

The following production-path families are complete and independently green:

- both public entrypoints execute the sole relation transaction;
- application identity and publisher-owned semantic lineage, including
  transitive change propagation and recursive-SCC stability;
- external error-return selected-edge correlation and channel-receive callback
  presence correlation;
- generic pure binary expression terms through the canonical Lua abstract
  operation kernel;
- exact call-argument and return object materialization, nested heap transport,
  forwarding through a lexical call, and scalar object-return shape;
- unified boundary-root/sealed-environment path binding, structural capture
  addresses, and resolved dynamic-index mutation through the existing factapply
  transaction;
- owner-decoded canonical state fingerprints for every registered keyspace
  namespace, including private boundary/existential keys;
- exact captured sibling-function values at call input; and
- zero-arity invocation reachability and removal of generic-definition-derived
  application routes (high fanout now retains 7 lexical schemas and 28 exact
  concrete substitutions with zero body solves); and
- the complete intraprocedural body suite, including the measured eligibility
  census (52/56 root assignments and 5/5 returns).

Application results remain grouped by lexical definition owner. Every distinct
concrete application is retained; only a generic definition application and
its derived contexts may be omitted when the binder proves stable function
identity, no escape, and a complete direct-call set. Parent-route metadata is
lineage, not a second dynamic result tree.

The current full canonical program suite localizes the remaining work to:

1. exact boundary-environment writes for total generic-for/result projections;
2. call-input/root seeding for captures, method self, and callback definition
   frames whose structural addresses are exact but value carriers are absent;
3. frozen root-assignment sources and the remaining result-source projection;
4. object-literal predicate and the remaining pure expression vocabulary; and
5. contextual path-evidence terms.

This list supersedes work on the deleted query/materialization solver. A green
test from any retired path is not evidence. The next whole-corpus performance
measurement is permitted only after the canonical program suite and zero-skip
fixture oracle are green.
