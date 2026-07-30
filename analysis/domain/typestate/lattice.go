package typestate

import "github.com/wippyai/go-lua/analysis/domain/lattice"

// Domain is the finite-height lattice for typestate stores.
var Domain = lattice.Lattice[Store]{
	Bottom:   Empty,
	Top:      top,
	Equal:    Equal,
	LessOrEq: LessOrEq,
	Join:     Join,
	Meet:     Meet,
	Widen:    Widen,
}

func top() Store {
	return Store{top: true}
}
