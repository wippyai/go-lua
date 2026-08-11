// Package continuation owns the canonical internal continuation projection.
//
// It retains only lexical Cell scopes and unpolarized reaching Guard support
// for the existing executable candidate-bearing subjects.  Evaluation order
// and already-evaluated operands belong to evaluation; this package does not
// import or retain that projection.  The result is a provenance-fenced,
// allocation-free query surface over the compact lexical-scope chain and
// append-only Guard prefix DAG.
package continuation
