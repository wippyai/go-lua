package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// OptionalAssignmentTargetOccurrence is a write through a container whose
// solved type still includes nil.
type OptionalAssignmentTargetOccurrence struct {
	Point          cfg.Point
	ContainerLabel string
	TargetLabel    string
	ContainerType  typ.Type
	ContainerSpan  SourceSpan
	TargetSpan     SourceSpan
}

// ForEachOptionalAssignmentTargetOccurrence visits writes through optional
// containers in deterministic CFG order.
func (r *Result) ForEachOptionalAssignmentTargetOccurrence(visit func(OrdinaryAssignmentFact, OptionalAssignmentTargetOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.OrdinaryAssignment(point)
		if !ok {
			continue
		}
		occ, ok := r.optionalAssignmentTargetOccurrence(point, fact)
		if !ok {
			continue
		}
		visited = true
		if !visit(fact, occ) {
			return true
		}
	}
	return visited
}

func (r *Result) optionalAssignmentTargetOccurrence(point cfg.Point, fact OrdinaryAssignmentFact) (OptionalAssignmentTargetOccurrence, bool) {
	container, ok := optionalAssignmentContainerExpr(fact.Target)
	if !ok || container == nil {
		return OptionalAssignmentTargetOccurrence{}, false
	}
	containerType, ok := r.ExpressionTypeBeforeBoundary(point, container)
	if !ok || containerType == nil ||
		typ.IsAny(containerType) ||
		typ.IsUnknown(containerType) ||
		typ.IsNever(containerType) ||
		!typevalue.ProjectionHasNil(containerType) {
		return OptionalAssignmentTargetOccurrence{}, false
	}
	return OptionalAssignmentTargetOccurrence{
		Point:          point,
		ContainerLabel: ExpressionLabel(container),
		TargetLabel:    ExpressionLabel(fact.Target),
		ContainerType:  containerType,
		ContainerSpan:  sourceSpanFromAST(ast.SpanOf(container)),
		TargetSpan:     sourceSpanFromAST(ast.SpanOf(fact.Target)),
	}, true
}

func optionalAssignmentContainerExpr(target ast.Expr) (ast.Expr, bool) {
	attr, ok := assignmentTargetAttrExpr(target)
	if !ok || attr.Object == nil {
		return nil, false
	}
	object := attr.Object
	for {
		next, ok := object.(*ast.AttrGetExpr)
		if !ok || next.KeySyntax != ast.AttrKeyIndex || next.Object == nil {
			return object, true
		}
		object = next.Object
	}
}

func assignmentTargetAttrExpr(expr ast.Expr) (*ast.AttrGetExpr, bool) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		return e, true
	case *ast.CastExpr:
		return assignmentTargetAttrExpr(e.Expr)
	default:
		return nil, false
	}
}
