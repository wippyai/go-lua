package product

import (
	"github.com/wippyai/go-lua/types/lattice"
)

// Domain is the abstract domain of value-level converged facts.
//
// The carrier is the interned reduced product AbstractValue (see value.go);
// each constituent axis has its own lattice and Domain joins/widens
// component-wise then reduces (DOMAIN_DESIGN.md §1–§5).
//
// LessOrEq is the carrier/join-induced order — a ⊑ b iff Join(a, b) = b —
// not Covers. The two are distinct: Covers is the semantic coverage
// preorder ("γ(b) ⊆ γ(a)"), and pairs like the bare record vs
// typ.NewAlias(...) wrapping the same record, or typ.Unknown vs typ.Any,
// mutually cover but intern to distinct canonical nodes. Using Covers
// directly would violate antisymmetry against the carrier-identity Equal
// (DOMAIN_DESIGN.md §4). The join-induced order is antisymmetric with
// Equal by construction and sound with respect to Covers (the converse
// may fail; see domain_test.go TestDomain_LessOrEqImpliesCovers).
//
// Meet is intentionally nil: this is a forward-analysis domain with no
// analyzer surface consuming a greatest lower bound across all axes. The
// LawSuite harness skips the meet-side laws (and absorption) on a nil
// Meet; see types/lattice/laws.go and DOMAIN_DESIGN.md §6.
//
// This struct value IS the agreement with types/lattice. Per the
// "no adapter" directive, no wrapper struct mediates the contract —
// Domain populates each function field directly with the existing
// package-level operations.
var Domain = lattice.Lattice[AbstractValue]{
	Bottom:   Bottom,
	Top:      Top,
	Equal:    Equal,
	LessOrEq: func(a, b AbstractValue) bool { return Equal(Join(a, b), b) },
	Join:     Join,
	Meet:     nil,
	Widen:    Widen,
}
