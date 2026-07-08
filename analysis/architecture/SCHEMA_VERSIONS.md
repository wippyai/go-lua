# Schema Versions

These versions are consumer contracts for the wippy v2 runtime lane. Runtime
CCS projection and GSPO reward code pin them explicitly; producers must fail
closed or negotiate before emitting a surface newer than the consumer supports.

| Surface | Constant | Current | Covers | Bump when |
| --- | --- | --- | --- | --- |
| Judgment IR | `judgment.JIRSchemaVersion` | v3 | Judgment code registry and exported judgment record shape. | A judgment code, code metadata, or exported judgment/evidence/subject field shape changes. |
| Signature escape vocabulary | `signature.EscapeVocabVersion` | v1 | Signature `EscapeKind` labels and audited ownership capability labels synced with arena CallArgEscape/Ownership. | An escape/ownership label is added, removed, renamed, or changes boundary meaning. Requires joint cross-repo signoff per fence #1425. |
| Boundary lane schema | `summary.BoundaryLaneSchemaVersion` | v1 | Summary descriptors, `NormalReturnFacts` descriptors, `CallOutcome` descriptors, and manifest wire-lane links. | A lane kind, slot/post-return classification, storage owner, or wire reference changes. |

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
