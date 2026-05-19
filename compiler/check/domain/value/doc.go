// Package value owns reusable structural relations over typ.Type values.
//
// These relations are below return summaries, function facts, and whole fact
// products: optional elision, soft-placeholder preference, table-key truthiness
// refinement, recursive-growth detection, convergence widening, and unsafe
// precision-drop checks live here so higher domains can compose them without
// reimplementing local helper clusters.
package value
