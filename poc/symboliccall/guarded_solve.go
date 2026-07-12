package symboliccall

import (
	"context"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// GuardedEquation is one transformer equation. Transfer must be monotone.
type GuardedEquation struct {
	ID       FunctionID
	Transfer func(read func(FunctionID) GuardedTransformer) GuardedTransformer
}

// SolveGuarded runs guarded transformer equations through the production WTO
// solver. influences is the semantic callee-to-caller graph; solver-owned self
// emission edges are added separately and do not make acyclic cells recursive.
func SolveGuarded(
	ctx context.Context,
	reg *axis.Registry,
	equations []GuardedEquation,
	influences func(FunctionID) []FunctionID,
	limits GuardedLimits,
	stats *solve.Stats,
) (map[FunctionID]GuardedTransformer, error) {
	cells := make([]FunctionID, len(equations))
	byID := make(map[FunctionID]GuardedEquation, len(equations))
	for i, equation := range equations {
		cells[i] = equation.ID
		byID[equation.ID] = equation
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i] < cells[j] })
	cyclic := recursiveCells(cells, influences)
	domain := lattice.Lattice[GuardedTransformer]{
		Bottom: func() GuardedTransformer { return GuardedTransformer{} },
		Top: func() GuardedTransformer {
			return GuardedTransformer{reg: reg, valid: true, contextual: "top"}
		},
		Equal:    EqualGuarded,
		LessOrEq: LessOrEqGuarded,
		Join:     JoinGuarded,
		Widen: func(prev, next GuardedTransformer) GuardedTransformer {
			return WidenGuarded(prev, next, limits)
		},
	}
	system := solve.EquationSystem[FunctionID, GuardedTransformer]{
		Lattice: domain,
		Cells:   cells,
		Transfer: func(id FunctionID, read func(FunctionID) GuardedTransformer, emit func(FunctionID, GuardedTransformer)) {
			equation, ok := byID[id]
			if !ok || equation.Transfer == nil {
				emit(id, GuardedTransformer{reg: reg, valid: true, contextual: "missing guarded equation"})
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
		out = append(out, id) // every equation emits its own approximation
		return out
	})
	result, _, err := solve.SolveWTOContextWithVersions(ctx, system, plan)
	return result, err
}

func recursiveCells(cells []FunctionID, successors func(FunctionID) []FunctionID) map[FunctionID]bool {
	out := make(map[FunctionID]bool)
	for _, start := range cells {
		seen := make(map[FunctionID]bool)
		var reaches func(FunctionID) bool
		reaches = func(current FunctionID) bool {
			for _, next := range successors(current) {
				if next == start {
					return true
				}
				if !seen[next] {
					seen[next] = true
					if reaches(next) {
						return true
					}
				}
			}
			return false
		}
		seen[start] = true
		out[start] = reaches(start)
	}
	return out
}
