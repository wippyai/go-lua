# Schema Versions

These versions are consumer contracts for the wippy v2 runtime lane. Runtime
CCS projection and GSPO reward code pin them explicitly; producers must fail
closed or negotiate before emitting a surface newer than the consumer supports.

| Surface | Constant | Current | Covers | Bump when |
| --- | --- | --- | --- | --- |
| Checker embedding | `embedding.EmbeddingSchemaVersion` | v1 | Stable document/source/resolution identity DTOs: `DocumentID`, digest-bound source locations and snapshots, unit plans/imports, resolution snapshots, `SolveSeq`, and `BodyInputDigest`. | An exported embedding DTO field, initial document scheme, or its identity/versioning semantics changes. |
| Judgment IR | `judgment.JIRSchemaVersion` | v11 | Judgment code registry and exported judgment record shape. | A judgment code, code metadata, or exported judgment/evidence/subject field shape changes. |
| Signature escape vocabulary | `signature.EscapeVocabVersion` | v1 | Signature `EscapeKind` labels and audited ownership capability labels synced with arena CallArgEscape/Ownership. | An escape/ownership label is added, removed, renamed, or changes boundary meaning. Requires joint cross-repo signoff per fence #1425. |
| Boundary lane schema | `summary.BoundaryLaneSchemaVersion` | v8 | Summary descriptors, `NormalReturnFacts` descriptors, `CallOutcome` descriptors, and manifest wire-lane links. | A lane kind, slot/post-return classification, storage owner, or wire reference changes. |
| Closure capture DTO | `readmodel.ClosureCaptureSchemaVersion` | v2 | Codegen-facing exported `readmodel.ClosureCapture` record shape. | A `ClosureCapture` field is added, removed, renamed, or changes type. |
| Hoistable load DTO | `readmodel.HoistableLoadSchemaVersion` | v1 | Codegen-facing exported `readmodel.HoistableLoad` record shape. | A `HoistableLoad` field is added, removed, renamed, or changes type. |
| Allocation site DTO | `readmodel.AllocationSiteSchemaVersion` | v2 | Codegen-facing exported `readmodel.AllocationSite` and `readmodel.AllocationField` record shapes. | An `AllocationSite`/`AllocationField` field is added, removed, renamed, or changes type. |
| Allocation site fact | `body.AllocationSiteFactSchemaVersion` | v3 | Internal solved `body.AllocationSiteFact` record shape plus its `StableShapeField`/`SourceSpan` payload types. | An `AllocationSiteFact` field, or a payload type it embeds, is added, removed, renamed, or changes type. |
| Artifact debug map | `service.DebugMapSchemaVersion` | v2 | `BodyDebugMap`, `DebugMapEntry`, phase vocabulary, canonical entry encoding, and `StaticArtifactID` DTO form. | A debug-map/anchor/local field, phase label, canonical encoding, or static-artifact ID component changes. |

## Bump Discipline

Every bump requires:

1. Update the version constant.
2. Update that version's expected guard hash in the package test.
3. Journal a D-entry describing the changed surface and migration impact.
4. Notify pinned consumers, including the runtime CCS projector and GSPO reward
   reader, before the producer emits the new version by default.

The guard tests intentionally hash live registries and descriptor tables. A
surface change without a version bump fails with: `surface changed: bump version
constant + journal a D-entry`.

## Journal

- D20: Post-union repin: Judgment IR v11 is the single consumer contract for
  the declared typestate-requirement codes, `BodyInputDigest` and digest-bound
  `SourceLocation` spans, and exported repair descriptors. Artifact debug maps
  are v2: their static body identity uses `BodyInputDigest`, debug locals use
  body-scoped `LocalID`s, and the engine build tag is `go-lua-engine-debug-map-v2`.
  Embedding v1, boundary v8, and the guarded codegen DTO/fact versions were
  rechecked unchanged. Consumers must negotiate these final versions rather
  than any transient integration pin.

- D19: Added the v1 artifact debug-map schema. WIR lowering assigns
  body-scoped RPO sequence-point ordinals and phase-qualified IDs; completed
  results publish canonical per-body maps and static artifact IDs. The map's
  lexical before/after visibility sets close the prior DbgLocal live-range-end
  marker gap for observable points. Runtime consumers must join live facts on
  the exact static artifact plus ordinal and phase, never on `cfg.Point` or a
  bare body version.

