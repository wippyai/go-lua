package cfgbuild

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ShortCircuits records the CFG topology points synthesized for value-position
// logical operators whose right operand must be evaluated on only one branch.
// It is structural cfgbuild output, not a semantic fact lane.
type ShortCircuits struct {
	guards map[cfg.Point]ShortCircuitGuard
	evals  map[cfg.Point]ExpressionEvaluation
}

// ShortCircuitGuard records the guard branch point for a short-circuit logical.
type ShortCircuitGuard struct {
	Condition ast.Expr
}

// ExpressionEvaluation records a structural evaluation point for a right-hand
// expression with no call point of its own.
type ExpressionEvaluation struct {
	Expr ast.Expr
}

func (s ShortCircuits) Guard(point cfg.Point) (ShortCircuitGuard, bool) {
	fact, ok := s.guards[point]
	return fact, ok
}

func (s *ShortCircuits) SetGuard(point cfg.Point, fact ShortCircuitGuard) {
	if s.guards == nil {
		s.guards = make(map[cfg.Point]ShortCircuitGuard)
	}
	s.guards[point] = fact
}

func (s ShortCircuits) Evaluation(point cfg.Point) (ExpressionEvaluation, bool) {
	fact, ok := s.evals[point]
	return fact, ok
}

func (s *ShortCircuits) SetEvaluation(point cfg.Point, fact ExpressionEvaluation) {
	if s.evals == nil {
		s.evals = make(map[cfg.Point]ExpressionEvaluation)
	}
	s.evals[point] = fact
}

// GuardPoints returns guard points in deterministic order.
func (s ShortCircuits) GuardPoints() []cfg.Point {
	if len(s.guards) == 0 {
		return nil
	}
	points := make([]cfg.Point, 0, len(s.guards))
	for point := range s.guards {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	return points
}
