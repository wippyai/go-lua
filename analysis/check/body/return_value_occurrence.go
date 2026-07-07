package body

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ReturnValueOccurrence is one returned expression with body-owned presentation
// data and source provenance.
type ReturnValueOccurrence struct {
	Point       cfg.Point
	Index       int
	Source      sourceprovenance.ASTSource
	Lowered     factflow.ValueSource
	HasLowered  bool
	SourceLabel string
	SourceSpan  SourceSpan
	SourcePath  pathdom.Path
	HasPath     bool
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
		meta := r.returnValueMetadata(point)
		for index, expr := range fact.Exprs {
			label := returnSourceExprLabel(expr)
			span := sourceSpanFromAST(ast.SpanOf(expr))
			if index < len(meta) {
				if meta[index].Label != "" {
					label = meta[index].Label
				}
				if metaSpan := sourceSpanFromWIR(meta[index].Span); sourceSpanValid(metaSpan) {
					span = metaSpan
				}
			}
			occ := ReturnValueOccurrence{
				Point:       point,
				Index:       index,
				Source:      returnSourceAt(fact, index),
				SourceLabel: label,
				SourceSpan:  span,
			}
			if lowered, ok := loweredReturnSourceAt(r, point, index); ok {
				occ.Lowered = lowered
				occ.HasLowered = true
			}
			occ.SourcePath, occ.HasPath = r.returnSourceExprPath(expr)
			visited = true
			if !visit(occ) {
				return true
			}
		}
	}
	return visited
}

func (r *Result) returnValueMetadata(point cfg.Point) []wir.ReturnValueMeta {
	if r == nil || r.wir == nil {
		return nil
	}
	for _, inst := range r.wir.PointInstructions(point) {
		if inst.Op == wir.OpReturn {
			return r.wir.ReturnValueMeta(inst.ReturnValues)
		}
	}
	return nil
}

func loweredReturnSourceAt(r *Result, point cfg.Point, index int) (factflow.ValueSource, bool) {
	if r == nil {
		return factflow.ValueSource{}, false
	}
	sources, ok := r.ReturnValueSources(point)
	if !ok || index < 0 || index >= len(sources) {
		return factflow.ValueSource{}, false
	}
	return sources[index], true
}

func (r *Result) returnSourceExprPath(expr ast.Expr) (pathdom.Path, bool) {
	if p, ok := r.ExpressionPath(expr); ok {
		return p, true
	}
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok || inner == nil || inner == expr {
		return pathdom.Path{}, false
	}
	return r.ExpressionPath(inner)
}

func sourceSpanFromWIR(span wir.Span) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
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
