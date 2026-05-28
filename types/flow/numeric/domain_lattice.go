// domain_lattice.go wires *numeric.State to the lattice.Lattice contract.
//
// Per DOMAIN_DESIGN.md §7 (rev 3), the lattice variable is named StateDomain
// (not Domain — that identifier is taken by the ProductDomain numeric.Domain
// struct at domain.go:42; Go has a single package-level namespace). No adapter
// type is used; the struct of function fields IS the contract and points
// directly at the package-level State operations.
package numeric

import (
	"github.com/wippyai/go-lua/types/lattice"
)

// StateDomain is the lattice contract for the numeric carrier (*State).
//
// Meet is left nil: numeric is forward-analysis. The natural binary meet is
// constraint conjunction, but the codebase consumes it via the transfer-
// specific State.ApplyXxx surfaces (atom_applier.go, theory.go), not via a
// generic Meet(a, b *State). Leaving Meet nil honestly reflects the consumer
// surface; LawSuite skips the meet-side laws automatically.
var StateDomain = lattice.Lattice[*State]{
	Bottom:   Bottom,
	Top:      Top,
	Equal:    func(a, b *State) bool { return a.Equals(b) },
	LessOrEq: LessOrEq,
	Join:     Join,
	Meet:     nil,
	Widen:    Widen,
}
