package solver

import (
	"strconv"
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

func TestLinearBackendOverflowStaysSound(t *testing.T) {
	// Bounds near the int64 extremes force the Fourier-Motzkin combination to
	// overflow; the backend must abandon the proof and return Unknown rather than
	// panic or report a spurious contradiction.
	const big = int64(1) << 62
	b := NewLinearBackend()
	assertAll(b,
		numeric.NewSumLe(key("i"), key("j"), key("n"), big),
		numeric.GeConst{X: key("j"), C: -big},
	)

	got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: big})
	if got == decision.Invalid {
		t.Fatalf("overflow path must never return Invalid, got %s", got)
	}
}

func TestLinearBackendDeepTransitiveChain(t *testing.T) {
	// A long difference chain x0 < x1 < ... < xk <= n proves x0 <= n - k.
	b := NewLinearBackend()
	const k = 12
	for d := range k {
		b.Assert(numeric.Le{X: key(chainVar(d)), Y: key(chainVar(d + 1)), C: -1})
	}
	b.Assert(numeric.Le{X: key(chainVar(k)), Y: key("n"), C: 0})

	if got := b.Entails(numeric.Le{X: key(chainVar(0)), Y: key("n"), C: -k}); got != decision.Valid {
		t.Fatalf("expected Valid for x0 <= n - %d, got %s", k, got)
	}
	if got := b.Entails(numeric.Le{X: key(chainVar(0)), Y: key("n"), C: -(k + 1)}); got != decision.Unknown {
		t.Fatalf("expected Unknown for the unproven tighter bound, got %s", got)
	}
}

func chainVar(i int) string {
	return "x" + strconv.Itoa(i)
}

func TestLinearBackendScaledGoalProvesScaledBound(t *testing.T) {
	// 2i <= n and i >= 0 prove the scaled goal 2i <= n.
	b := NewLinearBackend()
	assertAll(b,
		numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0),
		numeric.GeConst{X: key("i"), C: 0},
	)
	goal := numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0)
	if got := b.Entails(goal); got != decision.Valid {
		t.Fatalf("expected Valid for 2i <= n, got %s", got)
	}
}

func TestLinearBackendScaledBoundProvesUnitBound(t *testing.T) {
	// 2i <= n and i >= 0 prove i <= n, since i <= 2i <= n.
	b := NewLinearBackend()
	assertAll(b,
		numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0),
		numeric.GeConst{X: key("i"), C: 0},
	)
	if got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: 0}); got != decision.Valid {
		t.Fatalf("expected Valid for i <= n, got %s", got)
	}
}

func TestLinearBackendScaledBoundUnknownWithoutFloor(t *testing.T) {
	// 2i <= n alone does not bound i without i >= 0.
	b := NewLinearBackend()
	b.Assert(numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0))
	if got := b.Entails(numeric.Le{X: key("i"), Y: key("n"), C: 0}); got != decision.Unknown {
		t.Fatalf("expected Unknown without a floor on i, got %s", got)
	}
}

func TestLinearBackendScaledNeverInvalid(t *testing.T) {
	const big = int64(1) << 62
	cases := []struct {
		name     string
		asserted []numeric.NumericConstraint
		goal     numeric.NumericConstraint
	}{
		{
			name: "scaled-with-floor",
			asserted: []numeric.NumericConstraint{
				numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0),
				numeric.GeConst{X: key("i"), C: 0},
			},
			goal: numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0),
		},
		{
			name: "scaled-without-floor",
			asserted: []numeric.NumericConstraint{
				numeric.NewScaledLe(2, key("i"), 0, "", key("n"), 0),
			},
			goal: numeric.Le{X: key("i"), Y: key("n"), C: 0},
		},
		{
			name: "huge-coefficient-overflow",
			asserted: []numeric.NumericConstraint{
				numeric.NewScaledLe(big, key("i"), 0, "", key("n"), 0),
				numeric.GeConst{X: key("i"), C: -big},
			},
			goal: numeric.NewScaledLe(big, key("i"), 0, "", key("n"), big),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewLinearBackend()
			assertAll(b, tc.asserted...)
			if got := b.Entails(tc.goal); got == decision.Invalid {
				t.Fatalf("scaled backend must never return Invalid, got %s", got)
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
