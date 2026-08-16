package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/castsem"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NumericForOperandOccurrence is the body-owned projection of one numeric-for
// init, limit, or explicit step operand.
type NumericForOperandOccurrence struct {
	Point               cfg.Point
	Role                string
	OperandLabel        string
	OperandKey          string
	TypeWithPresence    typ.Type
	OperandSpan         SourceSpan
	ExplicitTopLikeCast bool
}

// ForEachNumericForOperandOccurrence visits numeric-for init, limit, and
// explicit step operands in deterministic RPO order.
func (r *Result) ForEachNumericForOperandOccurrence(visit func(NumericForOperandOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.NumericFor(point)
		if !ok || fact.Role != NumericForRoleInit {
			continue
		}
		operands := []struct {
			expr ast.Expr
			role string
		}{
			{expr: fact.Init, role: "initial value"},
			{expr: fact.Limit, role: "limit"},
		}
		if fact.Step != nil {
			operands = append(operands, struct {
				expr ast.Expr
				role string
			}{expr: fact.Step, role: "step"})
		}
		for _, operand := range operands {
			item, ok := r.numericForOperandOccurrence(point, operand.expr, operand.role)
			if !ok {
				continue
			}
			visited = true
			if !visit(item) {
				return true
			}
		}
	}
	return visited
}

func (r *Result) numericForOperandOccurrence(point cfg.Point, expr ast.Expr, role string) (NumericForOperandOccurrence, bool) {
	if expr == nil {
		return NumericForOperandOccurrence{}, false
	}
	explicitTopLikeCast := numericForOperandExplicitTopLikeCast(expr)
	typeExpr := expr
	if explicitTopLikeCast {
		if cast, ok := expr.(*ast.CastExpr); ok && cast != nil && cast.Expr != nil {
			typeExpr = cast.Expr
		}
	}
	t, ok := r.ExpressionTypeBeforeBoundary(point, typeExpr)
	if !ok || t == nil {
		return NumericForOperandOccurrence{}, false
	}
	return NumericForOperandOccurrence{
		Point:               point,
		Role:                role,
		OperandLabel:        ExpressionLabel(expr),
		OperandKey:          role + ":" + expressionKey(point, expr),
		TypeWithPresence:    t,
		OperandSpan:         sourceSpanFromAST(ast.SpanOf(expr)),
		ExplicitTopLikeCast: explicitTopLikeCast,
	}, true
}

func numericForOperandExplicitTopLikeCast(expr ast.Expr) bool {
	cast, ok := expr.(*ast.CastExpr)
	if !ok || cast == nil || cast.Type == nil {
		return false
	}
	primitive, ok := cast.Type.(*ast.PrimitiveTypeExpr)
	if !ok || primitive == nil {
		return false
	}
	return castsem.IsAnyTarget(primitive.Name) || castsem.IsUnknownTarget(primitive.Name)
}
