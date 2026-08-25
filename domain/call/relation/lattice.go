package relation

import (
	calldomain "github.com/wippyai/go-lua/domain/call"
)

// CallLattice is domain/call's ascent authority, spelled as methods on a
// sealed algebra.
type CallLattice struct {
	algebra *calldomain.Algebra
}

// NewCallLattice adopts one sealed call algebra.
func NewCallLattice(algebra *calldomain.Algebra) (CallLattice, bool) {
	if algebra == nil || !algebra.Valid() {
		return CallLattice{}, false
	}
	return CallLattice{algebra: algebra}, true
}

func (witness CallLattice) Join(left, right calldomain.Value) (calldomain.Value, bool) {
	return witness.algebra.Join(left, right)
}

func (witness CallLattice) Widen(previous, next calldomain.Value) (calldomain.Value, bool) {
	return witness.algebra.Widen(previous, next)
}

func (witness CallLattice) LessOrEq(left, right calldomain.Value) bool {
	return witness.algebra.LessOrEq(left, right)
}
