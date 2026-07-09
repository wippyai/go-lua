# Schema Versions

These versions are consumer contracts for the wippy v2 runtime lane. Runtime
CCS projection and GSPO reward code pin them explicitly; producers must fail
closed or negotiate before emitting a surface newer than the consumer supports.

| Surface | Constant | Current | Covers | Bump when |
| --- | --- | --- | --- | --- |
| Judgment IR | `judgment.JIRSchemaVersion` | v8 | Judgment code registry and exported judgment record shape. | A judgment code, code metadata, or exported judgment/evidence/subject field shape changes. |
| Signature escape vocabulary | `signature.EscapeVocabVersion` | v1 | Signature `EscapeKind` labels and audited ownership capability labels synced with arena CallArgEscape/Ownership. | An escape/ownership label is added, removed, renamed, or changes boundary meaning. Requires joint cross-repo signoff per fence #1425. |
| Boundary lane schema | `summary.BoundaryLaneSchemaVersion` | v4 | Summary descriptors, `NormalReturnFacts` descriptors, `CallOutcome` descriptors, and manifest wire-lane links. | A lane kind, slot/post-return classification, storage owner, or wire reference changes. |

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

- D8: Added default-enabled channel lifecycle error judgments
  `channel.send.closed` and `channel.close.closed`, both rendered by the new
  `channel_lifecycle` renderer with `channel.*.closed` diagnostic codes. This
  extends the code registry only; exported judgment record shapes are unchanged.
- D9: Bumped boundary lane schema to v3. The new manifest `ParamRelations`
  wire lane lowers into the existing normal-return `EscapeEvents` and
  `StoreRelations` owner lanes, so descriptor wire references changed without
  adding new solver or call-boundary storage lanes.
- D10: Bumped boundary lane schema to v4. Added the manifest
  `OperationalEffects.returnFlows` wire lane for the closed phase-1 return-flow
  relations `ReturnsParam` and `ReturnsParamMember`. This is additive:
  legacy `paramRelations[].throughReturn` remains emitted for old readers.
  Phase 2 container/element return-flow relations are intentionally not part of
  this schema.
