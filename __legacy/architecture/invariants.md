# Checker Invariants and Cadence

## Cadence contract

Before landing a change, run `bash scripts/wall.sh` from the repository root.
After each major analysis or refactor wave, run the sol-audit/review cycle as
well; the wall is the repeatable verification floor, not a substitute for that
review. `PROMPTMAP` is optional: the wall prints a visible `SKIPPED` row when
it is unavailable.

This is a rulebook, not a second design document. The linked designs own the
mechanics and history; this file records the load-bearing rules, why they must
remain true, and the current executable enforcement point. A listed design
lock without an implementation is called out explicitly rather than presented
as a tested fact.

## Enforcement map

| # | Rule | Primary enforcement |
| --- | --- | --- |
| 1 | Positive proofs fail closed at depth limits | [`analysis/type/subtype/subtype_test.go`](../type/subtype/subtype_test.go), [`analysis/check/readmodel/api.go`](../check/readmodel/api.go) |
| 2 | Returned fresh graphs leave the stack | [`analysis/engine/factapply/return_escape_deep_graph_test.go`](../engine/factapply/return_escape_deep_graph_test.go), [`analysis/domain/placement/policy.go`](../domain/placement/policy.go) |
| 3 | Optimization opcode policy is exhaustive and closed | [`analysis/check/body/decomposable_allocation_opcode_exhaustiveness_test.go`](../check/body/decomposable_allocation_opcode_exhaustiveness_test.go) |
| 4 | Suspension and every other license need a witness | [`analysis/check/body/suspension.go`](../check/body/suspension.go), [`analysis/check/checktest/suspension_lifetime_test.go`](../check/checktest/suspension_lifetime_test.go), [`analysis/domain/placement/license_test.go`](../domain/placement/license_test.go) |
| 5 | Claims are runtime checkcasts, not trusted proof | [`analysis/domain/value/proof/proof_test.go`](../domain/value/proof/proof_test.go), [`analysis/check/body/advice_claim.go`](../check/body/advice_claim.go) |
| 6 | Unknown aliasing or mutation loses precision | [`analysis/check/body/call_invalidation.go`](../check/body/call_invalidation.go), [`analysis/check/checktest/stable_shape_test.go`](../check/checktest/stable_shape_test.go) |
| 7 | Every state lane obeys join/widen laws | [`analysis/engine/factapply/lattice_laws_test.go`](../engine/factapply/lattice_laws_test.go) |
| 8 | Ordering, keys, and digests use canonical identity | [`analysis/domain/path/keyspace/order.go`](../domain/path/keyspace/order.go), [`analysis/ir/wir/print.go`](../ir/wir/print.go) |
| 9 | Digest equality coverage is exact | [`analysis/check/fixpoint/summary/digest.go`](../check/fixpoint/summary/digest.go), [`analysis/check/fixpoint/summary/digest_test.go`](../check/fixpoint/summary/digest_test.go) |
| 10 | Adapters are projections, never checker owners | [`service_surfaces_design.md`](service_surfaces_design.md), [`analysis/architecture/import_boundary_test.go`](import_boundary_test.go) |
| 11 | Capture precision is tiered and centrally selected | [`analysis/check/body/closure_capture_policy_law_test.go`](../check/body/closure_capture_policy_law_test.go), [`analysis/check/body/closure_capture.go`](../check/body/closure_capture.go) |
| 12 | Shape IDs are artifacts, never live bindings | [`analysis/ir/wir/design.md`](../ir/wir/design.md) (deferred design lock) |
| 13 | Solve-local intern IDs never cross a boundary | [`analysis/domain/path/keyspace/keyspace.go`](../domain/path/keyspace/keyspace.go), [`analysis/engine/state/pathevidence/rekey.go`](../engine/state/pathevidence/rekey.go) |
| 14 | Manifest growth is additive and codec-complete | [`analysis/module/manifest/operational_effects_codec_oracle_test.go`](../module/manifest/operational_effects_codec_oracle_test.go) |
| 15 | Seams and schemas remain compatibility-pinned | [`compiler/check/check.go`](../../compiler/check/check.go), [`SCHEMA_VERSIONS.md`](SCHEMA_VERSIONS.md) |

## Rules

### 1. Positive proof is false at depth exhaustion; may-contain remains true

**Rule.** A bounded positive relation (subtype, equality, admissible cast, or
definite-non-number) must return false when its recursion budget is exhausted.
A may-contain query is the dual: exhaustion means it may still contain the
candidate and therefore returns true.

**Why load-bearing.** Treating an incomplete traversal as success turns a
termination safeguard into an unsound proof. The may-query polarity prevents a
diagnostic from claiming an impossible value after it simply stopped looking.

**Where enforced.** `TestSubtypeDepthExhaustionFailsClosed`, the type-call and
runtime-cast depth tests, and `numericForMayContainNumber` implement the two
polarities. See the existing type-proof tests rather than duplicating their
matrices.

### 2. A returned fresh allocation graph is never stack

**Rule.** Returning a fresh allocation applies the return escape transition to
the complete reachable fresh graph; the result is owned heap, never stack.

