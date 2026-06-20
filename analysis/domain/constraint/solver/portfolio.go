package solver

import (
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
)

// Portfolio runs a cheapest-first list of theory backends and decides
// entailment by refutation.
type Portfolio struct {
	backends []func() Solver
}

// NewPortfolio builds a portfolio from backend constructors, cheapest first.
func NewPortfolio(backends ...func() Solver) Portfolio {
	return Portfolio{backends: backends}
}

// DefaultPortfolio is the standard portfolio: difference logic first, then the
// linear-arithmetic backend for goals the cheaper diff theory cannot prove.
func DefaultPortfolio() Portfolio {
	return NewPortfolio(NewDiffBackend, NewLinearBackend)
}

// Entails reports whether asserted entails goal.
//
// Entailment-by-refutation: for each backend a fresh Solver asserts all of
// asserted, then is queried for goal. A Valid from any backend proves the goal.
// An Invalid (only a complete backend ever returns it) refutes the goal and
// short-circuits to Invalid. Unknown continues to the next backend. After all
// backends, Valid if any proved it, else Unknown.
//
// This is entailment, not satisfiability aggregation, so it does not use
// decision.Result.Combine.
func (p Portfolio) Entails(asserted []numeric.NumericConstraint, goal numeric.NumericConstraint) decision.Result {
	proved := false
	for _, make := range p.backends {
		s := make()
		for _, c := range asserted {
			s.Assert(c)
		}
		switch s.Entails(goal) {
		case decision.Valid:
			proved = true
		case decision.Invalid:
			return decision.Invalid
		case decision.Unknown:
		}
	}
	if proved {
		return decision.Valid
	}
	return decision.Unknown
}
