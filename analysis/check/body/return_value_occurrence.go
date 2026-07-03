package body

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ReturnValueOccurrence is one returned expression with body-owned presentation
// data and source provenance.
type ReturnValueOccurrence struct {
	Point       cfg.Point
	Index       int
	Source      sourceprovenance.ASTSource
	SourceLabel string
	SourceSpan  SourceSpan
}

// ForEachReturnValueOccurrence visits returned expressions from reachable
// return points in deterministic order.
func (r *Result) ForEachReturnValueOccurrence(visit func(ReturnValueOccurrence) bool) bool {
	if r == nil || visit == nil {
		return false
	}
	visited := false
	for _, point := range r.ReturnPoints() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.ReturnFact(point)
		if !ok {
			continue
		}
		for index, expr := range fact.Exprs {
			occ := ReturnValueOccurrence{
				Point:       point,
				Index:       index,
				Source:      returnSourceAt(fact, index),
				SourceLabel: returnSourceExprLabel(expr),
				SourceSpan:  sourceSpanFromAST(ast.SpanOf(expr)),
			}
			visited = true
			if !visit(occ) {
				return true
			}
		}
	}
	return visited
}

func returnSourceAt(fact ReturnFact, index int) sourceprovenance.ASTSource {
	if index >= 0 && index < len(fact.Sources) {
		return fact.Sources[index]
	}
	return sourceprovenance.NewUnknownSource(sourceprovenance.NoSourceIndex)
}

func returnSourceExprLabel(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StringExpr:
		return strconv.Quote(e.Value)
	case *ast.NumberExpr:
		return e.Value
	case *ast.TrueExpr:
		return "true"
	case *ast.FalseExpr:
		return "false"
	case *ast.NilExpr:
		return "nil"
	default:
		return ExpressionLabel(expr)
	}
}
