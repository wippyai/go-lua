// Package abstract owns the checker abstract interpreter boundary.
//
// The checker is organized as one product interpreter:
//   - transfer lowers CFG/AST events into flow inputs,
//   - flow solves those inputs to a product state,
//   - query projects stable facts from the product state,
//   - domain packages own join, widening, and equality laws.
package abstract
