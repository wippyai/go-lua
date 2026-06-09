// Package interproc handles the postflow projection fact product.
//
// This package processes analysis results after noncanonical flow analysis converges,
// extracting projection facts for noncanonical compatibility/export paths. Canonical
// checking uses Summary instead.
//
// # Post-Flow Processing
//
// After flow analysis completes for a function, this package:
//   - Extracts return type summaries
//   - Computes parameter evidence from call sites
//   - Identifies captured variable assignments
//   - Propagates effect information
//
// # Projection Fact Propagation
//
// Projection facts enable noncanonical paths for:
//   - Return type inference across call boundaries
//   - Parameter type inference from argument types
//   - Captured variable type tracking in nested functions
//
// # Integration
//
// This package connects per-function flow analysis with the projection product
// iteration that resolves noncanonical cross-function dependencies.
package interproc
