// Package tuple owns the one transient relational row representation used by
// the W3 evaluator. A Tuple is not state and does not mint identities: it is
// an immutable, fenced frame of already-read cells, source row identities,
// one normalized decision scope, and one joined lineage reference. Batch is
// the sole immutable replay boundary for ordered Tuple vectors; it carries
// the same exact mount fence and one common scope without adding a callback,
// interface, or identity authority.
package tuple
