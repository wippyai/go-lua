// Package relation is Placement's typed relation boundary.
//
// Placement values become opaque semantic ValueTokens only through the owner
// columns in this package. The relation engine therefore never names
// placement.Fact, placement.Placement, or Placement's lattice directly. The
// hand-authored operation half calls only Placement-owned mathematics; emitted
// adapters decode an already-sealed frame and encode its declared result.
//
// Allocation birth is the first bound family: Value issues its exact
// allocation receipt and fresh fact, while Placement owns the initial Fact it
// publishes. Formal is the first joined scalar family. Containment's owner
// route relation consumes its complete Placement and Heap inputs, retains the
// authenticated parent Fact beside the selected child Fact, and then binds
// only those two scalar columns. Capture and Store are the four-input
// Placement bindings, carrying Heap's route key or Value's opaque storage
// receipt alongside Placement's route tag and selected Fact. Gaps records
// every remaining Placement signature and the owner boundary it still needs.
package relation
