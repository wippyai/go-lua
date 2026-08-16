package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NilableAccessEvidence is one nilable receiver access observed inside an
// assignment source expression.
type NilableAccessEvidence struct {
	Label  string
	Access string
	Span   SourceSpan
}

// AssignmentNilableAccessEvidence returns nilable access evidence for expr at
// point using solved expression projections.
func (r *Result) AssignmentNilableAccessEvidence(point cfg.Point, expr ast.Expr) []NilableAccessEvidence {
	var out []NilableAccessEvidence
	chain := make([]*ast.AttrGetExpr, 0, 4)
	for {
		attr, ok := expr.(*ast.AttrGetExpr)
		if !ok || attr.Object == nil || attr.Key == nil {
			break
		}
		chain = append(chain, attr)
		expr = attr.Object
	}
	for index := len(chain) - 1; index >= 0; index-- {
		attr := chain[index]
		label := AssignmentSourceLabel(attr.Object)
		access := AssignmentAttrKeyLabel(attr)
		if label == "" || access == "" {
			continue
		}
		t, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
		nilable := typ.TypeEquals(t, typ.Nil) || typevalue.TypeIncludesNil(t)
		if !ok || t == nil || obligationTypeIsGradual(t) || typ.IsNever(t) || !nilable {
			continue
		}
		out = append(out, NilableAccessEvidence{
			Label:  label,
			Access: access,
			Span:   sourceSpanFromAST(ast.SpanOf(attr.Object)),
		})
	}
	return out
}

func obligationTypeIsGradual(t typ.Type) bool {
	return typ.IsAny(t) || typ.IsUnknown(t)
}
