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
	guards  map[cfg.Point]ShortCircuitGuard
	evals   map[cfg.Point]ExpressionEvaluation
	regions map[*ast.LogicalOpExpr]ShortCircuitRegion
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

// ShortCircuitRegion is the source-authored ownership record for one
// value-position logical expression. OwnedRHSPoints is the complete CFG region
// whose execution is gated by the logical's RHS edge. It intentionally owns
// every point in that region, rather than only today's known effect opcodes, so
// adding a new effect axis cannot silently escape the structural boundary.
//
// Values are valid only when Complete reports true. Incomplete records remain
// internal to lowering and must not be published as analysis facts.
type ShortCircuitRegion struct {
	Guard          cfg.Point
	TrueTarget     cfg.Point
	FalseTarget    cfg.Point
	Join           cfg.Point
	RHSOnTrue      bool
	OwnedRHSPoints []cfg.Point
	complete       bool
}

// Complete reports whether lowering recorded the entire structural boundary.
func (r ShortCircuitRegion) Complete() bool { return r.complete }

// Region returns a defensive copy of the exact region for expr.
func (s ShortCircuits) Region(expr *ast.LogicalOpExpr) (ShortCircuitRegion, bool) {
	region, ok := s.regions[expr]
	if !ok {
		return ShortCircuitRegion{}, false
	}
	region.OwnedRHSPoints = append([]cfg.Point(nil), region.OwnedRHSPoints...)
	return region, true
}

func (s *ShortCircuits) setRegion(expr *ast.LogicalOpExpr, region ShortCircuitRegion) {
	if s == nil || expr == nil {
		return
	}
	if s.regions == nil {
		s.regions = make(map[*ast.LogicalOpExpr]ShortCircuitRegion)
	}
	region.OwnedRHSPoints = append([]cfg.Point(nil), region.OwnedRHSPoints...)
	s.regions[expr] = region
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
