# Boundary-fact descriptor flip plan

Stage-5 brick #2 added descriptor tables + parity oracles for three boundary-schema
lane families, additively (new files only). This document is the mechanical flip
recipe for after Stage 1 closes. The flip rewires each family's hand-wired lane
literal to derive from its descriptor table, then deletes the hand-wired literal.
The parity oracles already prove the derived behavior equals the hand-wired
behavior, so each flip is a promote-and-delete with no behavior change.

## Shared spine (already landed, no flip)

`analysis/engine/callboundary/lane_descriptors.go` defines the family-agnostic
spine reused by all three families:

- `BoundaryFactKind` — stable lane owner name.
- `BoundaryFactDescriptor[Ops]{ Kind, WireRef, Ops }` — one lane's name, its
  manifest OperationalEffects wire-lane cross-reference, and family-specific ops.
- `BoundaryFactTable[Ops]` — ordered, name-unique descriptor table with
  `Kinds()`, `Validate(family)`, and the generic `DeriveBoundaryLanes` driver.

`callboundary` is imported by both `callpayload` and `check/fixpoint/summary`, so
the spine is shared without a new package. It stays after every flip.

## Family B: NormalReturnFacts lanes (callboundary)

Descriptor table: `normalReturnFactDescriptors` (18 entries) in
`lane_descriptors.go`. Derive helper: `deriveNormalReturnLane`.

Flip steps:

1. In `normal_return_lanes.go`, replace the `var normalReturnFactLanes = []NormalReturnFactLane{ ... }`
   literal (lines ~216-359) with:
   ```go
   var normalReturnFactLanes = derivedNormalReturnFactLanes()
   ```
2. Delete the 18-entry literal and its inline `normalReturnSliceLane(...)` calls.
   Keep `normalReturnSliceLane`, `keepPath`, `keepRelOperand` — the descriptor
   entries call them.
3. In `lane_descriptors.go`, delete the three parity-only shims
   (`appendDerivedNormalReturnFacts`, `filterDerivedNormalReturnFacts`,
   `dropDerivedNormalReturnFacts`) and repoint the oracle at the public methods
   (`NormalReturnFacts.Append` / `FilterPaths` / `DropFactsTouchingPaths`), which
   now walk the derived `normalReturnFactLanes`.
4. `derivedNormalReturnFactLanes` may be inlined into the `normalReturnFactLanes`
   initializer or kept as the initializer expression.

Estimated deletion: ~145 lines of lane literal from `normal_return_lanes.go`;
net change roughly neutral (the closures move into the descriptor table), but
lane addition drops to one descriptor entry carrying its WireRef.

Post-flip the oracle (`lane_descriptors_test.go`) becomes a permanent
descriptor-integrity guard: derive order/field/filter parity, plus append /
filter / drop determinism over the rich corpus, plus the WireRef pin.

## Family C: CallOutcome lanes (callpayload)

Descriptor table: `callOutcomeDescriptors` (18 entries) in
`outcome_descriptors.go`. Derive helper: `deriveCallOutcomeLane`.

Flip steps:

1. In `call_outcome.go`, replace the `var callOutcomeLanes = []callOutcomeLane{ ... }`
   literal (lines ~174-255) with:
   ```go
   var callOutcomeLanes = derivedCallOutcomeLanes()
   ```
2. Delete the 18-entry literal. The `callOutcomeLane` struct definition stays
   (the derive helper builds it).
3. Delete the parity-only shims `derivedCallOutcomeEmpty` and
   `derivedCallOutcomeHasPostReturnEvidence`; repoint the oracle at the public
   `CallOutcome.Empty` / `HasPostReturnEvidence`, which now walk the derived
   lanes.

Estimated deletion: ~82 lines of lane literal from `call_outcome.go`.

## Family A: summary slots (check/fixpoint/summary)

Descriptor table: `summaryFactDescriptors` (16 entries) in `fact_descriptors.go`.
Derive helper: `deriveSummaryLane`.

Flip steps:

1. In `summary_lanes.go`, replace the `var summaryLanes = []summaryLane{ ... }`
   literal (lines ~20-373) with:
   ```go
   var summaryLanes = derivedSummaryLanes()
   ```
2. Delete the 16-entry literal. The `summaryLane` struct definition and the
   shared helpers (`cloneSlice`, `trimTrailingProducts`, and the
   `summaryLanesEmpty` / `summaryNonSlotLanes*` drivers) stay unchanged — they
   already iterate `summaryLanes`.
3. Delete the parity-only helper `derivedSummaryLanes` or fold it into the
   initializer.

Estimated deletion: ~350 lines of lane literal from `summary_lanes.go` (the
closures move into the descriptor table, so net production line count is roughly
neutral; the payoff is one uniform descriptor shape across all three families and
the WireRef cross-reference).

The whole-summary `Join` / `Widen` / `Equal` / `LessOrEq` / `Normalize` drivers
in `summary_lattice.go` are unchanged: they call the `summaryNonSlotLanes*`
helpers, which walk `summaryLanes`. Because the parity oracle proves the derived
lane table equals the hand-wired table lane-for-lane (order, field, slot flag,
every non-nil op behavior including panic behavior), these drivers stay invariant.

## WireRef mapping notes

WireRef links each boundary-fact kind to the manifest `OperationalEffects` wire
codec (`analysis/module/manifest/operational_effects_codec_descriptor.go`) by
field name. Ground truth:

- NormalReturnFacts lanes: lowered from `signature.OperationalEffects` in
  `analysis/engine/effectlowering/provider_operational.go` (+ `provider.go`).
  1:1 lanes carry the wire field; `PathRefinements` maps to both
  `NormalReturnPresenceRefinements` and `NormalReturnTypeRefinements`. Local-only
  lanes (PersistentPathWrites, DynamicAllValues, ChannelSelects, EffectDeltas,
  NumFloors, RelConstraints) carry nil — they never cross a signature boundary.
- CallOutcome / summary: only `ReturnPresenceRelations` maps 1:1 to the
  `ReturnPresenceRelations` wire lane (verified in
  `analysis/check/exportmanifest/function_signatures.go`). `NormalReturnFacts` is
  a nested family delegating to `NormalReturnFactDescriptors`. All other lanes
  are caller-relative param/return evidence or return-type/param-obligation
  summary lanes serialized through the signature return/param/postcondition
  encoders, not the OperationalEffects wire codec, so they carry nil.

The WireRef column is advisory metadata for the future summary -> NormalReturnFacts
-> wire generation pass sketched in the manifest codec descriptor doc comment. It
is not load-bearing for any current behavior; the oracles pin it against the
lowering ground truth so it cannot silently drift.

## factapply (read-only, not flipped here)

`analysis/engine/factapply/normal_return_apply_lanes.go` already binds
per-lane apply handlers to the NormalReturnFacts lane IDs via
`callboundary.BindNormalReturnFactLanes`. It is unaffected by this flip: the lane
IDs (= descriptor kinds) are stable, and `BindNormalReturnFactLanes` orders
handlers by the storage-lane registry, which after the flip derives from the
descriptor table with identical IDs and order. No factapply edit is required for
the flip. The eventual generation pass that produces NormalReturnFacts from a
summary would attach a per-kind projector to these same descriptors, but that is
out of scope for the flip.
