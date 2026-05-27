package constraint

import (
	"github.com/wippyai/go-lua/types/lattice"
)

// Domain is the abstract domain of path conditions.
//
// The carrier is DNF over the literal vocabulary appearing syntactically in
// the program (see DOMAIN_DESIGN.md §1–§3). The ordering is logical
// implication: a ⊑ b iff every state satisfying a also satisfies b. Bottom
// is FalseCondition (unsatisfiable), Top is TrueCondition (always true).
//
// Widen is the projection widening defined in DOMAIN_DESIGN.md §6 and
// implemented by Condition.WidenAgainst — see widen.go. Calling Widen
// directly in tight loops is unnecessary; production callers (the worklist
// in types/flow/propagate) apply Widen only at the feedback vertex set
// (DOMAIN_DESIGN.md §7).
//
// This struct value IS the agreement with types/lattice. Per DOMAIN_DESIGN.md
// §9, no adapter struct wraps anything — the Lattice contract is a struct of
// function fields and Domain populates each field directly.
var Domain = lattice.Lattice[Condition]{
	Bottom:   FalseCondition,
	Top:      TrueCondition,
	Equal:    func(a, b Condition) bool { return a.Equals(b) },
	LessOrEq: func(a, b Condition) bool { return b.Subsumes(a) },
	Join:     Or,
	Meet:     And,
	Widen:    func(prev, next Condition) Condition { return prev.WidenAgainst(next) },
}
