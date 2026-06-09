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
//   - Postflow/export projection lanes
//   - Final Summary-derived FunctionFacts projections for export/public APIs
//   - Module-level bindings and alias maps
//   - Query-tracked projection fact inputs for precise function-result
//     cache revalidation
//
// # Session Integration
//
// The store implements [api.StoreReader] and [api.CanonicalStore]. Canonical
// checking receives only [api.CanonicalStore], which excludes postflow projection
// reads and iteration writes. Noncanonical postflow callers use package-local
// owner interfaces instead of a public all-lane store bundle.
//
// # Projection Lane Visibility
//
// During postflow projection iteration, the store exposes visible lane values: the
// stable lanes from completed iterations overlaid with facts already produced in
// the current iteration. Query inputs track visible lane values per graph key.
package store
