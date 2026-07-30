# Formal Obligations Register

Status: prose-first mechanization inventory, 2026-07-18. Enumerates the
mathematical obligations the checker's soundness and the fixed-point solver's
termination rest on, each mapped to its current enforcement or gap. This is
the prerequisite for a later Lean formalization pass, not a replacement for
[invariants.md](invariants.md) (the executable rulebook this register cites
and extends) or the design documents it draws from:
[operator_plan_totality_design.md](operator_plan_totality_design.md),
[semantic_engine_v2_execution_plan.md](semantic_engine_v2_execution_plan.md)
sections 13/19-26, and
[factorized_transfer_visibility.md](factorized_transfer_visibility.md).

Every `ENFORCED BY` file/test reference below was checked against the current
working tree before being cited (`git status` shows 1,217 changed paths on
this branch — an active, uncommitted rewrite — so several references in
`invariants.md` itself no longer resolve; those cases are called out
explicitly rather than silently repeated).

Status legend: **enforced** (test/gate exists and passes today) · **partially**
(some tested cases, known uncovered cases or an unproven general form) ·
**stated-only** (a design document asserts it; no code or test attempts it
yet) · **gap** (a named, currently-open defect against a stated obligation).

## A. Lattice laws

### A1. Per-lane join/widen/order laws

