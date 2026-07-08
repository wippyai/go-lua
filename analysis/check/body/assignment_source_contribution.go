package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
		fact, ok := r.RootAssignment(candidate)
		if !ok ||
			fact.Kind() != factflow.RootAssignmentOrdinaryRootWrite ||
			fact.TargetSymbol() != readPath.Symbol ||
			len(fact.TargetPathRef().Segments) != 0 {
			continue
		}
		literal, ok := r.ObjectLiteralViewForSource(fact.Source())
		if !ok {
			continue
		}
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			if assignmentObjectEntryFieldName(entry) != field {
				return true
			}
			t, ok := r.assignmentSourceContributionEntryType(candidate, entry)
			if !ok {
				return true
			}
			span := sourceSpanFromFactflow(entry.ValueSpan())
			if literalSpan, ok := literal.Span(); ok {
				span = sourceSpanFromFactflow(literalSpan)
			}
			out = append(out, AssignmentSourceContribution{
				RootLabel: rootLabel,
				ReadLabel: readLabel,
				Type:      t,
				Span:      span,
			})
			return true
		})
	}
	return out
}

func assignmentObjectEntryFieldName(entry factflow.ObjectEntryView) string {
	if entry.SuffixSegmentCount() != 1 {
		return ""
	}
	seg, ok := entry.SuffixSegmentAt(0)
	if !ok {
		return ""
	}
	return assignmentStaticMemberSegmentName(seg)
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

func (r *Result) assignmentSourceContributionEntryType(point cfg.Point, entry factflow.ObjectEntryView) (typ.Type, bool) {
	value, ok := r.SourceValueAtBoundary(point, entry.Source())
	if !ok {
		return nil, false
	}
	return r.ValueType(value)
}
