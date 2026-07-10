package solver

import (
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric/diff"
)

// diffBackend is the difference-logic Solver backend. It wraps a
// diff.DifferenceGraph and represents the constraint kinds difference logic can
// express as AddLE edges. Kinds it cannot represent are ignored on Assert and
// yield Unknown on Entails.
//
// It is a partial decision procedure: Entails never returns decision.Invalid.
type diffBackend struct {
	g *diff.DifferenceGraph
}

// NewDiffBackend creates a fresh difference-logic Solver.
func NewDiffBackend() Solver {
	return &diffBackend{g: diff.NewDifferenceGraph()}
}

func (b *diffBackend) Assert(c numeric.NumericConstraint) {
	switch v := c.(type) {
	case numeric.Le:
		b.g.AddLE(string(v.X), string(v.Y), v.C)
	case numeric.Lt:
		b.g.AddLT(string(v.X), string(v.Y))
	case numeric.Ge:
		b.g.AddGE(string(v.X), string(v.Y))
	case numeric.Gt:
		b.g.AddGT(string(v.X), string(v.Y))
	case numeric.Eq:
		b.g.AddEQ(string(v.X), string(v.Y))
	}
}

func (b *diffBackend) Entails(goal numeric.NumericConstraint) decision.Result {
	le, ok := goal.(numeric.Le)
	if !ok {
		return decision.Unknown
	}
	bound, ok := b.g.GetBound(string(le.X), string(le.Y))
	if ok && bound <= le.C {
		return decision.Valid
	}
	return decision.Unknown
}