- D18: Judgment IR judgment/span surface changed with the embedding migration:
  `Judgment.ResultVersion` is now the explicitly named `BodyInputDigest`, and
  `SpanRef` carries a digest-bound `embedding.SourceLocation`. The legacy
  `SpanRef.File` remains a display-only compatibility projection while
  materialized service results populate the semantic location. This input is
  incorporated in the final v11 union repin recorded by D20.

- D17: Registered checker embedding schema v1 and its reflection-hash guard.
  This is independent of JIR: it pins the stable document identity/snapshot
  algebra used to materialize service inputs before a solve. `DocumentID`
  equality is `(Scheme, OpaqueKey)` only; source/provider/editor revisions,
  content digests, and workspace resolution views remain distinct snapshots.

- D16: Bumped boundary lane schema to v8 and Judgment IR to v10. Added the
  additive manifest `OperationalEffects.typestateRequirements` wire lane and
  the caller-relative `CallOutcome.TypestateRequirements` obligation lane.
  Registered default-enabled `typestate.invalid_requirement` errors and
  `typestate.unproven_requirement` warnings for declared lifecycle call-entry
  requirements.

- D15: Bumped boundary lane schema to v7. Added the internal
  `ProtectedCallTypestate` summary and call-outcome lanes so callback normal and
  exceptional lifecycle snapshots reach `pcall`/`xpcall` before their caught
  continuation joins the states. This lane has no manifest wire encoding.

- D14: Bumped Judgment IR to v9. Registered the default-enabled generic
  `typestate.invalid_transition` error judgment and made declared
  `effect.lifecycle.unreleased` obligations default-enabled warnings. Ambient
  channel failures continue to render through their existing `channel.*.closed`
  codes, so no channel consumer migration is required. Error-path transport
  across `pcall` remains the known L-gap: a final transition proven only inside
  an error path stays unproven and surfaces as the lifecycle warning.

- D13: Bumped boundary lane schema to v6. Added the additive
  `OperationalEffects.suspensionKnown` wire lane, which explicitly certifies
  that `maySuspend` is exhaustive. Older manifests without this bit decode as
  suspension-unknown and therefore remain conservatively may-suspend.

- D12: Registered structural guard tests for the four codegen DTO/fact schema
  pins (`readmodel.ClosureCaptureSchemaVersion`,
  `readmodel.HoistableLoadSchemaVersion`, `readmodel.AllocationSiteSchemaVersion`,
  `body.AllocationSiteFactSchemaVersion`). These constants previously had no
  registry entry and their tests only compared the constant to itself, so a
  field change could land without a version bump. No version changed; this
  entry only adds the reflection-hash guard mechanism already used by the JIR
  and boundary lane schemas.

- D11: Bumped boundary lane schema to v5. Added the normal-return
  `PathStaticMemberDeltas` lane and manifest
  `OperationalEffects.pathStaticMemberDeltas` wire lane for staged-shape
  interprocedural static field additions. The lane is additive; old readers
  that lack it retain the prior conservative prefix invalidation behavior.

- D10: Bumped boundary lane schema to v4. Added the manifest
  `OperationalEffects.returnFlows` wire lane for the closed phase-1 return-flow
  relations `ReturnsParam` and `ReturnsParamMember`. This is additive:
  legacy `paramRelations[].throughReturn` remains emitted for old readers.
  Phase 2 container/element return-flow relations are intentionally not part of
  this schema.

- D9: Bumped boundary lane schema to v3. The new manifest `ParamRelations`
  wire lane lowers into the existing normal-return `EscapeEvents` and
  `StoreRelations` owner lanes, so descriptor wire references changed without
  adding new solver or call-boundary storage lanes.

- D8: Added default-enabled channel lifecycle error judgments
  `channel.send.closed` and `channel.close.closed`, both rendered by the new
  `channel_lifecycle` renderer with `channel.*.closed` diagnostic codes. This
  extends the code registry only; exported judgment record shapes are unchanged.
