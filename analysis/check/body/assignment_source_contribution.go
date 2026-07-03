package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// AssignmentSourceContribution records a prior object-literal write that
// contributes a member type to an assignment source read.
type AssignmentSourceContribution struct {
	RootLabel string
	ReadLabel string
	Type      typ.Type
	Span      SourceSpan
}

// AssignmentSourceContributions returns prior object-literal writes that
// contributed the static member read used as expr.
func (r *Result) AssignmentSourceContributions(point cfg.Point, expr ast.Expr) []AssignmentSourceContribution {
	if r == nil || expr == nil || r.Graph() == nil {
		return nil
	}
	readPath, ok := r.ExpressionPath(expr)
	if !ok || readPath.Symbol == 0 || len(readPath.Segments) != 1 {
		return nil
	}
	field := assignmentStaticMemberSegmentName(readPath.Segments[0])
	if field == "" {
		return nil
	}
	rootLabel := readPath.Root
	if rootLabel == "" {
		rootLabel = assignmentSourceRootLabel(expr)
	}
	readLabel := AssignmentSourceLabel(expr)
	var out []AssignmentSourceContribution
	for _, candidate := range r.Graph().RPO() {
		if candidate == point {
			break
		}
		fact, ok := r.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != readPath.Symbol {
			continue
		}
		if fact.HasPath && len(fact.Path.Segments) != 0 {
			continue
		}
		table, ok := sourceprovenance.ProofInner(fact.Value)
		if !ok {
			continue
		}
		lit, ok := table.(*ast.TableExpr)
		if !ok {
			continue
		}
		for _, entry := range lit.Fields {
			if entry == nil || ast.KeyName(entry.Key) != field || entry.Value == nil {
				continue
			}
			t, ok := r.assignmentExpressionValueTypeAtBoundary(candidate, entry.Value)
			if !ok {
				continue
			}
			out = append(out, AssignmentSourceContribution{
				RootLabel: rootLabel,
				ReadLabel: readLabel,
				Type:      t,
				Span:      sourceSpanFromAST(ast.SpanOf(fact.Value)),
			})
		}
	}
	return out
}

func assignmentStaticMemberSegmentName(seg segment.Segment) string {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name
	default:
		return ""
	}
}

func assignmentSourceRootLabel(expr ast.Expr) string {
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr.Object != nil {
		return AssignmentSourceLabel(attr.Object)
	}
	return AssignmentSourceLabel(expr)
}

func (r *Result) assignmentExpressionValueTypeAtBoundary(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	value, ok := r.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return nil, false
	}
	return proof.New(r.registry, r.typeValues).ValueType(value)
}
