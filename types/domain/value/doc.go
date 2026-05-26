// Package value owns the low-level structural relations of the abstract value
// domain over typ.Type.
//
// # Layering
//
// This package is the bottom layer: it operates directly on raw typ.Type and
// reuses the proven structural merge/equality/widening logic. It imports nothing
// from the axis packages. The per-axis lattices (package value/axis/*) import
// this package to reuse MergeForConvergence, Covers, and WidenForConvergence; the
// composed reduced product (package value/product) imports value and the axes and
// is the single package that exposes AbstractValue. The dependency direction is
// value <- axis <- product, which is acyclic.
//
// # Reusable structural relations
//
// The package holds the structural relations below return summaries, function
// facts, and whole fact products: optional elision, soft-placeholder preference,
// table-key truthiness refinement, recursive-growth detection, convergence
// widening, and unsafe precision-drop checks. The axis packages and the product
// facade compose these without reimplementing local helper clusters.
//
// AbstractValue, the opaque interned reduced product, is defined in package
// value/product rather than here: the axes import value, so defining AbstractValue
// here would form a value -> axis -> value import cycle. The single-value-facade
// law is satisfied by product being the one package that exposes AbstractValue and
// its operations.
package value
