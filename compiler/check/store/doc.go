// Package store provides session state management for type checking.
//
// This package implements the storage layer for analysis results during
// type checking. It maintains the mapping between function identifiers
// and their analysis results, supporting both single-function checks
// and full-module analysis.
//
// # Store Contents
//
// The store holds:
//   - Built CFGs indexed by graph ID
//   - Analysis results (types, flow facts, diagnostics) per function
//   - Legacy compatibility fact products
//   - Final Summary-derived FunctionFacts projections for export/public APIs
//   - Module-level bindings and alias maps
//   - Query-tracked legacy fact inputs for precise function-result
//     cache revalidation
//
// # Session Integration
//
// The store implements [api.StoreReader] and [api.IterationStore] interfaces.
// Canonical checking receives only [api.CanonicalStore], which excludes legacy
// product reads and legacy iteration writes.
//
// # Legacy Product Visibility
//
// During legacy fixpoint iteration, the store exposes the visible product: the
// stable product from completed iterations overlaid with facts already produced
// in the current iteration. Query inputs track that visible product per graph key.
package store
