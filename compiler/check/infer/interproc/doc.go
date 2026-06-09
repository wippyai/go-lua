// Package interproc handles the legacy post-flow fact product.
//
// This package processes analysis results after old flow analysis converges,
// extracting legacy facts for noncanonical compatibility/export paths. Canonical
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
// # Legacy Fact Propagation
//
// Legacy facts enable old paths for:
//   - Return type inference across call boundaries
//   - Parameter type inference from argument types
//   - Captured variable type tracking in nested functions
//
// # Integration
//
// This package connects per-function flow analysis with the legacy product
// iteration that resolves old cross-function dependencies.
package interproc
