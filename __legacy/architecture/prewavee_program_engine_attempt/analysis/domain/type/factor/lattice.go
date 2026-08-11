package typefactor

import (
	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/semantic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/link"
)

func config(source *link.Link, table *typedomain.Table, universe *origin.Universe) (engine.FactorConfig[link.Value, carrier.Value], bool) {
	if source == nil || table == nil || !table.Sealed() || universe == nil {
		return engine.FactorConfig[link.Value, carrier.Value]{}, false
	}
	bottom, ok := carrier.Bottom(table, universe)
	if !ok {
		return engine.FactorConfig[link.Value, carrier.Value]{}, false
	}
	top, ok := carrier.Top(table, universe)
	if !ok {
		return engine.FactorConfig[link.Value, carrier.Value]{}, false
	}
	return engine.FactorConfig[link.Value, carrier.Value]{
		Keys:     engine.KeySpace{End: uint64(source.ValueCount())},
		Semantic: semantic.Factor(source),
		Lattice: lattice.Lattice[carrier.Value]{
			Bottom:   func() carrier.Value { return bottom },
			Top:      func() carrier.Value { return top },
			Equal:    carrier.Equal,
			LessOrEq: carrier.LessEqual,
			Join: func(left, right carrier.Value) carrier.Value {
				return mustJoin(left, right)
			},
			Widen: func(previous, next carrier.Value) carrier.Value {
				return mustWiden(previous, next)
			},
		},
		Default:     bottom,
		Fingerprint: carrier.Value.Hash,
		// There is deliberately no Narrow.  The carrier has a proven finite
		// Mu widening but no decreasing transfer law.
		WidenRank: engine.Measure[link.Value, carrier.Value]{
			Width: 4,
			At: func(_ link.Value, value carrier.Value, component int) uint64 {
				rank, ok := value.Rank(component)
				if !ok {
					panic("typedomain: invalid carrier reached Type Factor rank")
				}
				return rank
			},
		},
	}, true
}

func mustJoin(left, right carrier.Value) carrier.Value {
	result, ok := carrier.Join(left, right)
	if !ok {
		panic("typedomain: foreign carrier reached Type Factor Join")
	}
	return result
}

func mustWiden(previous, next carrier.Value) carrier.Value {
	result, ok := carrier.Widen(previous, next)
	if !ok {
		panic("typedomain: foreign carrier reached Type Factor Widen")
	}
	return result
}
