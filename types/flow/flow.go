// Package flow provides AST-free input carriers, value algebras, and
// producer-neutral fact surfaces for the type checker.
//
// The executable fixpoint lives under compiler/check/canonical. This package
// keeps the shared point-state facts, path queries, condition proofs, transfer
// laws, and deterministic input normalization used by that engine and its
// consumers.
//
// # Subpackages
//
//   - domain: type and shape domains used by condition proof projection
//   - numeric: numeric range and interval tracking
//   - pathkey: normalized path-key resolution for SSA versions
//   - propagate: condition propagation through the CFG
package flow