**Why load-bearing.** Stack placement after return is a use-after-frame bug in
the VM/codegen contract, including when the returned table contains nested
fresh tables.

**Where enforced.** `TestReturnEscapeAppliesOwnedHeapToDeeplyNestedFreshGraph`
drives a three-level-deep fresh literal graph through `PlanReturnTransaction`
and `ReturnAuthority.Apply` and asserts every reachable identity reads
`OwnedHeap`, never `Stack`. `EscapeTransitionReturn` is owned-heap policy;
`ApplyReturnFactorTransaction` walks the exact reachable-identity graph across
every registered coordinate family and publishes it through
`ProductDomain.PublishCoordinateReturnIdentity`, which targets `OwnedHeap`.

### 3. Optimization opcode policy is exhaustive and fails closed

**Rule.** Every WIR opcode must have an explicit decomposable-allocation policy.
An unclassified or unknown opcode disqualifies a tracked allocation rather than
silently preserving its optimization license.

**Why load-bearing.** New operations otherwise inherit an accidental
optimization permission before their identity, escape, and mutation behavior
has been audited.

**Where enforced.** `TestDecomposableUseTrackerClassifiesEveryWIROpcode` parses
both the opcode definition and classifier switch; the classifier's default
disqualifies a touched tracked value.

### 4. A license exists only with its witness; uncertified suspension is may-suspend

**Rule.** A positive optimization license requires its complete witness set.
In particular, a call with no `SuspensionKnown` certification is may-suspend;
frame-local/decomposable projection needs the solved placement, use proof, and
dies-before-suspension witness, not a best-effort approximation.

**Why load-bearing.** A missing fact is not evidence that the hazard is absent.
This prevents allocation reuse across a suspension or other unmodelled effect.

**Where enforced.** `PointMaySuspend` returns true for an absent/uncertified
outcome; `suspension_lifetime_test.go` covers certification and rejection;
`license_test.go` covers the canonical projection laws. The additive
`SuspensionKnown` wire lane is documented in [SCHEMA_VERSIONS.md](SCHEMA_VERSIONS.md).

### 5. Claims are checkcasts: never trusted and never self-eliminating

**Rule.** A source-level claim is a runtime validation boundary, not an
unearned proof. Its checked result can carry `RuntimeClaim`, but the claim may
not justify deleting itself; redundancy requires an independent pre-claim
subtype proof.

**Why load-bearing.** Trusting a declaration/cast would launder `any` or an
incompatible record into a concrete contract and make guard elimination
unsound.

**Where enforced.** `TestValueProofAdmissibleRejectsAnyClaimWithoutRuntimeProof`
and related proof tests reject unvalidated claims. `advice_claim.go` examines
the operand *before* the claim and only reports a redundant claim when that
independent proof already exists. The WIR design is linked below after its
claim wording was aligned with this rule.

### 6. Unknown aliasing and state always degrade, never optimize

**Rule.** Missing alias, invalidation, or writer information invalidates the
dependent proof/shape/license or leaves the result unknown. It never upgrades a
fact to exact, stable, or non-mutating.

**Why load-bearing.** Heap mutation and callback effects are precisely where a
false negative turns into stale reads, invalid guard reuse, or bad codegen.

**Where enforced.** `CallMayInvalidateTrackedPath` and
`CallMayInvalidateGuardFact` conservatively invalidate open calls;
`TestPrefixStableRejectsUnknownWriter` and the staged-shape probes cover the
observable behavior. See [staged_shapes_design.md](staged_shapes_design.md) for
the complete tier-kill matrix.

### 7. Join and widen laws hold per lane

**Rule.** Each registered state lane obeys its lattice laws: normalized equality
and order agree, join is a least upper bound, widen is monotone and converges
within the stated finite/threshold bound, and transfer maps are monotone.

**Why load-bearing.** The fixed-point solver assumes these laws. A single lane
that keeps a path-only fact at join, or oscillates under widen, invalidates both
soundness and termination of the product solve.

**Where enforced.** `TestCoreAbstractInterpretationLaws` in
[`lattice_laws_test.go`](../engine/factapply/lattice_laws_test.go) runs the
standard law suite lane-by-lane (including path evidence, placement, escape,
typestate, and thresholded numeric ceilings) and then checks core transfer
monotonicity. The generic harness lives in
[`analysis/test/laws/lattice`](../test/laws/lattice).

### 8. Canonical identity only: no debug formatting or pointer identity in semantic order, keys, or digests

**Rule.** Semantic ordering and identity use structural canonical fields and
stable hashes. Debug strings and allocation/pointer identity are display-only
and may not select map keys, ordering, or a digest contribution.

**Why load-bearing.** Pointer order and diagnostic text vary between runs; if
they leak into semantics, summaries and fixtures become nondeterministic and
cache identity is corrupted.

**Where enforced.** `keyspace.Less` orders structural spelling rather than an
intern ID; WIR printing explicitly excludes address identity; conditional
partition candidates use CFG point, canonical paths, shape, and stable product
hashes. See [scalar_key_design.md](scalar_key_design.md) for the boundary rules.

### 9. A digest covers exactly the content equality compares

