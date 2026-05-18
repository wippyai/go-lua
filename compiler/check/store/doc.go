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
//   - Interprocedural facts (canonical function facts, parameter hints)
//   - Module-level bindings and alias maps
//   - Query-tracked interprocedural snapshot inputs for precise function-result
//     cache revalidation
//
// # Session Integration
//
// The store implements [api.StoreReader] and [api.IterationStore] interfaces,
// providing read access for queries and write access for the fixpoint driver.
//
// # Snapshot Isolation
//
// During fixpoint iteration, the store provides stable snapshots of
// interprocedural facts while allowing incremental updates. This ensures
// consistent reads during a single iteration pass.
package store
