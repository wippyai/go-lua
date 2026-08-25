// Package lineage owns the canonical proof-sidecar join authority.
//
// A lineage reference is an immutable model token.  This package gives those
// tokens one solve-local, content-addressed join operation.  The operation is
// deliberately separate from semantic value algebra, scope, geometry,
// schema plans, and physical state: lineage records why a cell is justified;
// it never changes what the cell means.
//
// Authorities are bound to one runtime fence by an explicit Factory.  A
// bound authority owns one append-only hash-consed arena of normalized atom
// sets.  References issued by other owners are opaque atoms; references under
// the authority's owner are accepted only when present in that arena.
package lineage
