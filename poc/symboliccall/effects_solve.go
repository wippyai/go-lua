package symboliccall

import (
	"context"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

type EffectEquation struct {
	ID       FunctionID
	Transfer func(read func(FunctionID) EffectTransformer) EffectTransformer
}

// SolveEffects reuses the production WTO implementation. Recursive effect-row
// growth has a hard correlated-row budget and falls back atomically at the
// widening point rather than inventing return/effect combinations.
func SolveEffects(
	ctx context.Context,
	reg *axis.Registry,
	equations []EffectEquation,
	influences func(FunctionID) []FunctionID,
	maxRows int,
	stats *solve.Stats,
) (map[FunctionID]EffectTransformer, error) {
	cells := make([]FunctionID, len(equations))
	byID := make(map[FunctionID]EffectEquation, len(equations))
	for i, equation := range equations {
		cells[i] = equation.ID
		byID[equation.ID] = equation
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i] < cells[j] })
	cyclic := recursiveCells(cells, influences)
	domain := lattice.Lattice[EffectTransformer]{
		Bottom: func() EffectTransformer { return EffectTransformer{} },
		Top: func() EffectTransformer {
			return EffectTransformer{reg: reg, valid: true, contextual: "top"}
		},
		Equal:    EqualEffects,
		LessOrEq: LessOrEqEffects,
		Join:     JoinEffects,
		Widen: func(prev, next EffectTransformer) EffectTransformer {
			return WidenEffects(prev, next, maxRows)
		},
	}
	system := solve.EquationSystem[FunctionID, EffectTransformer]{
		Lattice: domain,
		Cells:   cells,
		Transfer: func(id FunctionID, read func(FunctionID) EffectTransformer, emit func(FunctionID, EffectTransformer)) {
			equation, ok := byID[id]
			if !ok || equation.Transfer == nil {
				emit(id, EffectTransformer{reg: reg, valid: true, contextual: "missing effect equation"})
				return
			}
			emit(id, equation.Transfer(read))
		},
		WidenAt: func(id FunctionID) bool { return cyclic[id] },
		WidenDelay: func(FunctionID) int {
			return 2
		},
		Stats: stats,
	}
	plan := solve.NewWTOPlan(cells, func(id FunctionID) []FunctionID {
		out := append([]FunctionID(nil), influences(id)...)
		return append(out, id)
	})
	result, _, err := solve.SolveWTOContextWithVersions(ctx, system, plan)
	return result, err
}
