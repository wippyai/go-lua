package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

type sourceExprRefKey struct {
	expr         ast.Expr
	kind         factflow.ValueSourceKind
	exprIndex    int
	targetIndex  int
	resultIndex  int
	callPoint    cfg.Point
	hasCallPoint bool
	final        bool
	expanded     bool
	adjusted     bool
	openTail     bool
}

func (l *lowerer) valueSourceExprRef(source sourceprovenance.ASTSource) (factflow.ExprRef, bool) {
	if !sourceScopedExprRef(source) {
		return l.exprRef(source.Expr)
	}
	return l.exprRef(sourceExprRefKey{
		expr:         source.Expr,
		kind:         source.Kind,
		exprIndex:    source.ExprIndex,
		targetIndex:  source.TargetIndex,
		resultIndex:  source.ResultIndex,
		callPoint:    source.CallPoint,
		hasCallPoint: source.HasCallPoint,
		final:        source.Final,
		expanded:     source.Expanded,
		adjusted:     source.Adjusted,
		openTail:     source.OpenTail,
	})
}

func sourceScopedExprRef(source sourceprovenance.ASTSource) bool {
	if !isAssertionWrapper(source.Expr) {
		return false
	}
	switch source.Kind {
	case factflow.ValueSourceCall, factflow.ValueSourceVararg:
		return true
	default:
		return false
	}
}

func isAssertionWrapper(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.CastExpr, *ast.NonNilAssertExpr:
		return true
	default:
		return false
	}
}
