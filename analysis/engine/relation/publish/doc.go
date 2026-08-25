// Package publish is the sole authenticated atomic publication door from one
// evaluated apply.Application into solve-local relation state. It preserves
// the semantic outcome separately from any cell delta: absence and refusal
// are never fabricated as lattice terminals. A successful write returns a
// Settlement whose roots and sparse delta are committed only by the state
// transaction's authenticated root swap.
package publish
