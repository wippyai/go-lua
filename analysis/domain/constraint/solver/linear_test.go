package solver

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
)

func assertAll(s Solver, cs ...numeric.NumericConstraint) {
	for _, c := range cs {
		s.Assert(c)
	}
}

func TestLinearBackendSumProvesIndexBound(t *testing.T) {
	// i + j <= n and j >= 0  =>  i <= n.
	b := NewLinearBackend()
	assertAll(b,
		numeric.NewSumLe(key("i"), key("j"), key("n"), 0),
		numeric.GeConst{X: key("j"), C: 0},
	)

	if got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: 0}); got != decision.Valid {
		t.Fatalf("expected Valid for i <= n, got %s", got)
	}
}

func TestLinearBackendSubsumesDifference(t *testing.T) {
	// i < j and j <= n  =>  i <= n - 1.
	b := NewLinearBackend()
	assertAll(b,
		numeric.Le{X: key("i"), Y: key("j"), C: -1},
		numeric.Le{X: key("j"), Y: key("n"), C: 0},
	)

	if got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: -1}); got != decision.Valid {
		t.Fatalf("expected Valid for i - n <= -1, got %s", got)
	}
}

func TestLinearBackendUnknownWithoutFloor(t *testing.T) {
	// i + j <= n alone does not bound i without j >= 0.
	b := NewLinearBackend()
	b.Assert(numeric.NewSumLe(key("i"), key("j"), key("n"), 0))

	if got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: 0}); got != decision.Unknown {
		t.Fatalf("expected Unknown without a floor on j, got %s", got)
	}
}

func TestLinearBackendUnknownTighterThanProven(t *testing.T) {
	// i + j <= n and j >= 0 prove i <= n but not the strictly tighter i <= n-1.
	b := NewLinearBackend()
	assertAll(b,
		numeric.NewSumLe(key("i"), key("j"), key("n"), 0),
		numeric.GeConst{X: key("j"), C: 0},
	)

	if got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: -1}); got != decision.Unknown {
		t.Fatalf("expected Unknown for unproven tighter bound, got %s", got)
	}
}

func TestLinearBackendNeverInvalid(t *testing.T) {
	cases := []struct {
		name     string
		asserted []numeric.NumericConstraint
		goal     numeric.Le
	}{
		{
			name: "sum-with-floor",
			asserted: []numeric.NumericConstraint{
				numeric.NewSumLe(key("i"), key("j"), key("n"), 0),
				numeric.GeConst{X: key("j"), C: 0},
			},
			goal: numeric.Le{X: key("i"), Y: key("n"), C: 0},
		},
		{
			name: "sum-without-floor",
			asserted: []numeric.NumericConstraint{
				numeric.NewSumLe(key("i"), key("j"), key("n"), 0),
			},
			goal: numeric.Le{X: key("i"), Y: key("n"), C: 0},
		},
		{
			name: "tighter-than-proven",
			asserted: []numeric.NumericConstraint{
				numeric.NewSumLe(key("i"), key("j"), key("n"), 0),
				numeric.GeConst{X: key("j"), C: 0},
			},
			goal: numeric.Le{X: key("i"), Y: key("n"), C: -1},
		},
		{
			name: "difference-chain",
			asserted: []numeric.NumericConstraint{
				numeric.Le{X: key("i"), Y: key("j"), C: -1},
				numeric.Le{X: key("j"), Y: key("n"), C: 0},
			},
			goal: numeric.Le{X: key("i"), Y: key("n"), C: -1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewLinearBackend()
			assertAll(b, tc.asserted...)
			if got := b.Entails(tc.goal); got == decision.Invalid {
				t.Fatalf("linear backend must never return Invalid, got %s", got)
			}
		})
	}
}

func TestPortfolioSumEntailment(t *testing.T) {
	asserted := []numeric.NumericConstraint{
		numeric.NewSumLe(key("i"), key("j"), key("n"), 0),
		numeric.GeConst{X: key("j"), C: 0},
	}
	goal := numeric.Le{X: key("i"), Y: key("n"), C: 0}

	if got := DefaultPortfolio().Entails(asserted, goal); got != decision.Valid {
		t.Fatalf("portfolio should prove i <= n via the linear backend, got %s", got)
	}
}
