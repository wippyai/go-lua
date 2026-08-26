// Package terminal owns the immutable public output of one relation solve.
//
// Runtime state remains owned by state/database and advances only through a
// publish settlement. Terminal owns the final database root, solve counters,
// and a typed application observation catalog keyed by the exact sealed
// dependency/operation pair. The catalog retains only Apply semantic result
// extents plus the observed root; the mounted contract already proves the
// unique occurrence identity, so no duplicate expression/node/kind directory
// is needed. It does not retain evaluator values, relation batches,
// settlements, callbacks, receipts, or a second row store.
package terminal
