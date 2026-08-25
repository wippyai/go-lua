package unary

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Scan redeems one state-owned Reader. The reader is the only source of
// logical rows, physical arrangements, typed cells, scope and lineage. The
// unary package does not rebuild any of those structures and does not retain
// rows after the callback returns.
func Scan(input read.Reader, visit func(read.Row) bool) (completed, valid bool) {
	if input == nil || visit == nil || !input.Layout().Available() {
		return false, false
	}
	return input.Scan(func(row read.Row) bool {
		if row == nil || !row.Available() {
			return false
		}
		return visit(row)
	})
}

// Select applies logical scope entailment to one reader stream. Scope is
// data: no point/edge/topology rewrite is performed, and no mask reaches a
// fold. Rows that do not satisfy the selected scope are simply not emitted.
// A malformed row or foreign scope is refused by the reader/mount boundary.
func Select(input read.Reader, mounted witness.Mounted, selected witness.Scope, visit func(read.Row) bool) (completed, valid bool) {
	if input == nil || !mounted.Available() || !selected.Available() || !selected.ValidFor(mounted.RuntimeFence()) || visit == nil || !input.Layout().ValidFor(mounted.Fence()) {
		return false, false
	}
	refused := false
	completed, valid = Scan(input, func(row read.Row) bool {
		if !row.Scope().ValidFor(mounted.RuntimeFence()) {
			refused = true
			return false
		}
		if !mounted.EntailsScopes(row.Scope(), selected) {
			return true
		}
		return visit(row)
	})
	if refused {
		return false, false
	}
	return completed, valid
}
