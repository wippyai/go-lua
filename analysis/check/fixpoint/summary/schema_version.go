package summary

// BoundaryLaneSchemaVersion pins the call-boundary lane descriptor surface. It
// covers summary slot descriptors, NormalReturnFacts lane descriptors,
// CallOutcome lane descriptors, and their manifest wire-lane links. Bump it
// when a lane kind, slot/post-return classification, storage field owner, or
// wire reference is added, removed, renamed, reordered semantically, or changes
// boundary meaning.
const BoundaryLaneSchemaVersion = 1