**STATEMENT.** For every registered `state.State` lane (`Values`,
`PathEvidence`, `DynamicIndex`, `HeapTableIdentity`, `FrozenTables`,
`EffectDeltas`, `EscapeEvents`, `ChannelSelect`, `StoreRelations`,
`KeyMemberships`, `Typestates`, `Placement`, `LenFloors`, `NumFloors`,
`NumCeils`, `DiffRelations`, `UserLattices`) and for the full 17-axis
`product.Value` product: `Join` is idempotent, commutative, associative, and a
least upper bound; `Bottom`/`Top` are identity/absorbing for `Join`; `a ⊑ b`
holds iff `Join(a,b) = b`; `Widen` over-approximates both operands; and every
ascending chain `s_{i+1} = Widen(s_i, Join(s_i, growth))` stabilizes within a
lane-declared finite bound (4 for most lanes, 6 for `NumCeils`'s 3 thresholds).

**WHY LOAD-BEARING.** The worklist fixed-point solver assumes these laws
per lane; one lane that drops a path-only fact at join or oscillates under
widen invalidates both soundness and termination of the product solve.

**ENFORCED BY.** `TestCoreAbstractInterpretationLaws` in
[`analysis/engine/factapply/lattice_laws_test.go`](../engine/factapply/lattice_laws_test.go),
driving the generic `latticelaws.LawSuite` in
[`analysis/test/laws/lattice/laws.go`](../test/laws/lattice/laws.go) over 17
`stateLawCases` plus the full product domain. The harness checks reflexivity/
antisymmetry/transitivity of `⊑`, join idempotence/commutativity/associativity/
bottom-identity/top-absorption/least-upper-bound, order-consistent-with-join,
meet laws when `Meet != nil`, absorption, widening over-approximation, and
widening-chain termination against a per-suite bound.

**STATUS.** Enforced.

**MECHANIZATION NOTE.** `forall lane L, forall a,b,c in Sample(L)`: the finite
law set above. A Lean statement would quantify over the *actual* (often
unbounded) carrier of each lane's Go type, not a finite sample — the gap
between "this sample obeys the laws" and "the type obeys the laws" is exactly
what a mechanized proof closes.

### A2. Canonical-representation law for coordinate families

**STATEMENT.** For the seven registered dependent coordinate-family carriers
used by the guarded transformer's decision kernel, `Join` at decision-diagram
terminal identity (not merely lattice equality) satisfies
`Canon(Join(a,b)) == Canon(Join(b,a))`,
`Canon(Join(Join(a,b),c)) == Canon(Join(a,Join(b,c)))`, and
`Equal(a,b) => Canon(a) == Canon(b)`.

**WHY LOAD-BEARING.** The incremental admission fold normalizes locally at
each recomputed binary node instead of re-normalizing the whole result
([operator_plan_totality_design.md](operator_plan_totality_design.md),
"Deterministic incremental admission"). If canonical terminal identity were
order-sensitive, the `O(log k)` replacement would silently produce a
different decision-diagram spelling than a full re-fold, making the solve's
result depend on update order.

**ENFORCED BY.** `TestCoordinateFamilyCanonicalRepresentationLaws` in
[`analysis/check/fixpoint/transformer/coordinate_family_canonical_laws_test.go`](../check/fixpoint/transformer/coordinate_family_canonical_laws_test.go).
All seven families pass (landed 2026-07-18, journal `#1832`).

**STATUS.** Enforced. The consumer of this law, the fixed-shape
`ContributionID` segment tree in
[`coordinate_contribution_fold.go`](../check/fixpoint/transformer/coordinate_contribution_fold.go),
has replaced the old seed-left-fold (`admitCoordinate` no longer exists in
the tree) but is "not yet accepted": it fails closed at a `DiffRelations`
relation-shape boundary case (journal `#1833`) — see B2/G2.

**MECHANIZATION NOTE.** `forall a,b,c` in one guarded kernel's decision-node
universe (finite per program run, unbounded across runs): the three
equalities above, quantified over `Canon` and `Join` as concrete functions on
a hash-consed node table — a natural target once the node table's invariants
are themselves specified (see roadmap).

### A3. Product composition preserves lawfulness

**STATEMENT.** `product.Domain(reg)`, assembled from independently registered
axes, obeys the same law suite as any single axis: lawfulness composes
pointwise across axes for the specific 17-axis `standard.Registry()` used in
production.

**WHY LOAD-BEARING.** The checker's abstract value is always the full
product; per-axis-only evidence would not cover the value actually used by
every transfer.

**ENFORCED BY.** The `"product-value-axes"` subtest of
`TestCoreAbstractInterpretationLaws` (`lattice_laws_test.go` lines 55-65),
using `lawProductSamples(reg)` over `standard.Registry()`.

**STATUS.** Enforced for the one production registry exercised.

**MECHANIZATION NOTE.** The tested claim is an instance, not the theorem: a
Lean statement would be `forall axis families {A_i} each obeying the law
suite pointwise, product(A_i) obeys the law suite` — a genuine composition
theorem over arbitrary registries, only one of which (the standard one) is
checked today.

### A4. Cross-axis Values dependency: declared or fail-closed

**STATEMENT.** Every registered lane must declare an explicit
`laneValueDependencyPolicy` (`Independent` with an enumerator, or
`Enumerated`); constructing a lane catalog with an undeclared policy panics
at build time rather than defaulting to any runtime behavior.

**WHY LOAD-BEARING.** `State.Join`/`Widen` only need to consult
`product.Value` when a lane's semantics actually depend on it. An
undeclared dependency would let an optimization silently treat a
Values-sensitive lane as independent — the general failure mode
[factorized_transfer_visibility.md](factorized_transfer_visibility.md) names
as "missing exact dependency information defaults to `allCoordinateValues`."

**ENFORCED BY.** `TestLaneCatalogRequiresExplicitValueDependencyPolicy` and
`TestDefaultLaneCatalogDeclaresValueDependenciesForEveryLane` in
[`analysis/engine/state/lane_value_dependencies_test.go`](../engine/state/lane_value_dependencies_test.go).

**STATUS.** Enforced for the current lane catalog (the `#1603`
lane-dependency precedent). The generalization to whole operations
(sealed `OperatorPlan` read/write sets) is design-only — see D1.

**MECHANIZATION NOTE.** `forall lane in LaneCatalog: lane.valueDependencies
in {Independent(enumerator), Enumerated}` — a totality-of-a-finite-map
property, decidable by construction and already effectively "mechanized" by
the panic; a Lean model would restate the catalog as a dependent record type
whose constructor cannot be built without the field.

## B. Fixpoint and termination

### B1. Transfer monotonicity (Kildall invariant)

**STATEMENT.** At the four instrumented application points — direct
assignment, branch restriction, implication-triggered restriction, and
write invalidation — `a ⊑ b ⇒ f(a) ⊑ f(b)` for the product order.

**WHY LOAD-BEARING.** Non-monotone transfers break the worklist algorithm's
termination and correctness guarantee outright; this is the one property the
entire fixed-point framework assumes about every transfer function.

**ENFORCED BY.** `testCoreTransferMonotonicity` (the
`core-transfer-monotonicity` subtest of `TestCoreAbstractInterpretationLaws`,
`lattice_laws_test.go` lines 400-471), covering
`applyBranchRefinement`, `activatePathPresenceImplications`, and
`invalidatePathSubtreeAt`.

**STATUS.** Enforced for these four points; full Cartesian fact coverage is
explicitly deferred to focused fixture tests per the file's own comment.

**MECHANIZATION NOTE.** `forall a,b in Sample, a ⊑ b => f(a) ⊑ f(b)` for
each named `f`; a general theorem would need this for every registered
transfer, not the four sampled application points.

### B2. WTO iteration with widening at heads terminates

**STATEMENT.** Call/resource-SCC and lexical-loop feedback vertices widen
only at designated heads; after the tuple equation stabilizes, narrowing
continues to equality with any growth aborting the round and resuming ascent
from the widened tuple ([execution_plan.md](semantic_engine_v2_execution_plan.md)
section 21). The whole product solve over the SCC-condensation graph must
terminate, not merely each lane in isolation (A1).

**WHY LOAD-BEARING.** This is the termination argument for the production
solver, not a textbook aside: the "sole relation carrier" cost model
(section 21) explicitly forbids restarting an inner loop-to-stability
process, so termination has to come from the widening/narrowing discipline
alone.

**ENFORCED BY.** No dedicated termination test exists for the whole
tuple-mu/WTO ascent. Termination currently rests on three separate,
independently-tested pieces: the per-lane widening-chain bound (A1); the
narrowing-abandons-on-growth rule (code, not independently tested for
termination); and the empirical backstop `TestPathologyBudgets`
(`pathology_budget_test.go`, see G2), which catches blowups after the fact
rather than proving absence of one.

**STATUS.** Partially. The per-lane piece is enforced; the whole-product,
whole-condensation-graph termination argument is design-stated and only
indirectly, empirically backstopped.

**MECHANIZATION NOTE.** A full proof needs a well-founded measure on
`(join-then-widen-at-head)` sequences over the whole product, finite because
(a) each lane's widening chain is finite (A1) and (b) the SCC/loop
condensation graph is finite and acyclic outside its own mu-binders — i.e.
compose A1 with a finiteness argument on the condensation graph itself.

### B3. Tuple-mu SCC equations (no per-context callee unfolding)

**STATEMENT.** For call/resource SCC `C`, the sole semantic equation is a
simultaneous tuple fixed point over its function-entry and `LoopMu` cells;
internal calls refer to the vector-mu variable and never unfold a callee body
or enumerate caller contexts ([execution_plan.md](semantic_engine_v2_execution_plan.md)
sections 21, 23-24).

**WHY LOAD-BEARING.** This is what makes interprocedural cost proportional to
`(bodies + Apply edges)` instead of `(callee CFG cost × call applications)` —
the multiplicative defect the whole v2 design exists to remove.

**ENFORCED BY.** Production entrypoints `RunBoundChunk`/`RunBoundFunction`
invoke only `runPreparedRelationProgram` (section 26: "the production cut is
complete... no feature flag or fallback"). `TestResolvedReturnTransactionScalarMatchesConcreteN5`
and its four companions in
[`analysis/engine/factapply/return_resolved_transaction_test.go`](../engine/factapply/return_resolved_transaction_test.go)
differentially prove the Apply-side transaction matches concrete N5 on the
covered shapes.

**STATUS.** Partially. The production cut has landed structurally, but the
exact call-frame representation is still being corrected as of today: `#1841`
proves the body-owned planner reaches only 1 caller today and needs the
10-caller recurrence lock to prove formal instantiation is required (which it
did, see C4); `#1846`/`#1847` (today, 20:21-20:24) converge on function-space
carrier semantics whose implementation (deleting `relationCodeRuntime`'s
per-invocation dirty cells) has not landed.

**MECHANIZATION NOTE.** `R_C = mu_widen <X_function, X_loop> . F_C(...)` —
state this as a genuine mutual fixed point over a finite index set (the SCC's
members), and prove `F_C` monotone (composing B1) as the precondition for
Knaster-Tarski to apply.

### B4. Depth budgets are fail-closed backstops with correct dual polarity (invariant #1)

**STATEMENT.** A bounded positive relation (subtype, admissible cast,
runtime-claim proof) returns false when its recursion budget is exhausted; its
may-contain dual returns true at the same exhaustion point. Exhaustion must
never be read as a positive proof.

**WHY LOAD-BEARING.** Treating an incomplete traversal as success turns a
termination safeguard into an unsound proof; the dual polarity is what
prevents a diagnostic from claiming an impossible value after simply giving up
looking.

**ENFORCED BY.** `TestSubtypeDepthExhaustionFailsClosed`
([`analysis/type/subtype/subtype_test.go`](../type/subtype/subtype_test.go))
and `TestValueProofAdmissibleRuntimeCastDepthExhaustionFailsClosed`
([`analysis/domain/value/proof/proof_test.go`](../domain/value/proof/proof_test.go)).
Reinforced today: journal `#1828` fixed a fatal stack-overflow crash in
`analysis/check/body/call_argument_trust.go`'s caller-owned-parameter
recursion by adding exactly this memo-key-plus-depth-backstop shape rather
than raising or removing a bound.

**STATUS.** Enforced, and extended to a new call site today.

**MECHANIZATION NOTE.** One well-founded-recursion measure `depth`, two
lemmas from the same structural induction: `depth > budget => Positive(x) =
false` and `depth > budget => MayContain(x) = true`, proved together so they
cannot silently diverge.

### B5. No caps or deadlines as semantics

**STATEMENT.** Depth/work/node budgets exist only as termination backstops
for algorithms that are otherwise exact; they must never change a result on
an in-budget input, and every semantic cap found must be replaced by an exact
iterative algorithm, not raised or hidden
([factorized_transfer_visibility.md](factorized_transfer_visibility.md),
"Remaining semantic caps found by the full scan").

**WHY LOAD-BEARING.** A cap that silently changes results on some in-budget
inputs is a hidden unsoundness with no test able to distinguish it from a
correct exact algorithm at small scale.

**ENFORCED BY.** Nothing currently rejects the two caps the design doc itself
names as outstanding violations, both confirmed still present in the tree:
`typ.DefaultRecursionDepth = 4096`
([`analysis/type/typ`](../type/typ), consumed by
`analysis/type/subtype/guard.go`, `analysis/type/typecall/generic_call.go`,
`analysis/type/typecall/shared.go`, `analysis/type/access/field_descent.go`)
changes subtype/generic-call results on sufficiently deep acyclic type
graphs; and the evidence axis's `maxOrigins = 4` cap
([`analysis/domain/value/axis/evidence/evidence.go`](../domain/value/axis/evidence/evidence.go))
silently truncates provenance beyond four origins.

**STATUS.** Gap, explicitly named by its own design document as unfixed;
owning fix is "replace with an exact iterative node/pair graph algorithm" and
"replace with an exact interned provenance set/graph," not raising either cap.

**MECHANIZATION NOTE.** For a target algorithm `f` bounded by cap `B`:
`forall x, depth(x) <= B => f_capped(x) = f_exact(x)`. Since `f_exact` isn't
implemented, this obligation is currently unprovable and unfalsifiable by
construction — it can only be closed by first landing the exact algorithm.

## C. Functional summaries and Apply

### C1. Composition soundness: Apply over-approximates inlining

**STATEMENT.** For every call site, applying the callee's relation at the
boundary must be at least as weak (⊒) as inlining and solving the callee body
directly at that call's abstract input — and, per the v2 design, is claimed
to be the *exact* canonical transpose, not merely an over-approximation
([execution_plan.md](semantic_engine_v2_execution_plan.md) section 21: "This
is the canonical transpose... not an approximation").

**WHY LOAD-BEARING.** This is the entire reason summaries/Apply exist instead
of re-solving every callee at every call site; if Apply could look more
precise than actual inlining it would be unsound, and if never checked a
regression degrading it to a mere approximation would go unnoticed.

**ENFORCED BY.** `TestResolvedReturnTransactionScalarMatchesConcreteN5`,
`...PathMatchesConcreteN5`, `...ObjectGraphMatchesConcreteN5`,
`...ZeroSourceMatchesConcreteN5`, `...DuplicateSourceMatchesConcreteN5`
(`analysis/engine/factapply/return_resolved_transaction_test.go`) — each
proves the resolved-return (Apply-side) transaction matches the concrete N5
(inlined) transaction bit-for-bit on the covered shapes.

**STATUS.** Partially. Differential equality is proven on enumerated shapes;
there is no general proof (or property-based test) that Apply is exact — as
opposed to merely sound — across the full call-cost model of section 21.

**MECHANIZATION NOTE.** `forall call site c, callee relation R, input i:
SolveInline(body, i) ⊑ Apply(R, i)` for soundness, `=` for the stronger
"canonical transpose" claim. A Lean model needs RelationCode/Apply's
denotational semantics defined once, and inlining defined as its unfolding,
to state exactness rather than mere order.

### C2. Substitution exactness at BindIn/BindOut

**STATEMENT.** The frame's substitution circuit maps every reachable
callee-owned guard atom, param/capture/global root, and result selector
through the frame's caller Value/Path expressions; a reachable guard/value/
path/effect with no substitution is a transaction error, never a silent
default ([execution_plan.md](semantic_engine_v2_execution_plan.md) section 25).

**WHY LOAD-BEARING.** A partial substitution would let a caller diagram
evaluate an atom still bearing the callee's private numbering, silently
correlating unrelated call sites or losing a guard.

**ENFORCED BY.** `TestRelationDefinitionLifetimeRequiresExactDefinitionGuardVocabulary`,
`TestRelationDefinitionGuardVocabularyFreezesStableTargetRename`,
`TestRelationDefinitionGuardVocabularyKeepsPostEntryRegisterAtomTargetLocal`,
`TestRelationGuardRankRenameRebuildsCompleteCoordinateFactor`
(`analysis/check/fixpoint/transformer/relation_forest_definition_equations_test.go`),
exercising `emitRelationDefinitionBindIn`
(`analysis/check/fixpoint/transformer/relation_forest_definition_equations.go`).

**STATUS.** Partially. The tested vocabulary is covered; `f(x,x)`
two-formals-to-one-actual coalescing and same-site-recursion existential
closing (both named in section 25) are listed as still-required salvage in
section 22 item 2 and are not independently confirmed here.

**MECHANIZATION NOTE.** `forall ApplyRef, forall reachable atom a in callee
schema: exists! substituted expression e. Apply(schema)[a] = e`; missing case
is `⊥`/error, never identity or default — a standard substitution lemma once
the schema is given a typed calculus.

### C3. Tabulation-key completeness (memo key = revision × namespace × guarded input)

**STATEMENT.** A demand-evaluated relation node's memo key must be exact over
`(dependency revision, unit namespace, immutable frame/guarded-input
environment)`; reusing a cached node under a changed revision, a different
namespace, or a different frame is unsound reuse
([execution_plan.md](semantic_engine_v2_execution_plan.md) section 21: "once
per dependency revision... memoized by relation node and immutable frame
environment").

**WHY LOAD-BEARING.** This is the memoization discipline that lets the sole
relation carrier avoid re-solving; an incomplete key is exactly the class of
bug invariant #9 (digest completeness) already guards for `summary.Summary` —
here the same discipline is newly required of relation-node memoization.

**ENFORCED BY.** Nothing yet. The current `summary.SummaryKey{Ref,
EntryKey{Values,Facts,References}}`
([`analysis/check/fixpoint/summary/summary_key.go`](../check/fixpoint/summary/summary_key.go))
digests call-entry facts but has no explicit revision or namespace field, and
no relation-node memo table with this exact key shape exists in the tree.

**STATUS.** Stated-only. This is the "new invariant the design introduces";
no code has attempted it, and it is not yet even blocked on a specific PR —
it is the natural next hardening once C4 lands (a per-invocation dirty cell,
today's `relationCodeRuntime`, has no key of this shape at all).

**MECHANIZATION NOTE.** `memoKey: (revision, namespace, frame) ⇀ RelationNode`
must be a partial injective function, and any write that changes `revision`
must invalidate every entry naming it — a cache-coherence property, provable
as an invariant of the cache's own transition relation once it exists.

### C4. Function-space carrier canonicality (`#1846`/`#1847`)

**STATEMENT.** The interprocedural body carrier is one immutable relation per
lexical body/SCC over `V -> V` (params/captures/globals/result/heap-template
roots), composed by Apply through substitution, alpha-renaming and join —
never a shared mutable per-invocation `State` cell. Apply must not enter
`WorldProgram.root`, clone a callee DAG, or restart an inner solve.

**WHY LOAD-BEARING.** Sharing a mutable row across contexts is the exact
false-recurrence bug the 2026-07-18 decision closed with a 10-caller
recurrence lock; it is the Sharir-Pnueli functional-approach precondition the
whole cost model in sections 19-24 depends on for soundness and cost.

**ENFORCED BY.** Journal `#1846` (decision) and `#1847` (refinement:
`relationCode` is already the correct immutable schema — `ValueTerm`/`Guard`/
`EffectTerm`/`boundaryStep`/`LoopMu`/`Apply` contain no `State` or caller key;
the defect is `relationCodeRuntime`'s per-invocation mutable dirty-cell
materialization, which must be deleted, not mirrored into a new IR). `#1841`
(10-caller recurrence milestone) is the evidence base for the decision.

**STATUS.** Gap. `relationCodeRuntime` is still present
([`relation_code_executor.go`](../check/fixpoint/transformer/relation_code_executor.go),
[`relation_forest_coordinate_scheduler.go`](../check/fixpoint/transformer/relation_forest_coordinate_scheduler.go),
and five more test files) — the decision converged today; the deletion has
not landed.

**MECHANIZATION NOTE.** One relation `R_f : Frame -> World` per lexical `f`;
`Apply(R_f, frame1)` and `Apply(R_f, frame2)` share no mutable cell, i.e.
`R_f` is a pure function of its frame — state this over the SCC-condensed
call graph, composing with B3.

## D. Access and width (operator-plan design)

### D1. Certificate totality at admission (the sealed `OperatorPlan`)

**STATEMENT.** Every semantic operation is admitted to the equation graph
only as one immutable `OperatorPlan` with a closed input-role sum, exact
component read/write sets, dependency hyperedges, and a declared algebra
kind; there is no open-ended `All` constructor, and adding a lane reopens
catalog admission
([operator_plan_totality_design.md](operator_plan_totality_design.md), "The
sealed OperatorPlan").

**WHY LOAD-BEARING.** An operation that can silently read/write an
undeclared component defeats every downstream exactness law (noninterference,
block factorization, boundary transport), because the executor can no longer
prove disjointness or closure.

**ENFORCED BY.** No `OperatorPlan` type exists in the tree yet. The weaker,
currently-enforced precedent this design explicitly "lifts to operations" is
the lane catalog (`#1603`): `TestDefaultLaneCatalogDeclaresValueDependenciesForEveryLane`
/ `TestLaneCatalogRequiresExplicitValueDependencyPolicy` (A4).

**STATUS.** Stated-only. Converged design (two-round adversarial review,
2026-07-18); zero-allocation `OperatorPlan`/binder/`DemandCursor` machinery is
unimplemented.

**MECHANIZATION NOTE.** `forall registered operation op: op.reads, op.writes`
are finite exact sets fixed at seal time, and any runtime accessor not named
in them is unrepresentable — a type-level (not merely runtime-checked)
property, closer to a typed-binder-calculus proof than a simple quantified
statement.

### D2. Selector cone law (visit only the seed-induced cone)

**STATEMENT.** A dynamic-coordinate selector may visit and return only the
semantic cone induced by its nonempty seeds/query through its registered
capability; it may never scan the family inventory to discover that cone.

**WHY LOAD-BEARING.** This is the noninterference precondition: once a
selector can scan the whole inventory, "declared reads" stops being a sound
proxy for "what this operation can see," and the affordable-noninterference
enforcement layers (D3) become unenforceable.

**ENFORCED BY.** Nothing yet enforces the closed algebra
(`Exact | Closure | Query | Union | Push | Pull`). The design itself names
two live violations, both confirmed still present:
`coordinateSelectionContract.Select(domain, []CoordinateSlot)`
([`coordinate_selection_contract.go`](../check/fixpoint/transformer/coordinate_selection_contract.go)),
described as "All with a callback around it"; and `projectDynamicReadFacts`
([`analysis/engine/state/dynamic_read_factor.go:655`](../engine/state/dynamic_read_factor.go)),
named by the design as "incident 3/4 still alive."

**STATUS.** Gap — named and unrepaired in the design document itself.

**MECHANIZATION NOTE.** `forall selector s with seed set S: visited(s) ⊆
cone(S, capability)`, where `cone` is defined structurally per family
(path-descendant closure, alias closure, ...) and `visited` is the
executor's trace — an operational-semantics-style proof, not a pure data law.

### D3. Noninterference (the catalog perturbation law)

**STATEMENT.** For any two stores equal on a projection's declared read set
but different outside it, the projection/closure/selector-capability/
boundary-morphism under test produces identical declared outputs and
identical terminal-vector counts
([operator_plan_totality_design.md](operator_plan_totality_design.md),
"Catalog lawsuite"). This is the same law
[factorized_transfer_visibility.md](factorized_transfer_visibility.md) states
at block granularity ("Exact factorization law":
`project_Ii(x) = project_Ii(y) => project_Bi(F(x)) = project_Bi(F(y))`).

**WHY LOAD-BEARING.** This is the formal noninterference property that
licenses "zero-allocation, undeclared-reads-unrepresentable" as sound rather
than merely fast.

**ENFORCED BY.** No generic per-registration lawsuite exists (the design
specifies one running "once per registered projection, closure, selector
capability, and boundary morphism"); only the per-lane order/join `LawSuite`
(A1) exists today, which does not test read-projection noninterference.

**STATUS.** Stated-only — specified at two altitudes (block-level and
selector-level) in two design documents, unimplemented as an executable gate
at either altitude.

**MECHANIZATION NOTE.** `forall projection P with declared reads I, forall
x,y: project_I(x) = project_I(y) => P(x) = P(y)` on `P`'s declared outputs —
a clean, already-quantified law; a strong Lean-first candidate once one
concrete `P` (e.g. one boundary morphism) is chosen as the pilot.

## E. Identity and canonicality

### E1. Structural canonical identity — no allocation-order leakage (invariant #8)

**STATEMENT.** Semantic ordering, map keys, and digests use canonical
structural fields and stable hashes; debug strings and allocation/pointer
identity (`Graph.ID()`, addresses) are display-only and must never select a
key, an order, or a digest contribution.

**WHY LOAD-BEARING.** Pointer order and diagnostic text vary run to run;
leaking them into semantics makes summaries/fixtures nondeterministic and
corrupts cache identity.

**ENFORCED BY.** `TestGraphInstanceIdentityDoesNotLeakIntoSemanticArtifacts`
([`analysis/architecture/graph_identity_projection_test.go`](graph_identity_projection_test.go)),
which checks a maintained, deliberately small allowlist
(`reviewedRunLocalGraphIDCalls`) of the only permitted run-local uses of
`Graph.ID()`; `keyspace.Less` orders structural spelling; `wir/print.go`
excludes address identity.

**STATUS.** Enforced.

**MECHANIZATION NOTE.** `forall instances a,b of the same canonical value
with different Graph.ID()/pointer identity: Key(a)=Key(b), Digest(a)=Digest(b)`.
Provable largely as an AST-closed-allowlist fact (the current test's shape)
rather than a semantic theorem.

### E2. Process-global interning validity of `product.Equal` across contexts

**STATEMENT.** `product`'s interning cache is a bounded, sharded,
FIFO-evicting fast path only; `product.Equal` is structural and holds
regardless of whether a value is currently interned, evicted, or was
constructed by a different solver/worker.

**WHY LOAD-BEARING.** A long-lived checker registry must not let a fast-path
cache become process-lifetime semantic ownership; eviction may lose a
pointer-identity shortcut but must never be able to change a structural
equality answer.

**ENFORCED BY.** `TestInternerEvictionKeepsDurableValuesValid`,
`TestInternerShardsOneRegistryByCandidateHash`
([`analysis/domain/value/product/intern_retention_test.go`](../domain/value/product/intern_retention_test.go)).

**STATUS.** Enforced.

**MECHANIZATION NOTE.** `forall v, evicting v does not change Equal(v,w) for
any w`; `forall shardings, sharding does not change any Equal/Hash result`
(only which mutex is taken).

### E3. Solve-local interned IDs never cross a boundary (invariant #13)

**STATEMENT.** `SegmentsID`, root intern IDs, and any whole-key scalar ID are
valid only in their producing `KeySpace`; they never appear in summaries,
manifests, diagnostics, fixture oracles, or persistent caches. Cross-keyspace
use rekeys through canonical spelling.

**WHY LOAD-BEARING.** Dense IDs depend on discovery order; serializing one
turns one solve's implementation detail into another solve's false path
identity.

**ENFORCED BY.** [`keyspace.go`](../domain/path/keyspace/keyspace.go)'s
documented scope restriction; `RekeyValueLanes`
([`analysis/engine/state/pathevidence/rekey.go`](../engine/state/pathevidence/rekey.go))
converts keys through source formatting before interning in the destination
keyspace; `analysis/engine/state/rekey_keyspace_test.go`,
`rekey_adversarial_test.go`, `rekey_exact_lane_test.go` exercise the boundary.

**STATUS.** Enforced.

**MECHANIZATION NOTE.** `forall K1 != K2, forall key k valid in K1: k is never
read as K2-scoped`; `Rekey(k, K1, K2)` round-trips through canonical string
spelling, never through `k`'s numeric value.

### E4. Witness equality stricter than structural equality (recursive-identity plan) — still-open gap

**STATEMENT.** The typewitness axis's equality contract is
`SameReachableRecursiveIdentitySet(a,b) AND typ.TypeEquals(a,b)` — strictly
stronger than `TypeEquals` alone — so two independently allocated anonymous
mu-types unfolding to the same graph remain distinguishable witnesses, while
the same authoritative declaration loaded twice compares equal. Canonical
disk encoding must fail closed whenever a reachable recursive node lacks a
portable stable identity — never erase the witness, substitute Top, or
silently skip the axis
([recursive_type_identity_plan.md](recursive_type_identity_plan.md)).

**WHY LOAD-BEARING.** Collapsing recursive identity to a structural digest
would weaken current lattice equality and could unsoundly collapse proofs the
domain currently keeps separate; conversely, leaking a process-global
allocation counter into a durable artifact key would make caches
nonportable and nondeterministic across processes.

**ENFORCED BY.** Today only the local/process-global numeric ID exists
(`typ.Recursive.ID`); the plan's `recursiveidentity` package and
manifest-wide family-ordinal scheme (plan section 6.1) do not exist in the
tree — confirmed absent. `TestCanonicalArtifactRejectsRecursiveWitnessAndEncodeOnlyAxis`
([`analysis/domain/value/product/canonical_artifact_test.go`](../domain/value/product/canonical_artifact_test.go))
covers today's narrower encode-only/rejection behavior, not the plan's full
dual-identity contract.

**STATUS.** Gap. This is an 8-stage reviewed implementation plan (plan
section 10) with no stage landed yet.

**MECHANIZATION NOTE.** Define witness equality as
`RecursiveIdentitySet(a) = RecursiveIdentitySet(b) ∧ TypeEquals(a,b)` over an
inductive/coinductive type graph with a separate stable-identity carrier;
prove (1) this relation is strictly finer than `TypeEquals` alone, (2)
canonical encoding is injective on witness-equal classes, (3) encoding is
total on authoritative nodes and explicitly partial (typed failure) otherwise.
Strong Lean-first candidate — see roadmap.

## F. Fail-closed semantics

### F1. Exactness rejection rather than degradation (invariant #1, general form)

**STATEMENT.** See B4: a bounded positive relation is false at budget
exhaustion; its may-dual is true at the same point. Stated here as the
general fail-closed principle rather than the specific depth-budget
instance.

**WHY LOAD-BEARING.** An incomplete traversal reported as a positive proof
turns a termination safeguard into an unsound proof.

**ENFORCED BY.** Same as B4: `TestSubtypeDepthExhaustionFailsClosed`,
`TestValueProofAdmissibleRuntimeCastDepthExhaustionFailsClosed`, and today's
`#1828` fix.

**STATUS.** Enforced.

**MECHANIZATION NOTE.** See B4.

### F2. Claims are checkcasts, never trusted proof (invariant #5)

**STATEMENT.** A source-level claim/cast is a runtime validation boundary;
its checked result may carry `RuntimeClaim`, but the claim may not justify
deleting itself — redundancy requires an independent pre-claim subtype proof.

**WHY LOAD-BEARING.** Trusting a declaration/cast would launder `any` or an
incompatible record into a concrete contract and make guard elimination
unsound.

**ENFORCED BY.** `TestValueProofAdmissibleRejectsAnyClaimWithoutRuntimeProof`
and companions
([`analysis/domain/value/proof/proof_test.go`](../domain/value/proof/proof_test.go));
`advice_claim.go` examines the operand before the claim.

**STATUS.** Enforced.

**MECHANIZATION NOTE.** `forall claim c on value v: c does not itself justify
redundancy(c); redundancy requires an independent proof P such that P holds
before c is evaluated` — a straightforward non-circularity property.

### F3. Unknown aliasing/mutation always degrades, never optimizes (invariant #6)

**STATEMENT.** Missing alias, invalidation, or writer information invalidates
the dependent proof/shape/license or leaves it unknown; it never upgrades a
fact to exact, stable, or non-mutating.

**WHY LOAD-BEARING.** Heap mutation and callback effects are precisely where
a false negative becomes a stale read, invalid guard reuse, or bad codegen.

**ENFORCED BY.** `CallMayInvalidateTrackedPath`, `CallMayInvalidateGuardFact`
([`analysis/check/body/call_invalidation.go`](../check/body/call_invalidation.go));
`TestPrefixStableRejectsUnknownWriter`
([`analysis/check/checktest/stable_shape_test.go`](../check/checktest/stable_shape_test.go)).
Reinforced today: journal `#1823` closed a soundness hole where a write
through an alias failed to invalidate a prior field-presence/discriminant
guard.

**STATUS.** Enforced.

**MECHANIZATION NOTE.** `forall path p, forall call c with unresolved
alias/writer info for p: fact(p) after c is Unknown or invalidated, never
strictly more precise than fact(p) before c`.

### F4. No second authority for any fact (parallel-authority rule)

**STATEMENT.** Exactly one production authority computes each semantic fact.
A second, independently recomputed path for the same fact — even if it looks
correct in isolation — is forbidden; the required fix is always to
consolidate onto the existing authority, never to patch the duplicate.

**WHY LOAD-BEARING.** Divergent recomputation is exactly how a fixed hole
reopens invisibly: an authority-side fix can be correct while an independent
recomputation elsewhere silently ignores it.

**ENFORCED BY (as an audited engineering discipline, with two live named
instances rather than one automatic gate).**
`#1844` — `Summary.NormalReturnParamConditions` is the correct authority for
`CallOutcome.ParamConditions`; `projectLexicalParamOutcomeFacts`
(`analysis/check/fixpoint/transformer/relation_coordinate_view.go:522`)
independently recomputes it via `valueref.CanBeTruthy/CanBeFalsy` on raw exit
values, ignoring the dominance-correct summary fact — confirmed still present
in the tree; `TestConditionalTerminationNarrowsCheckedBooleanExpressionOnNormalReturn`
is a live red pin.
`#1845` — a diagnostic-time authority
(`ChannelSelectNarrowedValueTypeBeforeBoundary`) recomputing channel-select
case-elimination was built, audited, and reverted under the no-compensations
rule; the real fix — wiring `factflow.BranchPathEvidenceNotEqual` into
`applyChannelSelectCaseInequality` inside `applyBranchPathEvidence`
([`analysis/engine/factapply/path_fact_apply.go`](../engine/factapply/path_fact_apply.go),
which today only branches on `BranchPathEvidenceEqual`) — is confirmed still
unwired; `TestCheckChannelSelectGuardedMethodCallNarrowsUnion`
([`analysis/check/checktest/channel_select_guarded_method_narrowing_test.go`](../check/checktest/channel_select_guarded_method_narrowing_test.go))
is a live red pin.

**STATUS.** Partially. The rule is enforced as an audit discipline (the
compensating implementation for `#1845` was built and then reverted rather
than kept); the two concrete fact-duplications it names are still open,
each pinned by a named red test.

**MECHANIZATION NOTE.** `forall semantic fact f: exists a unique producer
P_f; forall other function Q whose output is observationally in f's domain:
Q calls P_f rather than recomputing it` — a whole-program single-writer
property (a def-use/points-to-style static analysis of the analyzer itself).
Likely out of scope for a near-term Lean pass; noted but not in the roadmap.

## G. External gates

### G1. Exact census and architecture-boundary gates

**STATEMENT.** Package-import direction (lower layers never import upper
layers), adapter projection-only boundaries, and the semantic surface census
(3,644-row inventory of every `check/body` symbol and its final owner) are
checked mechanically.

**ENFORCED BY.** `TestLowerLayerImportBoundaries`,
`TestLowLevelLeafImportBoundaries`, `TestWIRImportBoundaries`,
`TestLSPAdapterImportBoundaries`, `TestRequiredSemanticSurfacesExist`,
`TestJudgmentImportBoundaries`, `TestCanonicalContractImportBoundaries`,
`TestPublicReadmodelImportBoundaries`, and further boundary tests in
[`analysis/architecture/import_boundary_test.go`](import_boundary_test.go);
the running inventory at
[`analysis/architecture/semantic_surface_census.csv`](semantic_surface_census.csv)
(3,644 rows).

**STATUS.** Enforced.

### G2. Pathology budget gate (outer regression net for width/admission defects)

**STATEMENT.** For named pathological fixtures (`many-implications`,
`recursive-cyclic-wide`, `type-engine-edge-matrix`), the sum of
engine-reported `decision_nodes`/`components`/block-lift/selected-roots
counters across every `RelationProgram.Solve` call must stay under a pinned
budget; every future closed pathology joins this gate set rather than being
verified once and left ungated
([operator_plan_totality_design.md](operator_plan_totality_design.md),
"Performance gates pin counters").

**WHY LOAD-BEARING.** Two of these three fixtures have regressed and been
re-fixed more than once (`#1663`, `#1730`→`#1732`→`#1733`, `#1736`, `#1789`)
with nothing gating the regression in between; cost admission is fail-open
even though semantics is fail-closed, so a fixed `O(everything)` blowup can
silently reopen.

**ENFORCED BY.** `TestPathologyBudgets` (`pathology_budget_test.go`, repo
root), opt-in via `GOLUA_PATHOLOGY_GATE=1`, consuming
`GOLUA_ENGINE_PROFILE=json` counters
([`analysis/check/fixpoint/transformer/engine_profile.go`](../check/fixpoint/transformer/engine_profile.go)).

**STATUS.** Enforced (gate is live, journal `#1814`). The two structural
defects it exists to catch are separately tracked: the admission refold fix
has landed (A2/`coordinate_contribution_fold.go`; `admitCoordinate` no
longer exists) but is "not yet accepted" pending a `DiffRelations` boundary
fix (`#1833`); the full-product boundary-transport fix (design step 3,
`applyGuardedBoundaryOutput`) has not landed.

### G3. Schema/manifest evolution gates (invariants #14, #15)

**STATEMENT.** Manifest wire lanes are append-only/additive with
conservative legacy decode; every registered lane has a codec-oracle case,
round-trips, and produces canonical permutation-invariant bytes;
compatibility façades stay pinned and schema surface changes require a
version bump, structural hash, and `SCHEMA_VERSIONS.md` entry.

**ENFORCED BY.** `TestOperationalEffectsDescriptorCodecRoundTrips`,
`TestOperationalEffectsCodecOracleCoversEveryRegisteredLane`
([`analysis/module/manifest/operational_effects_codec_oracle_test.go`](../module/manifest/operational_effects_codec_oracle_test.go));
`TestCheckerUsesGlobalTypesAsTypedEntryValues` and companions
([`compiler/check/global_types_test.go`](../../compiler/check/global_types_test.go));
[`SCHEMA_VERSIONS.md`](SCHEMA_VERSIONS.md) as the registration ledger.

**STATUS.** Enforced.

## Known regressions against `invariants.md` itself

Two of `invariants.md`'s own cited enforcement points do not currently exist
in the working tree (this branch has 1,217 changed paths — an active,
uncommitted rewrite):

- **Invariant #2** ("a returned fresh allocation graph is never stack") cites
  `analysis/engine/factapply/objects_test.go` and
  `TestReturnedFreshObjectGraphNeverContainsStack`. The file is deleted in
  the working tree; no test of that name exists anywhere in the repository.
  The underlying production logic (`placement.Return` in
  [`analysis/domain/placement/vocabulary.go`](../domain/placement/vocabulary.go),
  `applyReturn` in
  [`analysis/engine/factapply/return_resolved_transaction.go`](../engine/factapply/return_resolved_transaction.go))
  still exists and is indirectly exercised by
  `TestResolvedReturnTransactionObjectGraphMatchesConcreteN5`, but the
  adversarial nested-graph regression test itself is gone.
- **Invariant #11** ("capture policy has explicit precision tiers") cites
  `analysis/check/fixpoint/program/capture_seeding_test.go` and
  `TestCapturePolicyLawMatrix`. Both the test file and its production
  counterpart `capture_seeding.go` are absent from the tree. The closest
  surviving logic,
  [`analysis/check/body/closure_capture.go`](../check/body/closure_capture.go)'s
  `ClosureCapturePolicy`, exposes only two tiers (`Full`, `WriteInvariant`),
  not the three the invariant describes (full / write-invariant /
  escaped-invariant), and `closure_capture_test.go` currently contains no
  `Test` function, only a shared helper.

Both are flagged here rather than cited as enforced; `invariants.md` should
be re-verified against this branch once the in-flight rewrite settles.

## Mechanization roadmap

Lean-first candidates, ranked by how self-contained and finite their carrier
is:

1. **B4/F1 — depth-exhaustion dual polarity.** One well-founded recursion
   measure, two lemmas from the same structural induction
   (`Positive` false / `MayContain` true at exhaustion). Smallest possible
   first proof; directly backed by an existing test today.
2. **A1 — per-lane lattice laws, one finite lane as pilot.** Pick the
   smallest lane (`Placement`, a 4-element lattice) and prove the full law
   set as a real theorem over its Go-mirrored inductive type, then use the
   proof shape as a template for the remaining 16 lanes plus the product.
3. **D3 — noninterference / exact factorization law.** Already stated as a
   clean quantified law at two altitudes
   (`project_I(x)=project_I(y) => P(x)=P(y)`); pick one concrete boundary
   morphism as the pilot `P` and prove it before any executable lawsuite
   exists — a genuine spec-first proof preceding implementation.
4. **C2 — substitution exactness at BindIn/BindOut.** A standard
   capture-avoiding substitution lemma once `relationCode`'s guard/value
   vocabulary is given a typed calculus; well-trodden PL-theory territory
   with an existing test suite to validate the model against.
5. **E4 — witness equality (recursive-identity plan).** Define witness
   equality as identity-set equality conjoined with structural equality over
   an inductive/coinductive type-graph model; prove it strictly finer than
   structural equality alone and that canonical encoding is injective on
   witness-equal classes. Best done *before* the implementation (plan section
   10) lands, matching the register's own prose-first-then-mechanize order.