**Rule.** Normalize before digesting; exclude metadata equality excludes; include
every semantic axis equality includes. Unequal normalized summaries must not
share a digest.

**Why load-bearing.** A digest is an admission/cache identity. Omitting an
equality-relevant fact reuses a stale summary; including incidental metadata
causes unnecessary invalidation.

**Where enforced.** `NormalizedPayloadDigest` explicitly removes
`HeapKeySpace` because equality does, uses `product.Hash` for all product axes,
and `TestNormalizedPayloadDigestSeparatesUnequalSemanticContent` tests the
separation law.

### 10. Adapters contain zero checker logic

**Rule.** CLI, LSP, SDK, readmodel, rendering, and transport adapters project
completed core results. Checker decisions, digest/invalidation rules, filters,
and reusable query shapes belong in core first.

**Why load-bearing.** Re-implementing a decision in an adapter creates
profile-dependent answers and divergent diagnostics from the same solve.

**Where enforced.** The law and owner split are in
[service_surfaces_design.md](service_surfaces_design.md); architecture import
audits enforce the lower-layer direction and projection-only package boundaries.
The wall runs those audits as a gate.

### 11. Capture policy has explicit precision tiers

**Rule.** Closure capture selects exactly one central policy per captured
symbol: the full fact graph when the symbol is never reassigned in the body,
or a write-invariant declared/structural type once ordinary assignment syntax
writes it anywhere. Every capture routes through this one selector; nothing
downstream re-decides the tier.

**Why load-bearing.** Captures cross function boundaries and callbacks. Keeping
a live-value fact for a symbol that can be reassigned before the closure runs
would let a callee rely on a caller-local fact that no longer holds.

**Where enforced.**
`TestClosureCapturePolicyTierIsCentrallySelectedByWriteStatus` captures both
an unwritten and a later-written local from the same closure and asserts the
exported `ClosureCaptureFact.Policy` for each. `closureCapturePolicy` in
[`analysis/check/body/closure_capture.go`](../check/body/closure_capture.go)
is the sole selector: `ClosureCaptureFacts`/`closureCaptureFact` compute the
policy once per capture and consume it without re-deciding it.

### 12. ShapeID is an artifact, not a live binding

**Rule.** A future ShapeID is codegen/checker artifact data derived from solved
facts. It must not expose a live binder/environment handle or become a second
mutable semantic authority.

**Why load-bearing.** Live bindings would make artifact identity depend on
process lifetime and allow backend behavior to escape the solved snapshot.

**Where enforced.** This is a deferred design lock, not an implemented surface:
the WIR design states that ShapeID remains deferred to codegen work. Until an
implementation exists, review against
[analysis/ir/wir/design.md](../ir/wir/design.md) is the enforcement point; add
a schema/round-trip guard with the implementation rather than pretending one
already exists.

### 13. Solve-local interned IDs never serialize

**Rule.** `SegmentsID`, root intern IDs, and any whole-key scalar ID are valid
only in their producing `KeySpace`; they do not appear in summaries, manifests,
diagnostics, fixture oracles, or persistent caches. Cross-keyspace use rekeys
through canonical spelling.

**Why load-bearing.** Dense IDs depend on discovery order. Serializing one
would turn one solve's implementation detail into another solve's false path
identity.

**Where enforced.** `keyspace.go` states the scope restriction; the path-evidence
`RekeyValueLanes` boundary converts keys through source formatting before
interning them in the destination keyspace. See
[scalar_key_design.md](scalar_key_design.md) for the full migration contract.

### 14. Manifest evolution is append-only/additive and codec-oracle complete

**Rule.** Add boundary facts as additive lanes with conservative legacy decode.
Every registered wire lane must be populated and isolated by the codec oracle,
round-trip equal, and produce canonical permutation-invariant bytes.

**Why load-bearing.** Manifests cross module/version boundaries. Reinterpreting
an old omission as a new positive fact, or adding an untested descriptor,
silently changes callers' analysis.

**Where enforced.** `TestOperationalEffectsDescriptorCodecRoundTrips` checks
round trips and canonical bytes; `TestOperationalEffectsCodecOracleCoversEveryRegisteredLane`
fails when a registered lane lacks a rich or isolated oracle case. Legacy
suspension payloads stay unknown, hence conservatively may-suspend.

### 15. Compatibility façades and schema surfaces stay pinned until migration is real

**Rule.** Keep the `compiler/check`, `compiler/check/hooks`,
`compiler/check/scope`, and `types/query/core` façades while the external seam
imports them; remove a façade only after the seam no longer imports it. Any
schema surface change increments its registered version and updates its
structural hash pin and journal entry.

**Why load-bearing.** Deleting a compile-compatible name breaks the Wippy seam;
changing a wire/DTO shape without a version/hash update creates silent protocol
skew.

**Where enforced.** The façade packages are present at the pinned import paths
and checker façade behavior is covered by `compiler/check/global_types_test.go`.
Boundary, escape-vocabulary, and DTO/fact schema tests hash their live surfaces
and fail without a version entry; [SCHEMA_VERSIONS.md](SCHEMA_VERSIONS.md) is
the required registration and journal.
