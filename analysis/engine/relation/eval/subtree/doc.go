// Package subtree redeems one mount-sealed correlated relation subtree.
//
// The package is deliberately a small interpreter for the physical
// Input/Select/Join/Complete vocabulary.  It consumes only the
// CorrelatedSubtree witness issued by arrangement; it does not reconstruct a
// plan, scan a relation, or retain a cross-root row cache.
package subtree
