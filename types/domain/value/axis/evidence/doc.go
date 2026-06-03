// Package evidence is the SemanticEvidence axis of the reduced-product abstract
// value.
//
// # Axis law
//
// The SemanticEvidence axis abstracts the path-sensitive proofs attached to a
// value: discriminant/closed-variant exhaustiveness, type-predicate and :is
// refinements, tuple/sibling correlation tokens for error and multi-return
// patterns, occurrence and index-presence evidence, generic-for key/value
// evidence, and gradual any/unknown demotion. Its lattice orders evidence by
// information: Bottom is unreachable, Top carries no evidence, and Join keeps only
// evidence proven on all incoming paths.
//
// The axis is independently sound: Join never asserts evidence that fails on some
// path, so any narrowing derived from it is valid on every path reaching the join.
//
// # Status
//
// design step 2 establishes the package and the local lattice surface so the design step 5
// composition facade can import it. The evidence carriers and the
// discriminant/predicate/correlation/gradual reducers are implemented in design step 5;
// the law tests are skipped until then.
package evidence
