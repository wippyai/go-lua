// Package guard records guard-related facts that are not condition formulas.
//
// Condition expressions are normalized by compiler/check/domain/conditionexpr
// and compiler/check/domain/cond into constraint.Condition values. This package
// intentionally does not own a parallel AST-to-path guard language.
//
// # Remaining Responsibilities
//
// Guard owns small, non-formula facts used by transfer and synthesis:
//
//   - parsing and evaluating builtin type(expr) comparisons;
//   - graph facts for T:is(x) success-edge narrowing;
//   - graph facts for local predicate functions and assigned predicate results.
//
// Any API here that starts propagating path-key maps is a design regression:
// branch meaning should flow through constraint.Condition and canonical access
// paths instead.
package guard
