package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/lattice"
)

// TestConditionLattice_Laws applies the standard abstract-domain law
// harness to the Condition-DNF lattice. The sample covers Top, Bottom,
// single-conjunct, multi-conjunct, and multi-disjunct conditions over a
// small fixed set of literals. Any law violation reported here is a
// real, reproducible defect in the Condition domain.
//
// Forge journal seq 304 designates this domain as the source of fixture
// non-termination; see Phase F deliverable in that entry.
func TestConditionLattice_Laws(t *testing.T) {
	x := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	y := Path{Root: "y", Symbol: cfg.SymbolID(2)}
	z := Path{Root: "z", Symbol: cfg.SymbolID(3)}

	cxTruthy := FromConstraints(Truthy{Path: x})
	cyTruthy := FromConstraints(Truthy{Path: y})
	czTruthy := FromConstraints(Truthy{Path: z})
	cxNotNil := FromConstraints(NotNil{Path: x})

	cxyAnd := And(cxTruthy, cyTruthy)
	cxyOr := Or(cxTruthy, cyTruthy)
	cxyzAnd := And(cxyAnd, czTruthy)

	// A 4-disjunct condition mirrors the loop-preheader reinforcement
	// pattern: each `if` branch introduces a disjunct, and a subsequent
	// Meet against the running condition cross-products. With 4 disjuncts
	// in growth, the chain `t ← Meet(t, growth)` runs at 4 → 16 → 64 → 256
	// → 1024 disjuncts in raw DNF.
	branchDisjuncts := Or(
		Or(cxTruthy, cyTruthy),
		Or(czTruthy, cxNotNil),
	)

	sample := []Condition{
		FalseCondition(),
		TrueCondition(),
		cxTruthy,
		cyTruthy,
		czTruthy,
		cxNotNil,
		cxyAnd,
		cxyOr,
		cxyzAnd,
		branchDisjuncts,
	}

	suite := lattice.LawSuite[Condition]{
		Name:   "constraint.Condition",
		Domain: ConditionLattice{},
		Sample: sample,
	}
	suite.Run(t)
}
