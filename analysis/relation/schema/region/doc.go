// Package region owns the owner-neutral Boolean algebra used to describe a
// relation decision scope.
//
// A Region is a sealed reduced ordered binary decision diagram (ROBDD).  Its
// atoms are already-issued identity.ContentID values; this package never
// invents an owner, derives a model identity, or interprets an atom as a
// physical coordinate.  False and True are explicit terminals.  Every
// non-terminal is an if-then-else node whose low and high edges are immutable
// references into the sealed DAG.
//
// Transport rows are deliberately separate from the sealed Region.  The
// transport boundary validates the complete root-reachable graph, removes
// transport ordinals from canonical identity, and returns defensive views for
// later physical lowering.
package region
