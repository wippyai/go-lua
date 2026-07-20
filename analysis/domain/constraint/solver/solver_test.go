package solver

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

func key(s string) pathdom.PathKey { return pathdom.PathKey(s) }

func TestDiffBackendEntailsTransitiveBound(t *testing.T) {
	b := NewDiffBackend()
	// i < j and j <= len  =>  i - len <= -1, so i + 1 <= len.
	b.Assert(numeric.Lt{X: key("i"), Y: key("j")})
	b.Assert(numeric.Le{X: key("j"), Y: key("len"), C: 0})

	if got := b.Entails(numeric.Le{X: key("i"), Y: key("len"), C: -1}); got != decision.Valid {
		t.Fatalf("expected Valid for i - len <= -1, got %s", got)
	}
	if got := b.Entails(numeric.Le{X: key("i"), Y: key("len"), C: -2}); got != decision.Unknown {
		t.Fatalf("expected Unknown for unproven tighter bound, got %s", got)
	}
}

func TestDiffBackendUnknownOutsideTheory(t *testing.T) {
	b := NewDiffBackend()
	b.Assert(numeric.Le{X: key("i"), Y: key("j"), C: 0})

	if got := b.Entails(numeric.ModEq{X: key("i"), M: 2, R: 0}); got != decision.Unknown {
		t.Fatalf("expected Unknown for non-Le goal, got %s", got)
	}
	if got := b.Entails(numeric.Le{X: key("p"), Y: key("q"), C: 0}); got != decision.Unknown {
		t.Fatalf("expected Unknown for unconstrained variables, got %s", got)
	}
}

func TestPortfolioEntailsByRefutation(t *testing.T) {
	asserted := []numeric.NumericConstraint{
		numeric.Lt{X: key("i"), Y: key("j")},
		numeric.Le{X: key("j"), Y: key("len"), C: 0},
	}
	goal := numeric.Le{X: key("i"), Y: key("len"), C: -1}

	if got := DefaultPortfolio().Entails(asserted, goal); got != decision.Valid {
		t.Fatalf("expected Valid, got %s", got)
	}

	unprovable := numeric.Le{X: key("i"), Y: key("len"), C: -5}
	if got := DefaultPortfolio().Entails(asserted, unprovable); got != decision.Unknown {
		t.Fatalf("expected Unknown for unprovable goal, got %s", got)
	}
}

func TestPortfolioEmptyAssertedUnknown(t *testing.T) {
	goal := numeric.Le{X: key("i"), Y: key("len"), C: 0}
	if got := DefaultPortfolio().Entails(nil, goal); got != decision.Unknown {
		t.Fatalf("expected Unknown with no asserted constraints, got %s", got)
	}
}

func TestPortfolioStopsAfterCheaperProof(t *testing.T) {
	secondConstructed := false
	portfolio := NewPortfolio(
		func() Solver { return fixedResultSolver{result: decision.Valid} },
		func() Solver {
			secondConstructed = true
			return fixedResultSolver{result: decision.Unknown}
		},
	)
	if got := portfolio.Entails(nil, numeric.Le{X: key("x"), Y: key("y")}); got != decision.Valid {
		t.Fatalf("portfolio result = %s, want Valid", got)
	}
	if secondConstructed {
		t.Fatal("portfolio constructed the expensive backend after a decisive proof")
	}
}

type fixedResultSolver struct {
	result decision.Result
}

func (fixedResultSolver) Assert(numeric.NumericConstraint) {}

func (s fixedResultSolver) Entails(numeric.NumericConstraint) decision.Result {
	return s.result
}
