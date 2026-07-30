# Boundary-Fact Descriptor Registry

The boundary-schema lane families now derive their live registries from
descriptor tables:

- `callboundary.normalReturnFactLanes` derives from
  `normalReturnFactDescriptors`.
- `callpayload.callOutcomeLanes` derives from `callOutcomeDescriptors`.
- `summary.summaryLanes` derives from `summaryFactDescriptors`.

The descriptor spine lives in `analysis/engine/callboundary/lane_descriptors.go`:

- `BoundaryFactKind` is the stable lane owner name.
- `BoundaryFactDescriptor[Ops]` names one lane, its optional manifest wire-lane
  references, and the family-specific operations.
- `BoundaryFactTable[Ops]` preserves canonical order and validates unique kinds.
- `DeriveBoundaryLanes` builds a family registry from the table.

The tests remain as live integrity guards:

- `analysis/engine/callboundary/lane_descriptors_test.go`
- `analysis/engine/callpayload/outcome_descriptors_test.go`
- `analysis/check/fixpoint/summary/fact_descriptors_test.go`

`WireRef` links a descriptor kind to the manifest `OperationalEffects` wire
codec where a lane crosses a signature boundary. Lanes that are local-only or
serialized through return/param/postcondition encoders carry no wire ref.

The next registry step is not another lane-literal flip; those are complete.
Remaining work is to audit projection/application/wire-codec duplication and
prove that adding a new persistent fact kind touches no more than two production
files.
