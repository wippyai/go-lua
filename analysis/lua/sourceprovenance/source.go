package sourceprovenance

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// SourceKind classifies Lua AST provenance for one value-list slot.
type SourceKind uint8

const (
	SourceUnknown SourceKind = iota
	SourceExpression
	SourceCall
	SourceVararg
	SourceNil
)

// NoSourceIndex marks an index field that does not point at a source,
// target, or result slot.
const NoSourceIndex = -1

// SourceShape describes Lua value-list shape flags for an AST source.
type SourceShape struct {
	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// NewSourceShape creates a validated Lua value-list shape.
func NewSourceShape(final, expanded, adjusted, openTail bool) (SourceShape, bool) {
	shape := SourceShape{
		Final:    final,
		Expanded: expanded,
		Adjusted: adjusted,
		OpenTail: openTail,
	}
	return shape, shape.Valid()
}

// Valid reports whether the shape flags form a supported Lua value-list shape.
func (s SourceShape) Valid() bool {
	if s.Expanded && s.Adjusted {
		return false
	}
	if s.Expanded && !s.Final {
		return false
	}
	if s.OpenTail && (!s.Expanded || !s.Final) {
		return false
	}
	return true
}

// ASTSource describes Lua AST provenance for one value-list slot.
type ASTSource struct {
	Kind SourceKind
	Expr ast.Expr

	ExprIndex    int
	TargetIndex  int
	ResultIndex  int
	CallPoint    cfg.Point
	HasCallPoint bool

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// NewExpressionSource creates an AST-backed expression value source.
func NewExpressionSource(expr ast.Expr, exprIndex, targetIndex, resultIndex int, shape SourceShape) (ASTSource, bool) {
	source := astSourceWithShape(ASTSource{
		Kind:        SourceExpression,
		Expr:        expr,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewCallSource creates an AST-backed call result source with a resolved CFG call point.
func NewCallSource(expr ast.Expr, exprIndex, targetIndex, resultIndex int, callPoint cfg.Point, shape SourceShape) (ASTSource, bool) {
	source := astSourceWithShape(ASTSource{
		Kind:         SourceCall,
		Expr:         expr,
		ExprIndex:    exprIndex,
		TargetIndex:  targetIndex,
		ResultIndex:  resultIndex,
		CallPoint:    callPoint,
		HasCallPoint: callPoint != 0,
	}, shape)
	return source, source.Valid()
}

// NewVarargSource creates an AST-backed vararg value source.
func NewVarargSource(expr ast.Expr, exprIndex, targetIndex, resultIndex int, shape SourceShape) (ASTSource, bool) {
	source := astSourceWithShape(ASTSource{
		Kind:        SourceVararg,
		Expr:        expr,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewNilSource creates a nil-fill source for a target slot.
func NewNilSource(targetIndex int) ASTSource {
	return ASTSource{
		Kind:        SourceNil,
		ExprIndex:   NoSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: NoSourceIndex,
	}
}

// NewUnknownSource creates an explicit unknown source for a target slot.
func NewUnknownSource(targetIndex int) ASTSource {
	return ASTSource{
		Kind:        SourceUnknown,
		ExprIndex:   NoSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: NoSourceIndex,
	}
}

// Shape returns the source's value-list shape flags.
func (s ASTSource) Shape() SourceShape {
	return SourceShape{
		Final:    s.Final,
		Expanded: s.Expanded,
		Adjusted: s.Adjusted,
		OpenTail: s.OpenTail,
	}
}

// Valid reports whether the source satisfies the Lua provenance invariants.
func (s ASTSource) Valid() bool {
	if !s.Shape().Valid() {
		return false
	}
	if s.HasCallPoint != (s.CallPoint != 0) {
		return false
	}
	switch s.Kind {
	case SourceUnknown:
		return s.Expr == nil &&
			s.ExprIndex == NoSourceIndex &&
			s.ResultIndex == NoSourceIndex &&
			!s.HasCallPoint &&
			s.CallPoint == 0 &&
			!s.Final &&
			!s.Expanded &&
			!s.Adjusted &&
			!s.OpenTail
	case SourceExpression:
		return !exprNil(s.Expr) && !s.HasCallPoint && s.CallPoint == 0
	case SourceCall:
		return !exprNil(s.Expr) && s.HasCallPoint && s.CallPoint != 0 && s.ResultIndex >= 0
	case SourceVararg:
		return !exprNil(s.Expr) && !s.HasCallPoint && s.CallPoint == 0
	case SourceNil:
		return s.Expr == nil &&
			s.ExprIndex == NoSourceIndex &&
			s.ResultIndex == NoSourceIndex &&
			!s.HasCallPoint &&
			s.CallPoint == 0 &&
			!s.Final &&
			!s.Expanded &&
			!s.Adjusted &&
			!s.OpenTail
	default:
		return false
	}
}

func astSourceWithShape(source ASTSource, shape SourceShape) ASTSource {
	source.Final = shape.Final
	source.Expanded = shape.Expanded
	source.Adjusted = shape.Adjusted
	source.OpenTail = shape.OpenTail
	return source
}
