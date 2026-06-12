package sourceprovenance

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ASTSource describes Lua AST provenance for one value-list slot.
type ASTSource struct {
	Kind factflow.ValueSourceKind
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
func NewExpressionSource(expr ast.Expr, exprIndex, targetIndex, resultIndex int, shape factflow.ValueSourceShape) (ASTSource, bool) {
	source := astSourceWithShape(ASTSource{
		Kind:        factflow.ValueSourceExpression,
		Expr:        expr,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewCallSource creates an AST-backed call result source with a resolved CFG call point.
func NewCallSource(expr ast.Expr, exprIndex, targetIndex, resultIndex int, callPoint cfg.Point, shape factflow.ValueSourceShape) (ASTSource, bool) {
	source := astSourceWithShape(ASTSource{
		Kind:         factflow.ValueSourceCall,
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
func NewVarargSource(expr ast.Expr, exprIndex, targetIndex, resultIndex int, shape factflow.ValueSourceShape) (ASTSource, bool) {
	source := astSourceWithShape(ASTSource{
		Kind:        factflow.ValueSourceVararg,
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
		Kind:        factflow.ValueSourceNil,
		ExprIndex:   factflow.NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: factflow.NoValueSourceIndex,
	}
}

// NewUnknownSource creates an explicit unknown source for a target slot.
func NewUnknownSource(targetIndex int) ASTSource {
	return ASTSource{
		Kind:        factflow.ValueSourceUnknown,
		ExprIndex:   factflow.NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: factflow.NoValueSourceIndex,
	}
}

// Shape returns the source's value-list shape flags.
func (s ASTSource) Shape() factflow.ValueSourceShape {
	return factflow.ValueSourceShape{
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
	case factflow.ValueSourceUnknown:
		return s.Expr == nil &&
			s.ExprIndex == factflow.NoValueSourceIndex &&
			s.ResultIndex == factflow.NoValueSourceIndex &&
			!s.HasCallPoint &&
			s.CallPoint == 0 &&
			!s.Final &&
			!s.Expanded &&
			!s.Adjusted &&
			!s.OpenTail
	case factflow.ValueSourceExpression:
		return s.Expr != nil && !s.HasCallPoint && s.CallPoint == 0
	case factflow.ValueSourceCall:
		return s.HasCallPoint && s.CallPoint != 0 && s.ResultIndex >= 0
	case factflow.ValueSourceVararg:
		return s.Expr != nil && !s.HasCallPoint && s.CallPoint == 0
	case factflow.ValueSourceNil:
		return s.Expr == nil &&
			s.ExprIndex == factflow.NoValueSourceIndex &&
			s.ResultIndex == factflow.NoValueSourceIndex &&
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

func astSourceWithShape(source ASTSource, shape factflow.ValueSourceShape) ASTSource {
	source.Final = shape.Final
	source.Expanded = shape.Expanded
	source.Adjusted = shape.Adjusted
	source.OpenTail = shape.OpenTail
	return source
}
