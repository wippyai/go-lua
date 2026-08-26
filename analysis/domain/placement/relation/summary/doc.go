// Package summary contains Placement's owner-side semantic boundary for the
// two-row summary family.
//
// The package deliberately stops at typed operation values and codecs.  It
// does not own a relation store, a detached SummaryResult, or a query reader;
// the mounted schema and the relation substrate own those concerns.  A child
// row is one Heap allocation identity paired with the canonical Placement
// Fact and Placement-owned AllocationEvidence. The parent row carries only
// the exact Placement schema identity; its canonical cell presence is the
// answer marker.
package summary
